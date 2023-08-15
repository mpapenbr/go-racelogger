package processor

import (
	"fmt"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
)

type carState interface {
	Enter()
	Exit()
	Update(cd *CarData, cw *carWorkData)
}

type carInit struct{}

func (ci *carInit) Enter() { log.Info("Entering state: carInit") }
func (ci *carInit) Exit()  { log.Info("Leaving state: carInit") }
func (ci *carInit) Update(cd *CarData, cw *carWorkData) {
	cd.copyWorkData(cw)
	if cw.trackPos == -1 {
		cd.state = "OUT"
		cd.setState(&carOut{})
		// cd.prepareMsgData()
		return
	}
	if cw.pit {
		cd.state = "PIT"
		cd.stintLap = 0
		cd.setState(&carPit{})
	} else {
		cd.state = "RUN"
		cd.setState(&carRun{})
		return
	}
	cd.prepareMsgData()

}

type carRun struct{}

func (cr *carRun) Enter() { log.Info("Entering state: carRun") }
func (cr *carRun) Exit()  { log.Info("Leaving state: carRun") }
func (cr *carRun) Update(cd *CarData, cw *carWorkData) {
	if cw.trackPos == -1 {
		cd.state = "OUT"
		cd.setState(&carOut{})
		return
	}
	cd.copyWorkData(cw)
	if cw.pit {
		cd.state = "PIT"
		cd.pitstops += 1
		cd.setState(&carPit{})
		// call pit boundary monitor with entry
		return
	}

}

type carSlow struct{}
type carPit struct{}

func (cp *carPit) Enter() { log.Info("Entering state: carPit") }
func (cp *carPit) Exit()  { log.Info("Leaving state: carPit") }
func (cp *carPit) Update(cd *CarData, cw *carWorkData) {

	if cw.trackPos == -1 {
		cd.state = "OUT"
		cd.setState(&carOut{})
		return
	}
	cd.copyWorkData(cw)
	if cw.pit == false {
		cd.state = "RUN"
		cd.setState(&carRun{})
		// call pit boundary monitor with exit
		return
	}
}

type carFinished struct{}
type carOut struct{}

func (co *carOut) Enter() { log.Info("Entering state: carOut") }
func (co *carOut) Exit()  { log.Info("Leaving state: carOut") }
func (co *carOut) Update(cd *CarData, cw *carWorkData) {
	cd.copyWorkData(cw)

}

// contains data extracted from irsdk that needs to be processed by the carState
type carWorkData struct {
	carIdx        int32
	trackPos      float64
	pos           int32
	pic           int32
	lap           int32
	lc            int32
	currentSector int32
	pit           bool
	speed         float64
}

type CarData struct {
	carIdx        int32
	msgData       map[string]interface{}
	state         string
	trackPos      float64
	currentBest   float64
	slowMarker    bool
	currentSector int
	stintLap      int
	pitstops      int
	pos           int
	pic           int
	lap           int
	lc            int
	dist          int
	speed         float64
	interval      float64
	gap           float64
	currentState  carState
	carDriverProc *CarDriverProc
}

func NewCarData(carIdx int32, carDriverProc *CarDriverProc) *CarData {
	ret := CarData{carIdx: carIdx, carDriverProc: carDriverProc}
	ret.init()
	return &ret
}

func (cd *CarData) init() {
	cd.currentState = &carInit{}
	cd.msgData = make(map[string]interface{})

}

// CarProc is a struct that contains the logic to process data for a single car data.

func (cd *CarData) Process(api *irsdk.Irsdk) {

	cw := cd.extractIrsdkData(api)
	log.Debug("Dummy", log.Any("carWorkData", cw))
	cd.currentState.Update(cd, cw)
	cd.prepareMsgData()
}

func (cd *CarData) GetMsgData() map[string]interface{} {
	return cd.msgData
}

func (cd *CarData) setState(s carState) {
	cd.currentState.Exit()
	cd.currentState = s
	cd.currentState.Enter()
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
	cd.msgData["last"] = cd.currentBest
	cd.msgData["best"] = cd.currentBest
	cd.msgData["state"] = cd.currentState

	cd.msgData["userName"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).UserName
	cd.msgData["teamName"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).TeamName
	cd.msgData["car"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarScreenNameShort
	cd.msgData["carNum"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarNumber

	cd.msgData["carClass"] = cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarClassShortName
	if cd.msgData["carClass"] == "" {
		cd.msgData["carClass"] = fmt.Sprintf("CarClass %d", cd.carDriverProc.GetCurrentDriver(cd.carIdx).CarClassID)
	}
}

func (cd *CarData) extractIrsdkData(api *irsdk.Irsdk) *carWorkData {
	cw := carWorkData{}
	cw.carIdx = cd.carIdx
	cw.trackPos = justValue(api.GetValue("CarIdxLapDistPct")).([]float64)[cd.carIdx]
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
