package processor

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"golang.org/x/exp/slices"
)

// this struct is responsible for processing overall car data.
// this means overall standings, gaps, etc.
// the data for single cars is processed in CarData
type CarProc struct {
	api *irsdk.Irsdk
	gpd *GlobalProcessingData

	// minimum distance a car has to move to be considered valid
	minMoveDistPct       float64
	winnerCrossedTheLine bool
	aboutToFinishMarker  []finishMarker // order at the time the checkered flag was waved
	currentTime          float64        // current sessionTime at start of this cycle
	prevSessionTime      float64        // used for computing speed/distance
	prevLapDistPct       []float32      // data from previous iteration (CarIdxLapDistPct)
	carLookup            map[int]*CarData
	lastStandingsIR      []yaml.ResultsPositions

	carDriverProc   *CarDriverProc
	pitBoundaryProc *PitBoundaryProc
	speedmapProc    *SpeedmapProc
	bestSectionProc *BestSectionProc
}

type finishMarker struct {
	carIdx int32
	ref    float64 // lap + trackPos at the time the checkered flag was waved
}

var baseAttributes = []string{"state", "carIdx", "carNum", "userName", "teamName", "car", "carClass", "pos", "pic", "lap", "lc", "gap", "interval", "trackPos", "speed", "dist", "pitstops", "stintLap", "last", "best"}

// this will become the new baseAttributes later. "static" data will be removed
var newBaseAttributes = []string{"state", "carIdx", "pos", "pic", "lap", "lc", "gap", "interval", "trackPos", "speed", "dist", "pitstops", "stintLap", "last", "best"}

func CarManifest(gpd *GlobalProcessingData) []string {
	// create copy of baseAttributes
	ret := make([]string, len(baseAttributes))
	copy(ret, baseAttributes)
	for i := range gpd.TrackInfo.Sectors {
		ret = append(ret, fmt.Sprintf("s%d", i+1))
	}
	if gpd.EventDataInfo.NumCarClasses == 1 {
		idx := slices.Index(ret, "carClass")
		ret = slices.Delete(ret, idx, idx+1)
	}
	if gpd.EventDataInfo.TeamRacing == 0 {
		idx := slices.Index(ret, "teamName")
		ret = slices.Delete(ret, idx, idx+1)
	}

	return ret
}
func NewCarProc(
	api *irsdk.Irsdk,
	gpd *GlobalProcessingData,
	carDriverProc *CarDriverProc,
	pitBoundaryProc *PitBoundaryProc,
	speedmapProc *SpeedmapProc) *CarProc {

	ret := &CarProc{
		api:             api,
		gpd:             gpd,
		carDriverProc:   carDriverProc,
		pitBoundaryProc: pitBoundaryProc,
		speedmapProc:    speedmapProc,
	}

	ret.init()
	return ret
}

func (p *CarProc) init() {

	collectInts := func(m map[int32][]yaml.Drivers) []int {
		ret := make([]int, 0)
		for k := range m {
			ret = append(ret, int(k))
		}
		return ret
	}

	// car must move 10cm to be considered valid
	p.minMoveDistPct = 0.1 / p.gpd.TrackInfo.Length
	p.carLookup = make(map[int]*CarData)

	p.bestSectionProc = NewBestSectionProc(len(p.gpd.TrackInfo.Sectors),
		collectInts(p.carDriverProc.byCarClassIdLookup),
		collectInts(p.carDriverProc.byCarIdLookup),
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

// will be called every tick, we can assume to have valid data (no unexpected -1 values)
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
	processableCars := p.getProcessableCarIdxs()
	for _, idx := range processableCars {
		var carData *CarData
		var ok bool
		if carData, ok = p.carLookup[idx]; !ok {
			// we have a new car, create it
			carData = NewCarData(int32(idx), p.carDriverProc, p.pitBoundaryProc, p.gpd)
			p.carLookup[idx] = carData

		}
		carData.PreProcess(p.api)
		if slices.Contains([]string{CarStatePit, CarStateRun, CarStateSlow}, carData.state) {
			speed := p.calcSpeed(carData)
			carData.speed = speed
			p.computeTimes(carData)
			p.speedmapProc.Process(carData)

			// compute times for car
			// compute speed for car
			// call postProcess for carData
		}

	}

	// at this point all cars have been processed
	y := p.api.GetLatestYaml()
	sessionNum := justValue(p.api.GetIntValue("SessionNum")).(int32)
	curStandingsIR := y.SessionInfo.Sessions[sessionNum].ResultsPositions
	if curStandingsIR != nil && !reflect.DeepEqual(curStandingsIR, p.lastStandingsIR) {
		log.Info("New standings available")
		// fmt.Printf("Standings-Delta: %v\n", cmp.Diff(curStandingsIR, p.lastStandingsIR))
		p.processStandings(curStandingsIR)
		// standings changed, update
		p.lastStandingsIR = curStandingsIR

	}
	// do post processing for all cars
	for _, c := range processableCars {
		p.carLookup[c].PostProcess()
	}
	// copy data for next iteration
	p.prevSessionTime = currentTime
	p.prevLapDistPct = make([]float32, len(justValue(p.api.GetFloatValues("CarIdxLapDistPct")).([]float32)))
	copy(p.prevLapDistPct, justValue(p.api.GetFloatValues("CarIdxLapDistPct")).([]float32))

}

func (p *CarProc) computeTimes(carData *CarData) {
	i := len(p.gpd.TrackInfo.Sectors) - 1
	for carData.trackPos < p.gpd.TrackInfo.Sectors[i].SectorStartPct {
		i--
	}
	if carData.currentSector == -1 {
		carData.currentSector = i
		// don't compute this sector. on -1 we are pretty much rushing into a running race or just put into the car
		return
	}
	if carData.currentSector == i {
		return // nothing to do, actions are done on sector change
	}
	//  the prev sector is done (we assume the car is running in the correct direction)
	//  but some strange things may happen: car spins, comes to a halt, drives in reverse direction and crosses the sector mark multiple times ;)
	//  very rare, I hope
	//  so we check if the current sector is the next "expected" sector
	expectedSector := (carData.currentSector + 1) % len(p.gpd.TrackInfo.Sectors)
	if i != expectedSector {
		return
	}

	carNum := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarNumber
	carId := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarID
	carClassId := carData.carDriverProc.GetCurrentDriver(carData.carIdx).CarClassID

	// if the sector has no start time we ignore it. prepare the next one and leave
	// need a pointer here, otherwise changes done here will get lost
	sector := carData.laptiming.sectors[carData.currentSector]

	if sector.isStarted() == false {
		carData.startSector(i, p.currentTime)
		log.Debug("Sector had no start time. Now initialized",
			log.String("carNum", carNum),
			log.Int("sector", i))
		return
	}

	duration := sector.markStop(p.currentTime)
	log.Debug("Sector completed",
		log.String("carNum", carNum),
		log.Int("sector", carData.currentSector),
		log.Float64("duration", duration))

	p.bestSectionProc.markSector(sector, carData.currentSector, carClassId, carId)

	// mark sectors as old when crossing the line
	if carData.currentSector == 0 {
		carData.markSectorsAsOld()
	}

	// start next sector (this will be i)
	carData.startSector(i, p.currentTime)

	// compute own laptime
	if carData.currentSector == 0 {
		log.Debug("Car crossed the line", log.String("carNum", carNum))
		if carData.isLapStarted() {
			carData.stopLap(p.currentTime)
			// no need to call bestSectionProc. This will be handled in processStandings
			// TODO: check race finish
		}
		if p.winnerCrossedTheLine {
			carData.setState(&carFinished{})
			carData.stintLap -= 1
			carData.lap = carData.lc
			log.Info("Car finished the race", log.String("carNum", carNum))
			return

		} else {
			if len(p.aboutToFinishMarker) > 0 {
				if float64(carData.lap) > p.aboutToFinishMarker[0].ref {
					p.winnerCrossedTheLine = true
					carData.setState(&carFinished{})
					carData.stintLap -= 1
					carData.lap = carData.lc
					log.Info("Car WON the race", log.String("carNum", carNum))
					return
				}

			}
		}
		carData.startLap(p.currentTime)

	}
}

func (p *CarProc) calcSpeed(carData *CarData) float64 {
	// carData has already recieved current trackPos
	if len(p.prevLapDistPct) == 0 {
		return -1
	}
	currentTrackPos := carData.trackPos
	moveDist := deltaDistance(currentTrackPos, gate(float64(p.prevLapDistPct[carData.carIdx])))
	deltaTime := p.currentTime - p.prevSessionTime
	if deltaTime != 0 {
		if moveDist < p.minMoveDistPct {
			// log.Debug("Car moved less than 10cm", log.Float64("moveDist", moveDist), log.Float64("minMoveDistPct", p.minMoveDistPct))
			return 0
		}
		speed := p.gpd.TrackInfo.Length * moveDist / deltaTime * 3.6
		// old safe guard from python variant
		if speed > 400 {
			log.Warn("Speed > 400",
				log.String("carNum", p.carDriverProc.GetCurrentDriver(carData.carIdx).CarNumber),
				log.Float64("speed", speed))
			return -1
		}
		return speed
	} else {
		log.Debug("Delta time is 0")
		return 0
	}
	// compute speed
}

func (p *CarProc) processStandings(curStandingsIR []yaml.ResultsPositions) {
	// Note: IR-standings are provided with a little delay after cars crossed the line

	for _, st := range curStandingsIR {
		work := p.carLookup[st.CarIdx]
		if work == nil {
			// rare case when reconnecting to a session
			work = NewCarData(int32(st.CarIdx), p.carDriverProc, p.pitBoundaryProc, p.gpd)
			p.carLookup[st.CarIdx] = work
		}
		work.pos = int(st.Position)
		work.pic = int(st.ClassPosition)
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
			log.Debug("Best lap",
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
		byCar[car.carDriverProc.GetCurrentDriver(car.carIdx).CarID] = append(byCar[car.carDriverProc.GetCurrentDriver(car.carIdx).CarID], car)
		byClass[car.carDriverProc.GetCurrentDriver(car.carIdx).CarClassID] = append(byClass[car.carDriverProc.GetCurrentDriver(car.carIdx).CarClassID], car)
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
	return getProcessableCarIdxs(y.DriverInfo.Drivers)
}

// returns []*CarData in current race order
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
		return (float64(work[i].lap) + work[i].trackPos) > (float64(work[j].lap) + work[j].trackPos)
	}

	raceEndingOrder := func(i, j int) bool {
		return work[i].pos < work[j].pos
	}
	if p.winnerCrossedTheLine {
		sort.Slice(work, raceEndingOrder)
	} else {
		sort.Slice(work, standardRaceOrder)
	}
	return work
}

func (p *CarProc) RaceStarts() {
	log.Info("Recieved race start event. ")
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

func (p *CarProc) CreatePayload() [][]interface{} {
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
