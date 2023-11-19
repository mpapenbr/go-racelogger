package processor

import (
	"github.com/mpapenbr/goirsdk/irsdk"
)

func SessionManifest() []string {
	return []string{
		"sessionNum",
		"sessionTime",
		"timeRemain",
		"lapsRemain",
		"flagState",
		"timeOfDay",
		"airTemp",
		"airDensity",
		"airPressure",
		"trackTemp",
		"windDir",
		"windVel",
	}
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
