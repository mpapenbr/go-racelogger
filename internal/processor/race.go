package processor

import (
	"time"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/goirsdk/irsdk"
)

type raceState interface {
	Enter()
	Exit()
	Update(rp *RaceProc)
}

type RaceProc struct {
	api              *irsdk.Irsdk
	currentState     raceState
	cooldownEntered  time.Time
	carProc          *CarProc
	messageProc      *MessageProc
	RaceDoneCallback func()
}

type RaceInvalid struct{}

func (ri *RaceInvalid) Enter() { log.Info("Entering state: RaceInvalid") }
func (ri *RaceInvalid) Exit()  { log.Info("Leaving state: RaceInvalid") }
func (ri *RaceInvalid) Update(rp *RaceProc) {
	y, _ := rp.api.GetYaml()
	sessionNum := justValue(rp.api.GetIntValue("SessionNum")).(int32)
	if y.SessionInfo.Sessions[sessionNum].SessionType == "Race" {
		sessionSate := justValue(rp.api.GetIntValue("SessionState")).(int32)
		if sessionSate == int32(irsdk.StateRacing) {
			rp.messageProc.RaceStarts()
			rp.carProc.RaceStarts()
			rp.setState(&RaceRun{})
		}
	}
}

type RaceRun struct{}

func (rr *RaceRun) Enter() { log.Info("Entering state: RaceRun") }
func (rr *RaceRun) Exit()  { log.Info("Leaving state: RaceRun") }

// as long as we don't detect the checkered flag we stay in this state
func (rr *RaceRun) Update(rp *RaceProc) {
	sessionSate := justValue(rp.api.GetIntValue("SessionState")).(int32)
	if sessionSate == int32(irsdk.StateCheckered) {
		rp.messageProc.CheckeredFlagIssued()
		rp.carProc.CheckeredFlagIssued()
		rp.setState(&RaceFinishing{})
		return
	}
	rp.carProc.Process()
	// call carproc here
}

type RaceFinishing struct{}

func (rf *RaceFinishing) Enter() { log.Info("Entering state: RaceFinishing") }
func (rf *RaceFinishing) Exit()  { log.Info("Leaving state: RaceFinishing") }
func (rf *RaceFinishing) Update(rp *RaceProc) {
	sessionSate := justValue(rp.api.GetIntValue("SessionState")).(int32)
	if sessionSate == int32(irsdk.StateCoolDown) {
		rp.markEnterCooldown()
		rp.setState(&RaceCooldown{})
		return
	}
	rp.carProc.Process()
}

type RaceCooldown struct{}

func (rc *RaceCooldown) Enter() { log.Info("Entering state: RaceCooldown") }
func (rc *RaceCooldown) Exit()  { log.Info("Leaving state: RaceCooldown") }
func (rc *RaceCooldown) Update(rp *RaceProc) {
	if time.Since(rp.cooldownEntered) > time.Second*5 {
		rp.messageProc.RecordingDone()
		rp.setState(&RaceDone{})
		return
	}
	rp.carProc.Process()
}

type RaceDone struct{}

func (rd *RaceDone) Enter() { log.Info("Entering state: RaceDone") }
func (rd *RaceDone) Exit()  { log.Info("Leaving state: RaceDone") }
func (rd *RaceDone) Update(rp *RaceProc) {
	rp.onRaceDone()
}

func NewRaceProc(
	api *irsdk.Irsdk,
	carProc *CarProc,
	messageProc *MessageProc,
	raceDoneCallback func(),
) *RaceProc {
	ret := RaceProc{
		api:              api,
		carProc:          carProc,
		messageProc:      messageProc,
		RaceDoneCallback: raceDoneCallback,
	}
	ret.currentState = &RaceInvalid{}
	return &ret
}

func (rp *RaceProc) setState(s raceState) {
	rp.currentState.Exit()
	rp.currentState = s
	rp.currentState.Enter()
}

func (rp *RaceProc) markEnterCooldown() {
	rp.cooldownEntered = time.Now()
}

func (rp *RaceProc) onRaceDone() {
	// if handler registered, do something with it
	if rp.RaceDoneCallback != nil {
		rp.RaceDoneCallback()
	}
}

func (rp *RaceProc) Process() {
	rp.currentState.Update(rp)
}
