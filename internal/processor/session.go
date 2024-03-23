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
		"sessionStateRaw",
		"sessionFlagsRaw",
		"timeOfDay",
		"airTemp",
		"airDensity",
		"airPressure",
		"trackTemp",
		"windDir",
		"windVel",
		"trackWetness",
		"weatherDeclaredWet",
		"precipitation",
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
	msg["precipitation"] = justValue(s.api.GetValue("Precipitation"))
	msg["trackWetness"] = justValue(s.api.GetValue("TrackWetness"))
	msg["weatherDeclaredWet"] = justValue(s.api.GetValue("WeatherDeclardWet"))
	state, _ := s.api.GetIntValue("SessionState")
	flags, _ := s.api.GetIntValue("SessionFlags")
	msg["flagState"] = computeFlagState(state, int64(flags))
	msg["sessionStateRaw"] = justValue(s.api.GetValue("SessionState"))
	msg["sessionFlagsRaw"] = justValue(s.api.GetValue("SessionFlags"))

	return msg
}
