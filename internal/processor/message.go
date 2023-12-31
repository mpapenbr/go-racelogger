package processor

import (
	"fmt"

	"github.com/mpapenbr/go-racelogger/log"
)

func MessageManifest() []string {
	return []string{"type", "subType", "carIdx", "carNum", "carClass", "msg"}
}

type MessageProc struct {
	carDriverProc *CarDriverProc
	buffer        []GenericMessage
}

func NewMessageProc(carDriverProc *CarDriverProc) *MessageProc {
	ret := &MessageProc{carDriverProc: carDriverProc}
	ret.init()
	return ret
}

func (p *MessageProc) init() {
	p.buffer = make([]GenericMessage, 0)
}

func (p *MessageProc) Clear() {
	p.buffer = make([]GenericMessage, 0)
}

func (p *MessageProc) DriverEnteredCar(carIdx int) {
	log.Debug("Driver entered car", log.Int("carIdx", carIdx))
	p.buffer = append(p.buffer, GenericMessage{
		"type":     "Pits",
		"subType":  "Driver",
		"carIdx":   carIdx,
		"carNum":   p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
		"carClass": p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarClassShortName,
		"msg": fmt.Sprintf("#%s %s entered the car",
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).UserName,
		),
	})
}

func (p *MessageProc) ReportDriverLap(carIdx int, twm TimeWithMarker) {
	log.Debug("Report driver lap", log.Int("carIdx", carIdx))
	p.buffer = append(p.buffer, GenericMessage{
		"type":     "Timing",
		"subType":  "Driver",
		"carIdx":   carIdx,
		"carNum":   p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
		"carClass": p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarClassShortName,
		"msg": fmt.Sprintf("#%s (%s) new %s",
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).UserName,
			twm.String(),
		),
	})
}

func (p *MessageProc) RaceStarts() {
	p.buffer = append(p.buffer, GenericMessage{
		"type":     "Timing",
		"subType":  "RaceControl",
		"carIdx":   nil,
		"carNum":   nil,
		"carClass": nil,
		"msg":      "Race start",
	})
}

func (p *MessageProc) CheckeredFlagIssued() {
	p.buffer = append(p.buffer, GenericMessage{
		"type":     "Timing",
		"subType":  "RaceControl",
		"carIdx":   nil,
		"carNum":   nil,
		"carClass": nil,
		"msg":      "Checkered flag",
	})
}

func (p *MessageProc) RecordingDone() {
	p.buffer = append(p.buffer, GenericMessage{
		"type":     "Timing",
		"subType":  "RaceControl",
		"carIdx":   nil,
		"carNum":   nil,
		"carClass": nil,
		"msg":      "End of recording",
	})
}

func (p *MessageProc) CreatePayload() [][]interface{} {
	payload := make([][]interface{}, len(p.buffer))
	manifest := MessageManifest()
	createMessage := func(msgData GenericMessage) []interface{} {
		ret := make([]interface{}, len(manifest))

		for idx, attr := range manifest {
			ret[idx] = msgData[attr]
		}
		return ret
	}
	for i, c := range p.buffer {
		payload[i] = createMessage(c)
	}
	return payload
}
