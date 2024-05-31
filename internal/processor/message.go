package processor

import (
	"fmt"

	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"

	"github.com/mpapenbr/go-racelogger/log"
)

func MessageManifest() []string {
	return []string{"type", "subType", "carIdx", "carNum", "carClass", "msg"}
}

type MessageProc struct {
	carDriverProc *CarDriverProc
	bufferGen     []GenericMessage
	buffer        []*racestatev1.Message
}

func NewMessageProc(carDriverProc *CarDriverProc) *MessageProc {
	ret := &MessageProc{carDriverProc: carDriverProc}
	ret.init()
	return ret
}

func (p *MessageProc) init() {
	p.bufferGen = make([]GenericMessage, 0)
	p.buffer = make([]*racestatev1.Message, 0)
}

func (p *MessageProc) Clear() {
	p.bufferGen = make([]GenericMessage, 0)
	p.buffer = make([]*racestatev1.Message, 0)
}

func (p *MessageProc) DriverEnteredCar(carIdx int) {
	log.Debug("Driver entered car", log.Int("carIdx", carIdx))
	p.buffer = append(p.buffer, &racestatev1.Message{
		Type:     racestatev1.MessageType_MESSAGE_TYPE_PITS,
		SubType:  racestatev1.MessageSubType_MESSAGE_SUB_TYPE_DRIVER,
		CarIdx:   uint32(carIdx),
		CarNum:   p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
		CarClass: p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarClassShortName,
		Msg: fmt.Sprintf("#%s %s entered the car",
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).UserName,
		),
	})
}

func (p *MessageProc) ReportDriverLap(carIdx int, twm TimeWithMarker) {
	log.Debug("Report driver lap", log.Int("carIdx", carIdx))
	p.buffer = append(p.buffer, &racestatev1.Message{
		Type:     racestatev1.MessageType_MESSAGE_TYPE_TIMING,
		SubType:  racestatev1.MessageSubType_MESSAGE_SUB_TYPE_DRIVER,
		CarIdx:   uint32(carIdx),
		CarNum:   p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
		CarClass: p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarClassShortName,
		Msg: fmt.Sprintf("#%s (%s) new %s",
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).CarNumber,
			p.carDriverProc.GetCurrentDriver(int32(carIdx)).UserName,
			twm.String(),
		),
	})
}

func (p *MessageProc) RaceStarts() {
	p.buffer = append(p.buffer, &racestatev1.Message{
		Type:    racestatev1.MessageType_MESSAGE_TYPE_TIMING,
		SubType: racestatev1.MessageSubType_MESSAGE_SUB_TYPE_RACE_CONTROL,
		Msg:     "Race start",
	})
}

func (p *MessageProc) CheckeredFlagIssued() {
	p.buffer = append(p.buffer, &racestatev1.Message{
		Type:    racestatev1.MessageType_MESSAGE_TYPE_TIMING,
		SubType: racestatev1.MessageSubType_MESSAGE_SUB_TYPE_RACE_CONTROL,
		Msg:     "Checkered start",
	})
}

func (p *MessageProc) RecordingDone() {
	p.buffer = append(p.buffer, &racestatev1.Message{
		Type:    racestatev1.MessageType_MESSAGE_TYPE_TIMING,
		SubType: racestatev1.MessageSubType_MESSAGE_SUB_TYPE_RACE_CONTROL,
		Msg:     "End of recording",
	})
}

func (p *MessageProc) CreatePayloadWamp() [][]interface{} {
	payload := make([][]interface{}, len(p.bufferGen))
	manifest := MessageManifest()
	createMessage := func(msgData GenericMessage) []interface{} {
		ret := make([]interface{}, len(manifest))

		for idx, attr := range manifest {
			ret[idx] = msgData[attr]
		}
		return ret
	}
	for i, c := range p.bufferGen {
		payload[i] = createMessage(c)
	}
	return payload
}

func (p *MessageProc) CreatePayload() []*racestatev1.Message {
	return p.buffer
}
