package processor

import (
	"time"

	"github.com/mpapenbr/goirsdk/irsdk"
	iryaml "github.com/mpapenbr/goirsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	"gopkg.in/yaml.v3"

	"github.com/mpapenbr/go-racelogger/log"
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
	ChunkSize               int     // speedmap chunk size
	SpeedmapSpeedThreshold  float64 // speedmap speed threshold
	MaxSpeed                float64 // speeds above this value (km/h) are not processed
	GlobalProcessingData    *GlobalProcessingData
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

func WithSpeedmapSpeedThreshold(f float64) OptionsFunc {
	return func(o *Options) {
		o.SpeedmapSpeedThreshold = f
	}
}

func WithMaxSpeed(f float64) OptionsFunc {
	return func(o *Options) {
		o.MaxSpeed = f
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

func WithGlobalProcessingData(gpd *GlobalProcessingData) OptionsFunc {
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
	sessionProc          SessionProc
	carProc              *CarProc
	speedmapProc         *SpeedmapProc
	carDriverProc        *CarDriverProc
	raceProc             *RaceProc
	messageProc          *MessageProc
	pitBoundaryProc      *PitBoundaryProc
	lastDriverInfo       iryaml.DriverInfo
	stateOutput          chan model.StateData
	speedmapOutput       chan model.SpeedmapData
	extraInfoOutput      chan model.ExtraInfo
}

//nolint:whitespace,funlen // can't get different linters happy
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
	SetSpeedmapSpeedThreshold(opts.SpeedmapSpeedThreshold)
	pitBoundaryProc := NewPitBoundaryProc()
	carDriverProc := NewCarDriverProc(api, cardataOutput)
	messageProc := NewMessageProc(carDriverProc)
	carDriverProc.SetReportChangeFunc(messageProc.DriverEnteredCar)
	speedmapProc := NewSpeedmapProc(api, opts.ChunkSize, opts.GlobalProcessingData)
	carProc := NewCarProc(
		api,
		opts.GlobalProcessingData,
		carDriverProc,
		pitBoundaryProc,
		speedmapProc,
		messageProc,
		opts.MaxSpeed,
	)
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
		}
		time.Sleep(1 * time.Second) // wait a little to get outstandig messages transmitted
		if p.options.RecordingDoneChannel != nil {
			log.Debug("Signaling recording done")
			close(p.options.RecordingDoneChannel)
		}
	}
}

func (p *Processor) Process() {
	y := p.api.GetLatestYaml()
	p.raceProc.Process()

	if HasDriverChange(&y.DriverInfo, &p.lastDriverInfo) {
		log.Info("DriverInfo changed, updating state")
		var freshYaml iryaml.IrsdkYaml
		if err := yaml.Unmarshal([]byte(p.api.GetYamlString()), &freshYaml); err != nil {
			// let's try to repair the yaml and unmarshal again
			err := yaml.Unmarshal([]byte(p.api.RepairedYaml(p.api.GetYamlString())),
				&freshYaml)
			if err != nil {
				log.Error("Error unmarshalling irsdk yaml", log.ErrorField(err))
				return
			}
		}

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
	p.stateOutput <- data
	p.lastTimeSendState = time.Now()
	p.messageProc.Clear()
}
