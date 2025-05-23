//nolint:funlen,nestif,cyclop //checked various places, no need to refactor
package processor

import (
	"context"
	"fmt"
	"math"
	"reflect"
	"sort"

	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/mpapenbr/goirsdk/yaml"
	"github.com/samber/lo"
	"golang.org/x/exp/slices"

	"github.com/mpapenbr/go-racelogger/log"
)

// this struct is responsible for processing overall car data.
// this means overall standings, gaps, etc.
// the data for single cars is processed in CarData
type CarProc struct {
	ctx context.Context
	api *irsdk.Irsdk
	gpd *GlobalProcessingData

	// minimum distance a car has to move to be considered valid
	minMoveDistPct       float64
	winnerCrossedTheLine bool
	aboutToFinishMarker  []finishMarker // order at the time the checkered flag was waved
	currentTime          float64        // current sessionTime at start of this cycle
	prevSessionTime      float64        // used for computing speed/distance
	prevLapDistPct       []float32      // data from previous iteration (CarIdxLapDistPct)
	prevLapPos           []int32        // data from previous iteration (CarIdxLap)
	sessionNum           int32          // current session number
	carLookup            map[int]*CarData

	lastStandingsIR []yaml.ResultsPositions

	carDriverProc   *CarDriverProc
	pitBoundaryProc *PitBoundaryProc
	speedmapProc    *SpeedmapProc
	messageProc     *MessageProc
	bestSectionProc *BestSectionProc

	maxSpeed float64
	log      *log.Logger
}

type finishMarker struct {
	carIdx int32
	ref    float64 // lap + trackPos at the time the checkered flag was waved
}

//nolint:unused // used for reference
var oldBaseAttributes = []string{
	"state",
	"carIdx",
	"carNum",
	"userName",
	"teamName",
	"car",
	"carClass",
	"pos",
	"pic",
	"lap",
	"lc",
	"gap",
	"interval",
	"trackPos",
	"speed",
	"dist",
	"pitstops",
	"stintLap",
	"last",
	"best",
}

// this will become the new baseAttributes later. "static" data will be removed
var baseAttributes = []string{
	"state",
	"carIdx",
	"pos",
	"pic",
	"lap",
	"lc",
	"gap",
	"interval",
	"trackPos",
	"speed",
	"dist",
	"pitstops",
	"stintLap",
	"last",
	"best",
	"tireCompound",
}

//nolint:makezero // false positive?
func CarManifest(gpd *GlobalProcessingData) []string {
	// create copy of baseAttributes
	ret := make([]string, len(baseAttributes))
	copy(ret, baseAttributes)
	for i := range gpd.TrackInfo.Sectors {
		ret = append(ret, fmt.Sprintf("s%d", i+1))
	}
	// if gpd.EventDataInfo.NumCarClasses == 1 {
	// 	idx := slices.Index(ret, "carClass")
	// 	ret = slices.Delete(ret, idx, idx+1)

	// 	idx = slices.Index(ret, "pic")
	// 	ret = slices.Delete(ret, idx, idx+1)
	// }
	// if gpd.EventDataInfo.TeamRacing == 0 {
	// 	idx := slices.Index(ret, "teamName")
	// 	ret = slices.Delete(ret, idx, idx+1)
	// }

	return ret
}

//nolint:whitespace // can't get different linters happy
func NewCarProc(
	ctx context.Context,
	api *irsdk.Irsdk,
	gpd *GlobalProcessingData,
	carDriverProc *CarDriverProc,
	pitBoundaryProc *PitBoundaryProc,
	speedmapProc *SpeedmapProc,
	messageProc *MessageProc,
	maxSpeed float64,
) *CarProc {
	ret := &CarProc{
		ctx:             ctx,
		api:             api,
		gpd:             gpd,
		carDriverProc:   carDriverProc,
		pitBoundaryProc: pitBoundaryProc,
		speedmapProc:    speedmapProc,
		messageProc:     messageProc,
		maxSpeed:        maxSpeed,
		log:             log.GetFromContext(ctx).Named("CarProc"),
	}

	ret.init()
	return ret
}

//nolint:gocritic,gocognit // by design
func (p *CarProc) init() {
	collectInts := func(m map[int32][]yaml.Drivers) []int {
		ret := make([]int, 0)
		for k := range m {
			ret = append(ret, int(k))
		}
		return ret
	}

	// car must move 10cm to be considered valid
	p.minMoveDistPct = 0.1 / float64(p.gpd.TrackInfo.Length)
	p.carLookup = make(map[int]*CarData)

	p.bestSectionProc = NewBestSectionProc(len(p.gpd.TrackInfo.Sectors),
		collectInts(p.carDriverProc.byCarClassIDLookup),
		collectInts(p.carDriverProc.byCarIDLookup),
		func(carClassId, carId int) []*CarLaptiming {
			work := make([]*CarLaptiming, 0)
			for i, v := range p.carLookup {
				curEntry := p.carDriverProc.GetCurrentDriver(int32(i))
				if carId != -1 {
					if carId == curEntry.CarID {
						work = append(work, v.laptiming)
					}
				} else if carClassId != -1 {
					if carClassId == curEntry.CarClassID {
						work = append(work, v.laptiming)
					}
				} else {
					work = append(work, v.laptiming)
				}
			}
			return work
		})
}

func (p *CarProc) newCarData(carIdx int) *CarData {
	reportLapStatus := func(twm TimeWithMarker) {
		if twm.marker != MarkerOldLap {
			p.messageProc.ReportDriverLap(carIdx, twm)
		}
	}
	return NewCarData(
		p.ctx,
		int32(carIdx),
		p.carDriverProc,
		p.pitBoundaryProc,
		p.gpd,
		reportLapStatus)
}

// will be called every tick, we can assume to have valid data (no unexpected -1 values)
//
//nolint:gocognit,gocritic,errcheck // by design
func (p *CarProc) Process() {
	// do nothing
	// currentTick := justValue(s.api.GetIntValue("SessionTick"))
	currentTime := justValue(p.api.GetDoubleValue("SessionTime")).(float64)

	// check if we have valid data, otherwise return
	if currentTime < 0 || currentTime <= p.prevSessionTime {
		return
	}
	// check if a race session is ongoing
	if !shouldRecord(p.api) {
		return
	}
	p.currentTime = currentTime
	sessionNum := justValue(p.api.GetIntValue("SessionNum")).(int32)
	p.sessionNum = sessionNum
	processableCars := p.getProcessableCarIdxs()
	for _, idx := range processableCars {
		var carData *CarData
		var ok bool
		if carData, ok = p.carLookup[idx]; !ok {
			// we have a new car, create it
			carData = p.newCarData(idx)
			p.carLookup[idx] = carData
		}
		carData.PreProcess(p.api)
		if slices.Contains([]string{CarStatePit, CarStateRun, CarStateSlow}, carData.state) {
			driver := p.carDriverProc.GetCurrentDriver(int32(idx))
			speed := p.calcSpeed(carData)
			// use only valid speed values
			if speed >= 0 {
				carData.speed = speed
				// use speed for speedmap only is car is not in pits
				if carData.state != CarStatePit {
					p.speedmapProc.Process(carData, driver.CarClassID, driver.CarID)
				}
			}
			p.computeTimes(carData)
		}
	}
	// at this point all cars have been processed

	y := p.api.GetLatestYaml()

	if y.SessionInfo.Sessions[sessionNum].SessionType == "Race" {
		p.calcDelta()
	}

	curStandingsIR := y.SessionInfo.Sessions[sessionNum].ResultsPositions
	if curStandingsIR != nil && !reflect.DeepEqual(curStandingsIR, p.lastStandingsIR) {
		p.log.Info("New standings available")
		// fmt.Printf("Standings-Delta: %v\n", cmp.Diff(curStandingsIR, p.lastStandingsIR))
		p.processStandings(curStandingsIR)
		// standings changed, update
		p.lastStandingsIR = curStandingsIR
	}
	// do post processing for all cars
	for _, c := range processableCars {
		p.carLookup[c].PostProcess()
	}
	if len(p.getInCurrentRaceOrder()) > 0 {
		p.speedmapProc.SetLeaderTrackPos(p.getInCurrentRaceOrder()[0].trackPos)
	}
	// copy data for next iteration
	p.prevSessionTime = currentTime
	p.prevLapDistPct = make([]float32,
		len(justValue(p.api.GetFloatValues("CarIdxLapDistPct")).([]float32)))
	copy(p.prevLapDistPct, justValue(p.api.GetFloatValues("CarIdxLapDistPct")).([]float32))
	p.prevLapPos = make([]int32, len(justValue(p.api.GetIntValues("CarIdxLap")).([]int32)))
	copy(p.prevLapPos, justValue(p.api.GetIntValues("CarIdxLap")).([]int32))
}

//nolint:unused,gocritic // used for debugging
func (p *CarProc) carInfo(carIdx int) {
	carData := p.carLookup[carIdx]
	carNum := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarNumber
	// carId := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarID
	// carClassId := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarClassID

	p.log.Warn("CarInfo",
		log.String("carNum", carNum),
		log.Float64("carPos", carData.trackPos),
	)
}

func (p *CarProc) computeTimes(carData *CarData) {
	i := len(p.gpd.TrackInfo.Sectors) - 1
	for carData.trackPos < float64(p.gpd.TrackInfo.Sectors[i].StartPct) {
		i--
	}
	if carData.currentSector == -1 {
		carData.currentSector = i
		// don't compute this sector.
		// on -1 we are pretty much rushing into a running race or just put into the car
		return
	}
	if carData.currentSector == i {
		return // nothing to do, actions are done on sector change
	}
	// the prev sector is done (we assume the car is running in the correct direction)
	// but some strange things may happen:
	// car spins, comes to a halt, drives in reverse direction and crosses the sector
	// mark multiple times ;)
	// very rare, I hope
	// so we check if the current sector is the next "expected" sector
	expectedSector := (carData.currentSector + 1) % len(p.gpd.TrackInfo.Sectors)
	if i != expectedSector {
		return
	}

	carNum := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarNumber
	carID := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarID
	carClassID := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarClassID

	// if the sector has no start time we ignore it. prepare the next one and leave
	// need a pointer here, otherwise changes done here will get lost
	sector := carData.laptiming.sectors[carData.currentSector]

	if !sector.isStarted() {
		carData.startSector(i, p.currentTime)
		p.log.Debug("Sector had no start time. Now initialized",
			log.String("carNum", carNum),
			log.Int("sector", i))
		return
	}

	sector.markStop(p.currentTime)

	p.bestSectionProc.markSector(sector, carData.currentSector, carClassID, carID)

	// mark sectors as old when crossing the line
	if carData.currentSector == 0 {
		carData.markSectorsAsOld()
	}

	// start next sector (this will be i)
	carData.startSector(i, p.currentTime)

	// compute own laptime
	if carData.currentSector == 0 {
		p.log.Debug("Car crossed the line", log.String("carNum", carNum))
		if carData.isLapStarted() {
			carData.stopLap(p.currentTime)
			// no need to call bestSectionProc. This will be handled in processStandings
			// TODO: check race finish
		}
		if p.winnerCrossedTheLine {
			carData.setState(&carFinished{})
			carData.stintLap -= 1
			carData.lap = carData.lc
			p.log.Info("Car finished the race", log.String("carNum", carNum))
			return
		} else if len(p.aboutToFinishMarker) > 0 {
			if float64(carData.lap) > p.aboutToFinishMarker[0].ref {
				p.winnerCrossedTheLine = true
				carData.setState(&carFinished{})
				carData.stintLap -= 1
				carData.lap = carData.lc
				p.log.Info("Car WON the race", log.String("carNum", carNum))
				return
			}
		}
		carData.startLap(p.currentTime)
	}
}

func (p *CarProc) calcSpeed(carData *CarData) float64 {
	output := func(f float64) string {
		return fmt.Sprintf("%.4f", f)
	}
	// carData has already received current trackPos
	if len(p.prevLapDistPct) == 0 {
		return -1
	}
	currentTrackPos := float64(carData.lap) + carData.trackPos
	prevLap := p.prevLapPos[carData.carIdx]
	prevTrackPos := float64(p.prevLapPos[carData.carIdx]) +
		gate(float64(p.prevLapDistPct[carData.carIdx]))
	if prevLap < 0 || carData.lap < 0 {
		return -1
	}
	moveDist := currentTrackPos - prevTrackPos
	// issue warning if car moved backward more than minMoveDistPct
	if moveDist < 0 && math.Abs(moveDist) > p.minMoveDistPct {
		carData.log.Warn(
			"Car moved backward???",
			// log.String("carNum", p.carDriverProc.GetCurrentDriver(carData.carIdx).CarNumber),
			log.Float64("prevTrackPos", prevTrackPos),
			log.Float64("currentTrackPos", currentTrackPos),
			log.Float64("dist", moveDist),
			log.String(
				"prevTrackPos(m)",
				output(
					gate(float64(p.prevLapDistPct[carData.carIdx]))*float64(p.gpd.TrackInfo.Length),
				),
			),
			log.String(
				"currentTrackPos(m)",
				output(carData.trackPos*float64(p.gpd.TrackInfo.Length)),
			),
			log.String("dist(m)", output(moveDist*float64(p.gpd.TrackInfo.Length))),
		)
		return -1
	}
	deltaTime := p.currentTime - p.prevSessionTime
	if deltaTime != 0 {
		if moveDist < p.minMoveDistPct {
			return 0
		}
		speed := float64(p.gpd.TrackInfo.Length) * moveDist / deltaTime * 3.6
		if speed > p.maxSpeed {
			carData.log.Warn("Speed above maxSpeed. Ignoring",
				log.Float64("speed", speed),
				log.Float64("maxSpeed", p.maxSpeed))
			return -1
		}
		return speed
	} else {
		p.log.Debug("Delta time is 0")
		return 0
	}
	// compute speed
}

func (p *CarProc) calcDelta() {
	currentRaceOrder := p.getInCurrentRaceOrder()
	if len(currentRaceOrder) == 0 {
		return
	}
	for i, car := range currentRaceOrder[1:] {
		// since i starts at 0 we use this as index for currentRaceOrder for the car in front
		// note the range skips the first car in currentRaceOrder
		if car.pos < 0 {
			continue
		}
		if car.state == CarStateOut {
			continue
		}
		if car.state == CarStateFinish {
			carInFront := currentRaceOrder[i]
			gapOfCarInFront := carInFront.gap
			car.interval = car.gap - gapOfCarInFront
			car.dist = 0
			continue
		}

		car.dist = deltaDistance(
			currentRaceOrder[i].trackPos,
			car.trackPos,
		) * float64(
			p.gpd.TrackInfo.Length,
		)
		if car.speed <= 0 {
			car.interval = 999
		} else {
			carClassID := car.carDriverProc.GetCurrentDriver(car.carIdx).CarClassID
			deltaByCarClassSpeemap := p.speedmapProc.ComputeDeltaTime(
				carClassID,
				currentRaceOrder[i].trackPos,
				car.trackPos)
			if deltaByCarClassSpeemap < 0 {
				p.log.Warn("Negative delta by speedmap",
					log.String("carNum", car.carDriverProc.GetCurrentDriver(car.carIdx).CarNumber),
					log.Float64("cifPos", currentRaceOrder[i].trackPos),
					log.Float64("carPos", car.trackPos),
					log.Float64("delta", deltaByCarClassSpeemap))
			}
			car.interval = deltaByCarClassSpeemap
		}
	}
}

//nolint:gocritic // by design
func (p *CarProc) processStandings(curStandingsIR []yaml.ResultsPositions) {
	// Note: IR-standings are provided with a little delay after cars crossed the line
	for _, st := range curStandingsIR {
		work := p.carLookup[st.CarIdx]
		if work == nil {
			// rare case when reconnecting to a session
			work = p.newCarData(st.CarIdx)
			p.carLookup[st.CarIdx] = work
		}
		work.pos = st.Position
		// iRacing sets pic to 0-based at race finish, we correct it here
		if p.winnerCrossedTheLine {
			work.pic = st.ClassPosition + 1
		} else {
			work.pic = st.ClassPosition
		}
		work.gap = st.Time
		work.bestLap.time = st.FastestTime
		standingsLaptime := st.LastTime

		if standingsLaptime == -1 {
			work.useOwnLaptime()
		} else {
			work.setStandingsLaptime(st.LastTime)
		}

		p.bestSectionProc.markLap(work.laptiming.lap,
			work.carDriverProc.GetCurrentDriver(work.carIdx).CarClassID,
			work.carDriverProc.GetCurrentDriver(work.carIdx).CarID)
	}

	p.markBestLaps()
}

func (p *CarProc) markBestLaps() {
	// mark bestLaps
	byCar := make(map[int][]*CarData)
	byClass := make(map[int][]*CarData)
	all := make([]*CarData, 0)
	for _, car := range p.carLookup {
		if car.bestLap.time != -1 {
			all = append(all, car)
		}
	}
	if len(all) == 0 {
		return
	}
	sortByBestLap := func(a, b *CarData) int {
		if a.bestLap.time < b.bestLap.time {
			return -1
		} else if a.bestLap.time > b.bestLap.time {
			return 1
		}
		return 0
	}
	debugBest := func(title string, car []*CarData) {
		for _, item := range car {
			p.log.Debug("Best lap",
				log.String("title", title),
				log.String("carNum", item.carDriverProc.GetCurrentDriver(item.carIdx).CarNumber),
				log.Float64("bestLap", item.bestLap.time),
				log.String("marker", item.bestLap.marker))
		}
	}

	slices.SortStableFunc(all, sortByBestLap)
	if false {
		debugBest("All", all)
	}

	for _, car := range all {
		byCar[car.carDriverProc.GetCurrentDriver(car.carIdx).CarID] = append(
			byCar[car.carDriverProc.GetCurrentDriver(car.carIdx).CarID], car)
		byClass[car.carDriverProc.GetCurrentDriver(car.carIdx).CarClassID] = append(
			byClass[car.carDriverProc.GetCurrentDriver(car.carIdx).CarClassID], car)
	}

	// reset all marker
	for _, item := range all {
		item.bestLap.marker = ""
	}
	for _, item := range byCar {
		item[0].bestLap.marker = MarkerCarBest
	}
	for _, item := range byClass {
		item[0].bestLap.marker = MarkerClassBest
	}

	all[0].bestLap.marker = MarkerOverallBest
}

func (p *CarProc) getProcessableCarIdxs() []int {
	y := p.api.GetLatestYaml()
	collect := func(entries []yaml.Results) []int {
		ret := []int{}
		for i := range y.DriverInfo.Drivers {
			d := y.DriverInfo.Drivers[i]
			_, _, found := lo.FindIndexOf(entries,
				func(item yaml.Results) bool { return item.CarIdx == d.CarIdx },
			)
			if found {
				ret = append(ret, d.CarIdx)
			}
		}
		return ret
	}
	// first: check QualifyPositions on session
	if y.SessionInfo.Sessions[p.sessionNum].QualifyPositions != nil {
		return collect(y.SessionInfo.Sessions[p.sessionNum].QualifyPositions)
	}
	// second: check QualifyResultPositions on top level
	if y.QualifyResultsInfo.Results != nil {
		return collect(y.QualifyResultsInfo.Results)
	}
	// third: use fallback
	return getProcessableCarIdxs(y.DriverInfo.Drivers)
}

// returns []*CarData in current race order
//
//nolint:gocritic,makezero // no need to refactor
func (p *CarProc) getInCurrentRaceOrder() []*CarData {
	if len(p.carLookup) == 0 {
		return []*CarData{}
	}
	carIdxs := p.getProcessableCarIdxs()
	work := make([]*CarData, len(carIdxs))
	for i, idx := range carIdxs {
		work[i] = p.carLookup[idx]
	}

	standardRaceOrder := func(i, j int) bool {
		return (float64(work[i].lap) + work[i].trackPos) >
			(float64(work[j].lap) + work[j].trackPos)
	}

	raceEndingOrder := func(a, b *CarData) int {
		if a.pos < b.pos {
			return -1
		} else if a.pos > b.pos {
			return 1
		} else {
			return 0
		}
	}
	if p.winnerCrossedTheLine {
		invalidPos := make([]*CarData, 0)
		validPos := make([]*CarData, 0)
		for _, item := range work {
			if item.pos > 0 {
				validPos = append(validPos, item)
			} else {
				invalidPos = append(invalidPos, item)
			}
		}
		slices.SortStableFunc(validPos, raceEndingOrder)

		work = make([]*CarData, 0)
		work = append(work, validPos...)
		work = append(work, invalidPos...)
	} else {
		sort.Slice(work, standardRaceOrder)
	}
	return work
}

func (p *CarProc) RaceStarts() {
	p.log.Info("Received race start event. ")
	// have to check if we need this.....
	// for _, idx := range p.getProcessableCarIdxs() {
	// 	p.carLookup[idx].startLap(p.currentTime)
	// }
}

func (p *CarProc) CheckeredFlagIssued() {
	// from now on we only want data for cars who still not have finished the race
	// we compute a marker by lc + trackPos.
	// The next car that crosses the line with dist > marker is the winner
	// (Note: does not work if all cars currently on the leading lap do not reach the s/f)
	p.aboutToFinishMarker = make([]finishMarker, 0)
	for _, car := range p.getInCurrentRaceOrder() {
		p.aboutToFinishMarker = append(p.aboutToFinishMarker,
			finishMarker{carIdx: car.carIdx, ref: float64(car.lap) + car.trackPos})
	}
}

func (p *CarProc) CreatePayloadWamp() [][]interface{} {
	cars := p.getInCurrentRaceOrder()
	payload := make([][]interface{}, len(cars))
	manifest := CarManifest(p.gpd)
	createMessage := func(c *CarData) []interface{} {
		ret := make([]interface{}, len(manifest))
		msgData := c.GetMsgData()
		for idx, attr := range manifest {
			ret[idx] = msgData[attr]
		}
		return ret
	}
	for i, c := range cars {
		payload[i] = createMessage(c)
	}
	return payload
}

func (p *CarProc) CreatePayload() []*racestatev1.Car {
	cars := p.getInCurrentRaceOrder()
	payload := make([]*racestatev1.Car, len(cars))
	for i, c := range cars {
		payload[i] = c.prepareGrpcData()
	}
	return payload
}
