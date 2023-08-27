package internal

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"reflect"
	"time"

	"github.com/google/uuid"

	"github.com/mpapenbr/go-racelogger/internal/processor"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"github.com/mpapenbr/go-racelogger/pkg/wamp"
	"github.com/mpapenbr/go-racelogger/version"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/service"
	goyaml "gopkg.in/yaml.v3"
)

type EventKeyFunc func(*yaml.IrsdkYaml) string
type Config struct {
	ctx          context.Context
	cancel       context.CancelFunc
	eventKeyFunc EventKeyFunc
}
type ConfigFunc func(cfg *Config)

// Racelogger is the main component to control the connection to iRacing Telemetry API
type Racelogger struct {
	eventKey     string
	api          *irsdk.Irsdk
	dataprovider *wamp.DataProviderClient
	simIsRunning bool
	config       *Config
	globalData   processor.GlobalProcessingData
}

func defaultConfig() *Config {
	return &Config{
		eventKeyFunc: defaultEventKeyFunc,
	}
}

func defaultEventKeyFunc(irYaml *yaml.IrsdkYaml) string {
	out, err := goyaml.Marshal(irYaml.WeekendInfo)
	if err != nil {
		log.Warn("Could not marshal WeekendInfo", log.ErrorField(err))
		out = []byte(uuid.New().String())
	}
	h := md5.New()
	h.Write(out)
	ret := hex.EncodeToString(h.Sum(nil))
	return string(ret)
}
func WithContext(ctx context.Context, cancelFunc context.CancelFunc) ConfigFunc {
	return func(cfg *Config) { cfg.ctx = ctx; cfg.cancel = cancelFunc }
}

func NewRaceLogger(cfg ...ConfigFunc) *Racelogger {
	c := defaultConfig()
	for _, fn := range cfg {
		fn(c)
	}
	ret := &Racelogger{
		simIsRunning: false,
		dataprovider: wamp.NewDataProviderClient(config.URL, config.Realm, config.Password),
		config:       c,
	}

	ret.init()
	return ret
}

func (r *Racelogger) Close() {
	r.api.Close()
	r.dataprovider.Close()
}

func (r *Racelogger) RegisterProvider(eventName, eventDescription string) error {
	irYaml, err := r.api.GetYaml()
	if err != nil {
		return err
	}
	eventInfo, err := r.createEventInfo(irYaml)
	if err != nil {
		return err
	}

	trackInfo, err := r.createTrackInfo(irYaml)
	if err != nil {
		return err
	}
	if eventName != "" {
		eventInfo.Name = eventName
	} else {
		eventInfo.Name = fmt.Sprintf("%s %s", eventInfo.TrackDisplayName, eventInfo.EventTime)
	}
	if eventDescription != "" {
		eventInfo.Description = eventDescription
	}

	r.eventKey = r.config.eventKeyFunc(irYaml)
	r.globalData = processor.GlobalProcessingData{TrackInfo: *trackInfo, EventDataInfo: *eventInfo}

	req := service.RegisterEventRequest{
		EventInfo: *eventInfo,
		EventKey:  r.eventKey,
		TrackInfo: *trackInfo,
		Manifests: model.Manifests{
			Session: processor.SessionManifest(),
			Car:     processor.CarManifest(&r.globalData),
			Message: processor.MessageManifest(),
			// TODO: remove if go-analysis is available (now: compatibility Javascript routine)
			Pit: []string{},
		},
	}
	err = r.dataprovider.RegisterProvider(req)
	if err != nil {
		return err
	}
	r.setupDriverChangeDetector(time.Second)
	r.setupMainLoop()
	return nil
}

func (r *Racelogger) UnregisterProvider() {
	if err := r.dataprovider.UnregisterProvider(r.eventKey); err != nil {
		log.Warn("Could not unregister event", log.String("eventKey", r.eventKey), log.ErrorField(err))
	}
}

func (r *Racelogger) init() {
	r.setupWatchdog(time.Second)
	log.Debug("Ensure iRacing simulation is ready to provide data")
	for {
		if r.simIsRunning {
			break
		} else {
			log.Debug("Waiting for initialized simulation")
			time.Sleep(time.Second)
		}
	}
	log.Debug("Telemetry data is available")

}

func (r *Racelogger) createEventInfo(irYaml *yaml.IrsdkYaml) (*model.EventDataInfo, error) {

	pitSpeed, _ := processor.GetMetricUnit(irYaml.WeekendInfo.TrackPitSpeedLimit)
	trackLength, _ := processor.GetTrackLengthInMeters(irYaml.WeekendInfo.TrackLength)
	ret := model.EventDataInfo{
		TrackId:               irYaml.WeekendInfo.TrackID,
		TrackDisplayName:      irYaml.WeekendInfo.TrackDisplayName,
		TrackDisplayShortName: irYaml.WeekendInfo.TrackDisplayShortName,
		TrackConfigName:       irYaml.WeekendInfo.TrackConfigName,
		TrackPitSpeed:         pitSpeed,
		TrackLength:           trackLength,
		EventTime:             time.Now().Format("20060102-150405"),
		TeamRacing:            irYaml.WeekendInfo.TeamRacing,
		MultiClass:            irYaml.WeekendInfo.NumCarClasses > 1,
		NumCarTypes:           irYaml.WeekendInfo.NumCarTypes,
		NumCarClasses:         irYaml.WeekendInfo.NumCarClasses,
		IrSessionId:           irYaml.WeekendInfo.SessionID,
		Sectors:               r.convertSectors(irYaml.SplitTimeInfo.Sectors),
		Sessions:              r.convertSessions(irYaml.SessionInfo.Sessions),

		RaceloggerVersion: version.Version, // TODO: verify version setup by build
	}
	return &ret, nil
}

func (r *Racelogger) createTrackInfo(irYaml *yaml.IrsdkYaml) (*model.TrackInfo, error) {

	trackLength, _ := processor.GetTrackLengthInMeters(irYaml.WeekendInfo.TrackLength)
	ret := model.TrackInfo{
		ID:        irYaml.WeekendInfo.TrackID,
		Name:      irYaml.WeekendInfo.TrackDisplayName,
		ShortName: irYaml.WeekendInfo.TrackDisplayShortName,
		Config:    irYaml.WeekendInfo.TrackConfigName,
		Length:    trackLength,
		Sectors:   r.convertSectors(irYaml.SplitTimeInfo.Sectors),
	}
	return &ret, nil
}

func (r *Racelogger) convertSectors(sectors []yaml.Sectors) []model.Sector {
	ret := make([]model.Sector, len(sectors))
	for i, v := range sectors {
		ret[i] = model.Sector{SectorNum: v.SectorNum, SectorStartPct: v.SectorStartPct}
	}
	return ret
}

func (r *Racelogger) convertSessions(sectors []yaml.Sessions) []model.EventSession {
	ret := make([]model.EventSession, len(sectors))
	for i, v := range sectors {
		ret[i] = model.EventSession{Num: v.SessionNum, Name: v.SessionName}
	}
	return ret
}

func (r *Racelogger) setupWatchdog(interval time.Duration) {
	postData := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				log.Debug("watchdog recieved ctx.Done")
				return
			default:
				if irsdk.CheckIfSimIsRunning() {
					if r.api == nil {

						log.Debug("Initializing irsdk api")

						r.api = irsdk.NewIrsdk()
						r.api.WaitForValidData()
						// as long as there are no entries we have to try again
						for len(r.api.GetValueKeys()) == 0 {
							r.api.Close()
							log.Debug("iRacing not yet ready. Retrying in 5s")
							time.Sleep(5 * time.Second)
							r.api = irsdk.NewIrsdk()
							r.api.WaitForValidData()
						}
						r.api.GetData()
						r.simIsRunning = true
					}
				} else {
					if r.api != nil {
						log.Debug("Resetting irsdk api")
						r.api.Close()
					}
					r.api = nil
					r.simIsRunning = false
				}

				time.Sleep(time.Duration(interval))
			}
		}
	}

	go postData(r.config.ctx)
}

func (r *Racelogger) setupDriverChangeDetector(interval time.Duration) {
	lastDriverInfo := yaml.DriverInfo{DriverCarIdx: 12}
	postData := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				log.Debug("driverChangeDectector recieved ctx.Done")
				return
			default:
				if !r.simIsRunning {
					continue
				}
				r.api.GetData()

				if work, err := r.api.GetYaml(); err == nil {

					hasChanged := !reflect.DeepEqual(work.DriverInfo, lastDriverInfo)
					if hasChanged {
						log.Debug("DriverInfo have changed.")
						lastDriverInfo = work.DriverInfo
						data := make(map[string]interface{})
						data["changedDriverInfo"] = work.DriverInfo
						r.dataprovider.PublishDriverData(r.eventKey, &lastDriverInfo)
					}
				} else {
					fmt.Printf("Result of GetYaml(): %v\n", err)
				}

			}
			time.Sleep(time.Duration(interval))
		}
	}

	go postData(r.config.ctx)
}

func (r *Racelogger) setupMainLoop() {

	stateChannel := make(chan model.StateData, 2)
	speedmapChannel := make(chan model.SpeedmapData, 1)
	carDataChannel := make(chan model.CarData, 1)
	extraInfoChannel := make(chan model.ExtraInfo, 1)

	recordingDoneChannel := make(chan struct{}, 1)
	proc := processor.NewProcessor(
		r.api,
		stateChannel,
		speedmapChannel,
		carDataChannel,
		extraInfoChannel,
		processor.WithGlobalProcessingData(r.globalData),
		processor.WithChunkSize(10),
		processor.WithRecordingDoneChannel(recordingDoneChannel),
	)

	// sessionProc := processor.NewSessionProc(r.api, stateChannel)
	// r.processStateChannel(stateChannel)
	r.dataprovider.PublishStateFromChannel(r.eventKey, stateChannel)
	r.dataprovider.PublishSpeedmapDataFromChannel(r.eventKey, speedmapChannel)
	r.dataprovider.PublishCarDataFromChannel(r.eventKey, carDataChannel)
	r.dataprovider.SendExtraInfoFromChannel(r.eventKey, extraInfoChannel)

	mainLoop := func(ctx context.Context) {
		durations := []time.Duration{}
		for {
			select {
			case <-ctx.Done():
				log.Debug("mainLoop recieved ctx.Done")
				return
			case _, more := <-recordingDoneChannel:
				log.Debug("mainLoop recieved recordingDoneChannel", log.Bool("more", more))
				if !more {
					log.Info("Recording done.")
					r.config.cancel()
					return
				}
			default:
				if !r.simIsRunning {
					log.Warn("Sim is not running. Should not happen", log.String("method", "mainLoop"))
					time.Sleep(time.Second)
					continue
				}
				if r.api.GetData() {
					startProc := time.Now()
					proc.Process()
					durations = append(durations, time.Since(startProc))
					// log.Debug("Processed data", log.Duration("duration", time.Since(startProc)))
					if len(durations) == 120 {
						// set min to 1s
						min := time.Duration(1 * time.Second)
						max := time.Duration(0)
						sum := int64(0)
						for _, v := range durations {
							if v < min {
								min = v
							}
							if v > max {
								max = v
							}
							sum += v.Nanoseconds()
						}

						log.Debug("Processed data", log.Duration("min", min), log.Duration("max", max), log.Duration("avg", time.Duration(sum/int64(len(durations)))))
						durations = []time.Duration{}

					}
				}
				// log.Debug("end of loop")
				// time.Sleep(time.Second)
			}
		}
	}

	go mainLoop(r.config.ctx)
}

func (r *Racelogger) processStateChannel(stateChannel chan model.StateData) {
	handleChannelMessages := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				log.Debug("processStateChannel recieved ctx.Done")
				return
			case msg := <-stateChannel:
				{
					log.Debug("recieved stateChannel msg", log.Any("msg", msg))
				}

			}
		}
	}
	go handleChannelMessages(r.config.ctx)
}
