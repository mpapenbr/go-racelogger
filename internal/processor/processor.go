package processor

import (
	"reflect"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/mpapenbr/goirsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	goyaml "gopkg.in/yaml.v3"
)

type (
	GenericMessage       map[string]interface{}
	GlobalProcessingData struct {
		TrackInfo     model.TrackInfo
		EventDataInfo model.EventDataInfo
	}
)

type Options struct {
	StatePublishInterval    time.Duration
	SpeedmapPublishInterval time.Duration
	CarDataPublishInterval  time.Duration
	ChunkSize               int // speedmap chunk size
	GlobalProcessingData    GlobalProcessingData
	RecordingDoneChannel    chan struct{}
}

func defaultOptions() *Options {
	return &Options{
		StatePublishInterval:    1 * time.Second,
		SpeedmapPublishInterval: 30 * time.Second,
		CarDataPublishInterval:  1 * time.Second,
	}
}

// functional options pattern for Options
type OptionsFunc func(*Options)

func WithStatePublishInterval(d time.Duration) OptionsFunc {
	return func(o *Options) {
		o.StatePublishInterval = d
	}
}

func WithSpeedmapPublishInterval(d time.Duration) OptionsFunc {
	return func(o *Options) {
		o.SpeedmapPublishInterval = d
	}
}

func WithCarDataPublishInterval(d time.Duration) OptionsFunc {
	return func(o *Options) {
		o.CarDataPublishInterval = d
	}
}

func WithChunkSize(i int) OptionsFunc {
	return func(o *Options) {
		o.ChunkSize = i
	}
}

func WithGlobalProcessingData(gpd GlobalProcessingData) OptionsFunc {
	return func(o *Options) {
		o.GlobalProcessingData = gpd
	}
}

func WithRecordingDoneChannel(c chan struct{}) OptionsFunc {
	return func(o *Options) {
		o.RecordingDoneChannel = c
	}
}

type Processor struct {
	api                  *irsdk.Irsdk
	options              *Options
	lastTimeSendState    time.Time
	lastTimeSendSpeedmap time.Time
	lastTimeSendCardata  time.Time
	sessionProc          SessionProc
	carProc              *CarProc
	speedmapProc         *SpeedmapProc
	carDriverProc        *CarDriverProc
	raceProc             *RaceProc
	messageProc          *MessageProc
	pitBoundaryProc      *PitBoundaryProc
	lastDriverInfo       yaml.DriverInfo
	stateOutput          chan model.StateData
	speedmapOutput       chan model.SpeedmapData
	extraInfoOutput      chan model.ExtraInfo

	recordingDoneChannel chan struct{}
}

func NewProcessor(
	api *irsdk.Irsdk,
	stateOutput chan model.StateData,
	speedmapOutput chan model.SpeedmapData,
	cardataOutput chan model.CarData,
	extraInfoOutput chan model.ExtraInfo,
	options ...OptionsFunc,
) *Processor {
	opts := defaultOptions()
	for _, o := range options {
		o(opts)
	}

	pitBoundaryProc := NewPitBoundaryProc()
	carDriverProc := NewCarDriverProc(api, cardataOutput)
	messageProc := NewMessageProc(carDriverProc)
	speedmapProc := NewSpeedmapProc(api, opts.ChunkSize, &opts.GlobalProcessingData)
	carProc := NewCarProc(api, &opts.GlobalProcessingData, carDriverProc, pitBoundaryProc, speedmapProc)
	raceProc := NewRaceProc(api,
		carProc,
		messageProc,
		nil)
	ret := Processor{
		api:               api,
		options:           opts,
		lastTimeSendState: time.Time{},
		stateOutput:       stateOutput,
		speedmapOutput:    speedmapOutput,
		extraInfoOutput:   extraInfoOutput,
		messageProc:       messageProc,
		carProc:           carProc,
		raceProc:          raceProc,
		sessionProc:       SessionProc{api: api},
		speedmapProc:      speedmapProc,
		carDriverProc:     carDriverProc,
		pitBoundaryProc:   pitBoundaryProc,
	}
	ret.init()
	return &ret
}

func (p *Processor) init() {
	p.raceProc.RaceDoneCallback = func() {
		p.sendSpeedmapMessage()
		p.sendStateMessage()
		// if enough data was collected, send it to server
		if p.pitBoundaryProc.pitEntry.computed && p.pitBoundaryProc.pitExit.computed {
			log.Info("Pit entry and exit computed during session, sending to server")
			pitLaneLength := func(entry, exit float64) float64 {
				if exit > entry {
					return (exit - entry) * p.options.GlobalProcessingData.TrackInfo.Length
				} else {
					return (1.0 - entry + exit) * p.options.GlobalProcessingData.TrackInfo.Length
				}
			}
			pitInfo := model.PitInfo{
				Entry: p.pitBoundaryProc.pitEntry.middle,
				Exit:  p.pitBoundaryProc.pitExit.middle,
				LaneLength: pitLaneLength(
					p.pitBoundaryProc.pitEntry.middle, p.pitBoundaryProc.pitExit.middle),
			}
			p.raceProc.carProc.gpd.TrackInfo.Pit = &pitInfo
			p.extraInfoOutput <- model.ExtraInfo{Track: p.raceProc.carProc.gpd.TrackInfo}
			time.Sleep(1 * time.Second) // wait a little to get message transmitted
		}
		if p.options.RecordingDoneChannel != nil {
			log.Debug("Signaling recording done")
			close(p.options.RecordingDoneChannel)
		}
	}
}

func (p *Processor) Process() {
	y := p.api.GetLatestYaml()
	p.raceProc.Process()

	if !reflect.DeepEqual(y.DriverInfo, p.lastDriverInfo) &&
		!cmp.Equal(y.DriverInfo, p.lastDriverInfo) {

		log.Info("DriverInfo changed, updating state")
		var freshYaml yaml.IrsdkYaml
		if err := goyaml.Unmarshal([]byte(p.api.GetYamlString()), &freshYaml); err != nil {
			log.Error("Error unmarshalling irsdk yaml", log.ErrorField(err))
			return
		}
		// fmt.Printf("Delta: %v\n", cmp.Diff(y.DriverInfo, p.lastDriverInfo))
		// p.lastDriverInfo = reflect.ValueOf(y.DriverInfo).Interface().(yaml.DriverInfo)
		p.lastDriverInfo = freshYaml.DriverInfo
		p.carDriverProc.Process(&freshYaml)
	}

	if time.Now().After(p.lastTimeSendState.Add(p.options.StatePublishInterval)) {
		p.sendStateMessage()
	}
	if time.Now().After(p.lastTimeSendSpeedmap.Add(p.options.SpeedmapPublishInterval)) {
		p.sendSpeedmapMessage()
	}
}

func (p *Processor) sendSpeedmapMessage() {
	data := model.SpeedmapData{
		Type:      int(model.MTSpeedmap),
		Timestamp: float64Timestamp(time.Now()),
		Payload:   p.speedmapProc.CreatePayload(),
	}

	// log.Debug("About to send new speedmap data", log.Any("msg", data))
	p.speedmapOutput <- data
	p.lastTimeSendSpeedmap = time.Now()
}

func (p *Processor) sendStateMessage() {
	data := model.StateData{
		Type:      int(model.MTState),
		Timestamp: float64Timestamp(time.Now()),
		Payload: model.StatePayload{
			Session:  p.sessionProc.CreatePayload(),
			Cars:     p.carProc.CreatePayload(),
			Messages: p.messageProc.CreatePayload(),
		},
	}
	// log.Debug("About to send new state data", log.Any("msg", data))
	// log.Debug("CarProc data", log.Any("prevLapDistPct", p.carProc.prevLapDistPct))
	p.stateOutput <- data
	p.lastTimeSendState = time.Now()
	p.messageProc.Clear()
}
