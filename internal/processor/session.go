package processor

import (
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	"github.com/mpapenbr/goirsdk/irsdk"
)

type SessionProc struct {
	api *irsdk.Irsdk
}

func NewSessionProc(api *irsdk.Irsdk) *SessionProc {
	return &SessionProc{api: api}
}

func (s *SessionProc) CreatePayload() *racestatev1.Session {
	ret := &racestatev1.Session{
		SessionNum:    readUint32(s.api, "SessionNum"),
		SessionTime:   float32(readFloat64(s.api, "SessionTime")),
		TimeRemain:    float32(readFloat64(s.api, "SessionTimeRemain")),
		LapsRemain:    readInt32(s.api, "SessionLapsRemainEx"),
		TimeOfDay:     uint32(readFloat32(s.api, "SessionTimeOfDay")),
		AirTemp:       readFloat32(s.api, "AirTemp"),
		AirDensity:    readFloat32(s.api, "AirDensity"),
		AirPressure:   readFloat32(s.api, "AirPressure"),
		TrackTemp:     readFloat32(s.api, "TrackTempCrew"),
		WindDir:       readFloat32(s.api, "WindDir"),
		WindVel:       readFloat32(s.api, "WindVel"),
		TrackWetness:  convertTrackWetness(s.api),
		Precipitation: readFloat32(s.api, "Precipitation"),
		FlagState: computeFlagState(
			readInt32(s.api, "SessionState"),
			int64(readUint32(s.api, "SessionFlags")),
		),
		SessionStateRaw: readInt32(s.api, "SessionState"),
		SessionFlagsRaw: readUint32(s.api, "SessionFlags"),
	}
	return ret
}
