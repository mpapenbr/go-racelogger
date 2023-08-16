package processor

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/google/go-cmp/cmp"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"golang.org/x/exp/slices"
)

// this struct is responsible for processing overall car data.
// this means overall standings, gaps, etc.
// the data for single cars is processed in CarData
type CarProc struct {
	api             *irsdk.Irsdk
	gpd             *GlobalProcessingData
	lastSessionTime float64
	// minimum distance a car has to move to be considered valid
	minMoveDistPct       float64
	winnerCrossedTheLine bool
	prevSessionTime      float64   // used for computing speed/distance
	prevLapDistPct       []float32 // data from previous iteration (CarIdxLapDistPct)
	carLookup            map[int]*CarData
	lastStandingsIR      []yaml.ResultsPositions

	carDriverProc   *CarDriverProc
	pitBoundaryProc *PitBoundaryProc
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
	if gpd.EventDataInfo.NumCarClasses > 1 {
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
	pitBoundaryProc *PitBoundaryProc) *CarProc {

	ret := &CarProc{api: api, gpd: gpd, carDriverProc: carDriverProc, pitBoundaryProc: pitBoundaryProc}
	ret.init()
	return ret
}

func (p *CarProc) init() {
	// car must move 10cm to be considered valid
	p.minMoveDistPct = 0.1 / p.gpd.TrackInfo.Length
	p.carLookup = make(map[int]*CarData)

}

// will be called every tick, we can assume to have valid data (no unexpected -1 values)
func (p *CarProc) Process() {
	// do nothing
	// currentTick := justValue(s.api.GetIntValue("SessionTick"))
	currentTime := justValue(p.api.GetDoubleValue("SessionTime")).(float64)

	// check if we have valid data, otherwise return
	if currentTime < 0 || currentTime <= p.lastSessionTime {
		return
	}
	// check if a race session is ongoing
	if !shouldRecord(p.api) {
		return
	}
	for _, idx := range p.getProcessableCarIdxs() {
		var carData *CarData
		var ok bool
		if carData, ok = p.carLookup[idx]; !ok {
			// we have a new car, create it
			carData = NewCarData(int32(idx), p.carDriverProc, p.pitBoundaryProc, p.gpd)
			p.carLookup[idx] = carData
		}
		carData.PreProcess(p.api)
		if slices.Contains([]string{CarStatePit, CarStateRun, CarStateSlow}, carData.state) {
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
		fmt.Printf("Standings-Delta: %v\n", cmp.Diff(curStandingsIR, p.lastStandingsIR))
		p.processStandings(curStandingsIR)
		// standings changed, update
		p.lastStandingsIR = curStandingsIR

	}

	// copy data for next iteration
	p.lastSessionTime = currentTime
	p.prevLapDistPct = make([]float32, len(justValue(p.api.GetFloatValues("CarIdxLapDistPct")).([]float32)))
	copy(p.prevLapDistPct, justValue(p.api.GetFloatValues("CarIdxLapDistPct")).([]float32))

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
		work.bestLap = st.FastestTime
		work.lastLap = st.LastTime
	}
	// TODO: mark laps as ob,pb,cb
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

	sort.Slice(work, standardRaceOrder)
	return work
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
