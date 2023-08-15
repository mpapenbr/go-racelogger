package processor

import (
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
)

// SpeedmapProc is a struct that contains the logic to process the speedmap data.
// It is used by the Processor struct.

type SpeedmapProc struct {
	api             *irsdk.Irsdk
	chunkSize       int
	gpd             *GlobalProcessingData
	lastSessionTime float64
}

func NewSpeedmapProcessor(api *irsdk.Irsdk, chunkSize int, gpd *GlobalProcessingData) *SpeedmapProc {
	return &SpeedmapProc{api: api, chunkSize: chunkSize, gpd: gpd}
}

func (s *SpeedmapProc) Process() {
	// do nothing
	// currentTick := justValue(s.api.GetIntValue("SessionTick"))
	currentTime := justValue(s.api.GetDoubleValue("SessionTime")).(float64)

	// check if we have valid data, otherwise return
	if currentTime < 0 || currentTime <= s.lastSessionTime {
		return
	}
	// check if a race session is ongoing
	if !shouldRecord(s.api) {
		return
	}

}

func (s *SpeedmapProc) CreatePayload() model.SpeedmapPayload {
	content := s.CreateOutput()
	ret := model.SpeedmapPayload{
		ChunkSize:   s.chunkSize,
		Data:        content,
		SessionTime: justValue(s.api.GetValue("SessionTime")).(float64),
		TimeOfDay:   justValue(s.api.GetValue("TimeOfDay")).(float64),
		TrackTemp:   justValue(s.api.GetValue("TrackTemp")).(float64),
		TrackLength: s.gpd.TrackInfo.Length,
	}
	return ret
}

func (s *SpeedmapProc) CreateOutput() map[string]*model.ClassSpeedmapData {
	return map[string]*model.ClassSpeedmapData{}
}
