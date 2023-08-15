package processor

import (
	"reflect"
	"time"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
)

type GenericMessage map[string]interface{}
type GlobalProcessingData struct {
	TrackInfo     model.TrackInfo
	EventDataInfo model.EventDataInfo
}

type Options struct {
	StatePublishInterval    time.Duration
	SpeedmapPublishInterval time.Duration
	CarDataPublishInterval  time.Duration
	ChunkSize               int // speedmap chunk size
	GlobalProcessingData    GlobalProcessingData
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

type Processor struct {
	api                  *irsdk.Irsdk
	options              *Options
	lastTimeSendState    time.Time
	lastTimeSendSpeedmap time.Time
	lastTimeSendCardata  time.Time
	sessionProc          SessionProc
	carProc              CarProc
	speedmapProc         SpeedmapProc
	carDriverProc        CarDriverProc
	raceProc             RaceProc
	lastDriverInfo       yaml.DriverInfo
	stateOutput          chan model.StateData
	speedmapOutput       chan model.SpeedmapData
	carDataOutput        chan model.CarData
}

func NewProcessor(
	api *irsdk.Irsdk,
	stateOutput chan model.StateData,
	speedmapOutput chan model.SpeedmapData,
	cardataOutput chan model.CarData,
	options ...OptionsFunc) *Processor {

	opts := defaultOptions()
	for _, o := range options {
		o(opts)
	}

	carDriverProc := CarDriverProc{api: api, output: cardataOutput}
	carProc := CarProc{api: api}
	raceProc := RaceProc{api: api, carProc: &carProc}
	return &Processor{
		api:               api,
		options:           opts,
		lastTimeSendState: time.Time{},
		stateOutput:       stateOutput,
		speedmapOutput:    speedmapOutput,
		carDataOutput:     cardataOutput,
		carProc:           carProc,
		raceProc:          raceProc,
		sessionProc:       SessionProc{api: api},
		speedmapProc:      SpeedmapProc{api: api, chunkSize: opts.ChunkSize, gpd: &opts.GlobalProcessingData},
		carDriverProc:     carDriverProc,
	}
}

func (p *Processor) Process() {
	p.raceProc.Process()
	p.speedmapProc.Process()
	y, _ := p.api.GetYaml()
	if !reflect.DeepEqual(y.DriverInfo, p.lastDriverInfo) {
		log.Info("DriverInfo changed, updating state")
		p.lastDriverInfo = reflect.ValueOf(y.DriverInfo).Interface().(yaml.DriverInfo)
		p.carDriverProc.Process(y)
	}

	p.handleStateMessage()
	p.handleSpeedmapMessage()
	// p.handleCarDataMessage()
}

func (p *Processor) handleSpeedmapMessage() {
	if time.Now().After(p.lastTimeSendSpeedmap.Add(p.options.SpeedmapPublishInterval)) {
		data := model.SpeedmapData{
			Type:      int(model.MTSpeedmap),
			Timestamp: float64(time.Now().UnixMilli()),
			Payload: model.SpeedmapPayload{
				Data:      map[string]*model.ClassSpeedmapData{},
				ChunkSize: 10,
			},
		}
		log.Debug("About to send new speedmap data", log.Any("msg", data))
		p.speedmapOutput <- data
		p.lastTimeSendSpeedmap = time.Now()
	}
}

func (p *Processor) handleCarDataMessage() {
	if time.Now().After(p.lastTimeSendCardata.Add(p.options.CarDataPublishInterval)) {
		data := model.CarData{
			Type:      int(model.MTCar),
			Timestamp: float64(time.Now().UnixMilli()),
			Payload: model.CarPayload{
				Cars:       []model.CarInfo{},
				CarClasses: []model.CarClass{},
				Entries:    []model.CarEntry{},
			},
		}
		log.Debug("About to send new car data", log.Any("msg", data))
		p.carDataOutput <- data
		p.lastTimeSendCardata = time.Now()
	}
}

func (p *Processor) handleStateMessage() {
	if time.Now().After(p.lastTimeSendState.Add(p.options.StatePublishInterval)) {
		data := model.StateData{
			Type:      int(model.MTState),
			Timestamp: float64(time.Now().UnixMilli()),
			Payload: model.StatePayload{
				Session: p.sessionProc.CreatePayload(),
				Cars:    p.carProc.CreatePayload(),
				// Cars:     [][]interface{}{},
				Messages: []interface{}{},
			},
		}
		log.Debug("About to send new state data", log.Any("msg", data))
		p.stateOutput <- data
		p.lastTimeSendState = time.Now()
	}
}
