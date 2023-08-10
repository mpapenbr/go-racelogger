package processor

import (
	"time"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
)

type GenericMessage map[string]interface{}

type Processor struct {
	api               *irsdk.Irsdk
	lastTimeSendState time.Time
	sessionProc       SessionProc
	output            chan model.StateData
}

func NewProcessor(api *irsdk.Irsdk, output chan model.StateData) *Processor {
	return &Processor{
		api:               api,
		lastTimeSendState: time.Time{},
		output:            output,
		sessionProc:       SessionProc{api: api},
	}
}

func (p *Processor) Process() {
	p.handleStateMessage()
	p.handleSpeedmapMessage()
	p.handleCarDataMessage()
}

func (p *Processor) handleSpeedmapMessage() {
}
func (p *Processor) handleCarDataMessage() {
}

func (p *Processor) handleStateMessage() {
	if time.Now().After(p.lastTimeSendState.Add(time.Second)) {
		data := model.StateData{
			Type:      int(model.MTState),
			Timestamp: float64(time.Now().UnixMilli()),
			Payload: model.StatePayload{
				Session:  p.sessionProc.CreatePayload(),
				Cars:     [][]interface{}{},
				Messages: []interface{}{},
			},
		}
		log.Debug("About to send new state data", log.Any("msg", data))
		p.output <- data
		p.lastTimeSendState = time.Now()
	}
}
