package processor

import (
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
)

func SessionManifest() []string {
	return []string{"sessionNum", "sessionTime", "timeRemain", "lapsRemain", "flagState", "timeOfDay", "airTemp", "airDensity", "airPressure", "trackTemp", "windDir", "windVel"}
}

type SessionProc struct {
	api *irsdk.Irsdk
}

func NewSessionProc(api *irsdk.Irsdk) *SessionProc {

	return &SessionProc{api: api}
}

func (s *SessionProc) CreatePayload() []interface{} {
	content := s.CreateOutput()
	ret := make([]interface{}, len(SessionManifest()))
	for idx, key := range SessionManifest() {
		ret[idx] = content[key]
	}
	return ret
}
func (s *SessionProc) CreateOutput() GenericMessage {

	msg := GenericMessage{}
	msg["sessionNum"] = justValue(s.api.GetValue("SessionNum"))
	msg["sessionTime"] = justValue(s.api.GetValue("SessionTime"))
	msg["timeRemain"] = justValue(s.api.GetValue("SessionTimeRemain"))
	msg["lapsRemain"] = justValue(s.api.GetValue("SessionLapsRemainEx"))
	msg["timeOfDay"] = justValue(s.api.GetValue("SessionTimeOfDay"))
	msg["airTemp"] = justValue(s.api.GetValue("AirTemp"))
	msg["airDensity"] = justValue(s.api.GetValue("AirDensity"))
	msg["airPressure"] = justValue(s.api.GetValue("AirPressure"))
	msg["trackTemp"] = justValue(s.api.GetValue("TrackTemp"))
	msg["windDir"] = justValue(s.api.GetValue("WindDir"))
	msg["windVel"] = justValue(s.api.GetValue("WindVel"))
	state, _ := s.api.GetIntValue("SessionState")
	flags, _ := s.api.GetIntValue("SessionFlags")
	msg["flagState"] = computeFlagState(state, int64(flags))
	return msg

}

func computeFlagState(state int32, flags int64) string {
	if state == int32(irsdk.StateRacing) {
		if flags&int64(irsdk.FlagStartHidden) == int64(irsdk.FlagStartHidden) {
			return "GREEN"
		} else if flags>>16&int64(irsdk.FlagGreen) == int64(irsdk.FlagGreen) {
			return "GREEN"
		} else if flags>>16&int64(irsdk.FlagYello) == int64(irsdk.FlagYello) {
			return "YELLOW"
		} else if flags>>16&int64(irsdk.FlagCheckered) == int64(irsdk.FlagCheckered) {
			return "CHECKERED"
		} else if flags>>16&int64(irsdk.FlagWhite) == int64(irsdk.FlagWhite) {
			return "WHITE"
		}
	} else if state == int32(irsdk.StateCheckered) {
		return "CHECKERED"
	} else if state == int32(irsdk.StateCoolDown) {
		return "CHECKERED"
	} else if state == int32(irsdk.StateGetInCar) {
		return "PREP"
	} else if state == int32(irsdk.StateParadeLaps) {
		return "PARADE"
	} else if state == int32(irsdk.StateInvalid) {
		return "INVALID"
	}
	return "NONE"

}
