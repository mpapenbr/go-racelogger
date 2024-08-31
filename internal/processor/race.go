//nolint:errcheck // won't check everytime on type conversion
package processor

import (
	"context"
	"time"

	"github.com/mpapenbr/goirsdk/irsdk"

	"github.com/mpapenbr/go-racelogger/log"
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
	RaceRunCallback  func()
	RaceDoneCallback func()

	stateInvalid, stateRun, stateFinishing, stateCooldown, stateDone raceState
}

type RaceInvalid struct {
	log *log.Logger
}

func (ri *RaceInvalid) Enter() { ri.log.Info("enter state") }
func (ri *RaceInvalid) Exit()  { ri.log.Info("exit state") }
func (ri *RaceInvalid) Update(rp *RaceProc) {
	y := rp.api.GetLatestYaml()
	sessionNum := justValue(rp.api.GetIntValue("SessionNum")).(int32)
	if y.SessionInfo.Sessions[sessionNum].SessionType == "Race" {
		sessionSate := justValue(rp.api.GetIntValue("SessionState")).(int32)
		if sessionSate == int32(irsdk.StateRacing) {
			rp.messageProc.RaceStarts()
			rp.carProc.RaceStarts()
			rp.setState(rp.stateRun)
			if rp.RaceRunCallback != nil {
				rp.RaceRunCallback()
			}
		}
	}
}

type RaceRun struct {
	log *log.Logger
}

func (rr *RaceRun) Enter() { rr.log.Info("enter state") }
func (rr *RaceRun) Exit()  { rr.log.Info("exit state") }

// as long as we don't detect the checkered flag we stay in this state
func (rr *RaceRun) Update(rp *RaceProc) {
	sessionSate := justValue(rp.api.GetIntValue("SessionState")).(int32)
	if sessionSate == int32(irsdk.StateCheckered) {
		rp.messageProc.CheckeredFlagIssued()
		rp.carProc.CheckeredFlagIssued()
		rp.setState(rp.stateFinishing)
		return
	}
	rp.carProc.Process()
	// call carproc here
}

type RaceFinishing struct {
	log *log.Logger
}

func (rf *RaceFinishing) Enter() { rf.log.Info("enter state") }
func (rf *RaceFinishing) Exit()  { rf.log.Info("exit state") }
func (rf *RaceFinishing) Update(rp *RaceProc) {
	sessionSate := justValue(rp.api.GetIntValue("SessionState")).(int32)
	if sessionSate == int32(irsdk.StateCoolDown) {
		rp.markEnterCooldown()
		rp.setState(rp.stateCooldown)
		return
	}
	rp.carProc.Process()
}

type RaceCooldown struct {
	log *log.Logger
}

func (rc *RaceCooldown) Enter() { rc.log.Info("enter state") }
func (rc *RaceCooldown) Exit()  { rc.log.Info("exist state") }
func (rc *RaceCooldown) Update(rp *RaceProc) {
	if time.Since(rp.cooldownEntered) > time.Second*5 {
		rp.messageProc.RecordingDone()
		rp.setState(rp.stateDone)
		return
	}
	rp.carProc.Process()
}

type RaceDone struct {
	log *log.Logger
}

func (rd *RaceDone) Enter() { rd.log.Info("enter state") }
func (rd *RaceDone) Exit()  { rd.log.Info("exit state") }
func (rd *RaceDone) Update(rp *RaceProc) {
	rp.onRaceDone()
}

//nolint:whitespace // can't get different linters happy
func NewRaceProc(
	ctx context.Context,
	api *irsdk.Irsdk,
	carProc *CarProc,
	messageProc *MessageProc,
	raceDoneCallback func(),
) *RaceProc {
	createLogger := func(name string) *log.Logger {
		return log.GetFromContext(ctx).Named("state" + "." + name)
	}
	ret := RaceProc{
		api:              api,
		carProc:          carProc,
		messageProc:      messageProc,
		RaceDoneCallback: raceDoneCallback,
		stateInvalid:     &RaceInvalid{createLogger("invalid")},
		stateRun:         &RaceRun{createLogger("run")},
		stateFinishing:   &RaceFinishing{createLogger("finishing")},
		stateCooldown:    &RaceCooldown{createLogger("cooldown")},
		stateDone:        &RaceDone{createLogger("done")},
	}
	ret.currentState = ret.stateInvalid
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
