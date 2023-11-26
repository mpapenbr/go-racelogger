package processor

import (
	"fmt"

	"github.com/mpapenbr/goirsdk/irsdk"

	"github.com/mpapenbr/go-racelogger/log"
)

type carState interface {
	Enter(cd *CarData)
	Exit(cd *CarData)
	// gets called by the main processor with raw data from irsdk
	// at this point there are no main processor calculations done yet
	// this method is used to process the data from iRacing telemetry
	UpdatePre(cd *CarData, cw *carWorkData)
	// gets called by the main processor after all calculations are done
	// this method is used to process the data from the main processor
	UpdatePost(cd *CarData)
}

const (
	CarStateOut    = "OUT"
	CarStateRun    = "RUN"
	CarStatePit    = "PIT"
	CarStateSlow   = "SLOW"
	CarStateFinish = "FIN"
	CarSlowSpeed   = 25 // a car is considered slow if it is slower than this (km/h)
)

type carInit struct{}

func (ci *carInit) Enter(cd *CarData) { log.Info("Entering state: carInit") }
func (ci *carInit) Exit(cd *CarData)  { log.Info("Leaving state: carInit") }
func (ci *carInit) UpdatePre(cd *CarData, cw *carWorkData) {
	cd.copyWorkData(cw)
	if cw.trackPos == -1 {
		cd.state = CarStateOut
		cd.setState(&carOut{})
		return
	}
	if cw.pit {
		cd.state = CarStatePit
		cd.stintLap = 0
		cd.setState(&carPit{})
	} else {
		cd.state = CarStateRun
		cd.setState(&carRun{})
		return
	}
	cd.prepareMsgData()
}
func (ci *carInit) UpdatePost(cd *CarData) {}

type carRun struct{}

func (cr *carRun) Enter(cd *CarData) { log.Info("Entering state: carRun") }
func (cr *carRun) Exit(cd *CarData)  { log.Info("Leaving state: carRun") }
func (cr *carRun) UpdatePre(cd *CarData, cw *carWorkData) {
	if cw.trackPos == -1 {
		cd.state = CarStateOut
		cd.setState(&carOut{})
		return
	}
	if !cw.pit && int(cw.lc) > cd.lc {
		cd.stintLap += 1
	}
	cd.copyWorkData(cw)
	if cw.pit {
		cd.state = CarStatePit
		cd.pitstops += 1
		cd.setState(&carPit{})
		return
	}
}

func (cr *carRun) UpdatePost(cd *CarData) {
	if cd.speed > 0 && cd.speed < CarSlowSpeed {
		cd.state = CarStateSlow
		cd.setState(&carSlow{})
	}
}

type carSlow struct{}

func (cs *carSlow) Enter(cd *CarData) { log.Info("Entering state: carSlow") }
func (cs *carSlow) Exit(cd *CarData)  { log.Info("Leaving state: carSlow") }
func (cs *carSlow) UpdatePre(cd *CarData, cw *carWorkData) {
	if cw.trackPos == -1 {
		cd.state = CarStateOut
		cd.setState(&carOut{})
		return
	}
	if !cw.pit && int(cw.lc) > cd.lc {
		cd.stintLap += 1
	}
	cd.copyWorkData(cw)
	if cw.pit {
		cd.state = CarStatePit
		cd.pitstops += 1
		cd.setState(&carPit{})
		return
	}
}

func (cs *carSlow) UpdatePost(cd *CarData) {
	if cd.speed > 0 && cd.speed > CarSlowSpeed {
		cd.state = CarStateRun
		cd.setState(&carRun{})
	}
}

type carPit struct{}

func (cp *carPit) Enter(cd *CarData) {
	log.Info("Entering state: carPit")
	cd.pitBoundaryProc.processPitEntry(cd.trackPos)
}

func (cp *carPit) Exit(cd *CarData) {
	log.Info("Leaving state: carPit")
	cd.pitBoundaryProc.processPitExit(cd.trackPos)
}

func (cp *carPit) UpdatePre(cd *CarData, cw *carWorkData) {
	if cw.trackPos == -1 {
		cd.state = CarStateOut
		cd.setState(&carOut{})
		return
	}
	cd.copyWorkData(cw)

	if !cw.pit {
		cd.state = CarStateRun
		cd.setState(&carRun{})
		return
	}
}
func (cp *carPit) UpdatePost(cd *CarData) {}

type carFinished struct{}

func (cf *carFinished) Enter(cd *CarData) { log.Info("Entering state: carFinished") }
func (cf *carFinished) Exit(cd *CarData)  { log.Info("Leaving state: carFinished") }
func (cf *carFinished) UpdatePre(cd *CarData, cw *carWorkData) {
	// do nothing - final state
	cd.state = CarStateFinish
}
func (cf *carFinished) UpdatePost(cd *CarData) {}

type carOut struct{}

func (co *carOut) Enter(cd *CarData) { log.Info("Entering state: carOut") }
func (co *carOut) Exit(cd *CarData)  { log.Info("Leaving state: carOut") }
func (co *carOut) UpdatePre(cd *CarData, cw *carWorkData) {
	// this may happen after resets or tow to pit road.
	// if not on the pit road it may just be a short connection issue.
	if cw.pit {
		cd.state = CarStatePit
		cd.stintLap = 0
		cd.setState(&carPit{})
		return
	} else if cw.trackPos > -1 {
		cd.state = CarStateRun
		cd.setState(&carRun{})
		return
	}
}
func (co *carOut) UpdatePost(cd *CarData) {}

// contains data extracted from irsdk that needs to be processed by the carState
type carWorkData struct {
	carIdx   int32
	trackPos float64
	pos      int32
	pic      int32
	lap      int32
	lc       int32
	pit      bool
}

// CarData is a struct that contains the logic to process data for a single car data.
// Part of data is computed externally (e.g. CarProc) and passed in
type CarData struct {
	carIdx          int32
	msgData         map[string]interface{}
	state           string
	trackPos        float64
	bestLap         TimeWithMarker
	lastLap         TimeWithMarker
	currentSector   int
	stintLap        int
	pitstops        int
	pos             int
	pic             int
	lap             int
	lc              int
	dist            float64
	speed           float64
	interval        float64
	gap             float64
	currentState    carState
	laptiming       *CarLaptiming
	carDriverProc   *CarDriverProc
	pitBoundaryProc *PitBoundaryProc
	gpd             *GlobalProcessingData
}

//nolint:whitespace // can't get different linters happy
func NewCarData(
	carIdx int32,
	carDriverProc *CarDriverProc,
	pitBoundaryProc *PitBoundaryProc,
	gpd *GlobalProcessingData,
	reportLapStatus ReportTimingStatus,
) *CarData {
	laptiming := NewCarLaptiming(len(gpd.TrackInfo.Sectors), reportLapStatus)
	ret := CarData{
		carIdx:          carIdx,
		currentState:    &carInit{},
		msgData:         make(map[string]interface{}),
		carDriverProc:   carDriverProc,
		pitBoundaryProc: pitBoundaryProc,
		laptiming:       laptiming,
		gpd:             gpd,
		currentSector:   -1,
		lastLap:         TimeWithMarker{time: -1, marker: ""},
		bestLap:         TimeWithMarker{time: -1, marker: ""},
	}

	return &ret
}

func (cd *CarData) PreProcess(api *irsdk.Irsdk) {
	cw := cd.extractIrsdkData(api)
	cd.currentState.UpdatePre(cd, cw)
}

func (cd *CarData) PostProcess() {
	cd.currentState.UpdatePost(cd)
	cd.prepareMsgData()
}

func (cd *CarData) GetMsgData() map[string]interface{} {
	return cd.msgData
}

func (cd *CarData) setState(s carState) {
	cd.currentState.Exit(cd)
	cd.currentState = s
	cd.currentState.Enter(cd)
}

func (cd *CarData) prepareMsgData() {
	cd.msgData["carIdx"] = cd.carIdx
	cd.msgData["trackPos"] = cd.trackPos
	cd.msgData["pos"] = cd.pos
	cd.msgData["pic"] = cd.pic
	cd.msgData["lap"] = cd.lap
	cd.msgData["lc"] = cd.lc
	cd.msgData["pitstops"] = cd.pitstops
	cd.msgData["stintLap"] = cd.stintLap
	cd.msgData["speed"] = cd.speed
	cd.msgData["dist"] = cd.dist
	cd.msgData["interval"] = cd.interval
	cd.msgData["gap"] = cd.gap
	cd.msgData["last"] = []interface{}{
		cd.laptiming.lap.duration.time,
		cd.laptiming.lap.duration.marker,
	}
	cd.msgData["best"] = []interface{}{cd.bestLap.time, cd.bestLap.marker}
	cd.msgData["state"] = cd.state

	for i := 0; i < len(cd.gpd.TrackInfo.Sectors); i++ {
		cd.msgData[fmt.Sprintf("s%d", i+1)] = []interface{}{
			cd.laptiming.sectors[i].duration.time,
			cd.laptiming.sectors[i].duration.marker,
		}
	}

	cd.msgData["userName"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).UserName
	cd.msgData["teamName"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).TeamName
	cd.msgData["car"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarScreenNameShort
	cd.msgData["carNum"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarNumber

	cd.msgData["carClass"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarClassShortName
	if cd.msgData["carClass"] == "" {
		cd.msgData["carClass"] = fmt.Sprintf("CarClass %d",
			cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarClassID)
	}
}

func (cd *CarData) startSector(sectorNum int, t float64) {
	cd.currentSector = sectorNum
	cd.laptiming.sectors[sectorNum].markStart(t)
}

func (cd *CarData) markSectorsAsOld() {
	for i := 1; i < len(cd.gpd.TrackInfo.Sectors); i++ {
		cd.laptiming.sectors[i].markDuration(MarkerOldLap)
	}
}

func (cd *CarData) startLap(t float64) {
	cd.laptiming.lap.markStart(t)
}

func (cd *CarData) stopLap(t float64) float64 {
	return cd.laptiming.lap.markStop(t)
}

func (cd *CarData) isLapStarted() bool {
	return cd.laptiming.lap.isStarted()
}

func (cd *CarData) useOwnLaptime() {
	cd.lastLap.time = cd.laptiming.lap.duration.time
}

func (cd *CarData) setStandingsLaptime(t float64) {
	cd.laptiming.lap.duration.time = t
}

//nolint:lll // wrapping is not helpful here
func (cd *CarData) extractIrsdkData(api *irsdk.Irsdk) *carWorkData {
	cw := carWorkData{}
	cw.carIdx = cd.carIdx
	cw.trackPos = float64(justValue(api.GetValue("CarIdxLapDistPct")).([]float32)[cd.carIdx])
	cw.pos = justValue(api.GetValue("CarIdxPosition")).([]int32)[cd.carIdx]
	cw.pic = justValue(api.GetValue("CarIdxClassPosition")).([]int32)[cd.carIdx]
	cw.lap = justValue(api.GetValue("CarIdxLap")).([]int32)[cd.carIdx]
	cw.lc = justValue(api.GetValue("CarIdxLapCompleted")).([]int32)[cd.carIdx]
	cw.pit = justValue(api.GetValue("CarIdxOnPitRoad")).([]bool)[cd.carIdx]

	return &cw
}

func (cd *CarData) copyWorkData(cw *carWorkData) {
	cd.trackPos = gate(cw.trackPos)
	cd.pos = int(cw.pos)
	cd.pic = int(cw.pic)
	cd.lap = int(cw.lap)
	cd.lc = int(cw.lc)
	cd.dist = 0
	cd.interval = 0
}
