package processor

import (
	"time"

	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	eventv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/event/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	trackv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/track/v1"
	"github.com/mpapenbr/goirsdk/irsdk"
	iryaml "github.com/mpapenbr/goirsdk/yaml"
	"google.golang.org/protobuf/types/known/timestamppb"
	"gopkg.in/yaml.v3"

	"github.com/mpapenbr/go-racelogger/log"
)

type (
	GenericMessage       map[string]interface{}
	GlobalProcessingData struct {
		TrackInfo     *trackv1.Track
		EventDataInfo *eventv1.Event
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
	stateOutput          chan *racestatev1.PublishStateRequest
	speedmapOutput       chan *racestatev1.PublishSpeedmapRequest
	extraInfoOutput      chan *racestatev1.PublishEventExtraInfoRequest
	recording            bool
	racing               bool
}

//nolint:whitespace,funlen // can't get different linters happy
func NewProcessor(
	api *irsdk.Irsdk,
	stateOutput chan *racestatev1.PublishStateRequest,
	speedmapOutput chan *racestatev1.PublishSpeedmapRequest,
	cardataOutput chan *racestatev1.PublishDriverDataRequest,
	extraInfoOutput chan *racestatev1.PublishEventExtraInfoRequest,
	options ...OptionsFunc,
) *Processor {
	opts := defaultOptions()
	for _, o := range options {
		o(opts)
	}
	SetSpeedmapSpeedThreshold(opts.SpeedmapSpeedThreshold)
	pitBoundaryProc := NewPitBoundaryProc()
	carDriverProc := NewCarDriverProc(api, cardataOutput, opts.GlobalProcessingData)
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
		api:                  api,
		options:              opts,
		lastTimeSendState:    time.Time{},
		lastTimeSendSpeedmap: time.Now(),
		stateOutput:          stateOutput,
		speedmapOutput:       speedmapOutput,
		extraInfoOutput:      extraInfoOutput,
		messageProc:          messageProc,
		carProc:              carProc,
		raceProc:             raceProc,
		sessionProc:          SessionProc{api: api},
		speedmapProc:         speedmapProc,
		carDriverProc:        carDriverProc,
		pitBoundaryProc:      pitBoundaryProc,
		recording:            true,
		racing:               false,
	}
	ret.init()
	return &ret
}

func (p *Processor) init() {
	p.raceProc.RaceRunCallback = func() {
		p.racing = true
		p.lastTimeSendSpeedmap = time.Now().Add(p.options.SpeedmapPublishInterval)
	}
	p.raceProc.RaceDoneCallback = func() {
		p.sendSpeedmapMessage()
		p.sendStateMessage()
		// if enough data was collected, send it to server
		if p.pitBoundaryProc.pitEntry.computed && p.pitBoundaryProc.pitExit.computed {
			log.Info("Pit entry and exit computed during session, sending to server")
			pitLaneLength := func(entry, exit float32) float32 {
				if exit > entry {
					return (exit - entry) * p.options.GlobalProcessingData.TrackInfo.Length
				} else {
					return (1.0 - entry + exit) * p.options.GlobalProcessingData.TrackInfo.Length
				}
			}
			pitInfo := trackv1.PitInfo{
				Entry: float32(p.pitBoundaryProc.pitEntry.middle),
				Exit:  float32(p.pitBoundaryProc.pitExit.middle),
				LaneLength: pitLaneLength(
					float32(p.pitBoundaryProc.pitEntry.middle),
					float32(p.pitBoundaryProc.pitExit.middle)),
			}
			p.raceProc.carProc.gpd.TrackInfo.PitInfo = &pitInfo

			msg := racestatev1.PublishEventExtraInfoRequest{
				Event: &commonv1.EventSelector{
					Arg: &commonv1.EventSelector_Key{
						Key: p.options.GlobalProcessingData.EventDataInfo.Key,
					},
				},
				Timestamp: timestamppb.Now(),
				ExtraInfo: &racestatev1.ExtraInfo{PitInfo: &pitInfo},
			}
			p.extraInfoOutput <- &msg
		}
		time.Sleep(1 * time.Second) // wait a little to get outstandig messages transmitted
		p.recording = false         // signal recording done
		p.racing = false            // signal racing done
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

	if p.recording &&
		time.Now().After(p.lastTimeSendState.Add(p.options.StatePublishInterval)) {

		p.sendStateMessage()
	}

	if p.recording && p.racing &&
		time.Now().After(p.lastTimeSendSpeedmap.Add(p.options.SpeedmapPublishInterval)) {

		p.sendSpeedmapMessage()
	}
}

func (p *Processor) sendSpeedmapMessage() {
	msg := racestatev1.PublishSpeedmapRequest{
		Event: &commonv1.EventSelector{
			Arg: &commonv1.EventSelector_Key{
				Key: p.options.GlobalProcessingData.EventDataInfo.Key,
			},
		},
		Speedmap:  p.speedmapProc.CreatePayload(),
		Timestamp: timestamppb.Now(),
	}

	p.speedmapOutput <- &msg
	p.lastTimeSendSpeedmap = time.Now()
}

func (p *Processor) sendStateMessage() {
	msg := racestatev1.PublishStateRequest{
		Event: &commonv1.EventSelector{
			Arg: &commonv1.EventSelector_Key{
				Key: p.options.GlobalProcessingData.EventDataInfo.Key,
			},
		},
		Cars:      p.carProc.CreatePayload(),
		Session:   p.sessionProc.CreatePayload(),
		Messages:  p.messageProc.CreatePayload(),
		Timestamp: timestamppb.Now(),
	}

	p.stateOutput <- &msg
	p.lastTimeSendState = time.Now()
	p.messageProc.Clear()
}
