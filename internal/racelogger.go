package internal

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"github.com/mpapenbr/go-racelogger/pkg/wamp"
	"github.com/mpapenbr/go-racelogger/version"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/service"
	"golang.org/x/exp/slices"
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
}

// TODO: Define this in service-manager
type EventSession struct {
	Num  int    `json:"num"`
	Name string `json:"name"`
}

var ErrUnknownValueWithUnit = errors.New("Unknown value with unit format")

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
	req := service.RegisterEventRequest{
		EventInfo: *eventInfo,
		EventKey:  r.eventKey,
		TrackInfo: *trackInfo,
		Manifests: model.Manifests{},
	}
	return r.dataprovider.RegisterProvider(req)

}
func (r *Racelogger) UnregisterProvider() {
	r.dataprovider.UnregisterProvider(r.eventKey)
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
	r.setupDriverChangeDetector(time.Second)

}

func (r *Racelogger) createEventInfo(irYaml *yaml.IrsdkYaml) (*model.EventDataInfo, error) {

	pitSpeed, _ := r.getMetricUnit(irYaml.WeekendInfo.TrackPitSpeedLimit)
	trackLength, _ := r.getMetricUnit(irYaml.WeekendInfo.TrackLength)
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

		RaceloggerVersion: version.Version, // TODO
	}
	return &ret, nil
}

func (r *Racelogger) createTrackInfo(irYaml *yaml.IrsdkYaml) (*model.TrackInfo, error) {

	trackLength, _ := r.getMetricUnit(irYaml.WeekendInfo.TrackLength)
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

func (r *Racelogger) getMetricUnit(s string) (float64, error) {
	re := regexp.MustCompile("(?P<value>[0-9.]+)\\s+(?P<unit>.*)")

	if !re.Match([]byte(s)) {
		log.Error("invalid data with unit", log.String("data", s))
		return 0, ErrUnknownValueWithUnit
	}
	matches := re.FindStringSubmatch(s)
	value := matches[re.SubexpIndex("value")]
	unit := matches[re.SubexpIndex("unit")]
	if f, err := strconv.ParseFloat(value, 64); err != nil {

		if slices.Contains([]string{"m", "km", "kph", "C"}, unit) {
			return f, nil
		}
		switch unit {
		case "mi":
			return f * 1.60934, nil
		default:
			return f, nil
		}

	} else {
		return 0, err
	}
}

func (r *Racelogger) convertSectors(sectors []yaml.Sectors) []model.Sector {
	ret := make([]model.Sector, len(sectors))
	for i, v := range sectors {
		ret[i] = model.Sector{SectorNum: v.SectorNum, SectorStartPct: v.SectorStartPct}
	}
	return ret
}

// TODO: modify and use this if EventSession is available in iracelog-service-manager
func (r *Racelogger) convertSessionsA(sectors []yaml.Sessions) []EventSession {
	ret := make([]EventSession, len(sectors))
	for i, v := range sectors {
		ret[i] = EventSession{Num: v.SessionNum, Name: v.SessionName}
	}
	return ret
}

// TODO:  remove this if EventSession is available in iracelog-service-manager
func (r *Racelogger) convertSessions(sectors []yaml.Sessions) []struct {
	Num  int    `json:"num"`
	Name string `json:"name"`
} {
	ret := make([]struct {
		Num  int    `json:"num"`
		Name string `json:"name"`
	}, len(sectors))
	for i, v := range sectors {
		ret[i] = EventSession{Num: v.SessionNum, Name: v.SessionName}
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

	mainLoop := func(ctx context.Context) {
		for {
			select {
			case <-ctx.Done():
				log.Debug("mainLoop recieved ctx.Done")
				return
			default:
				if !r.simIsRunning {
					log.Warn("Sim is not running. Should not happen", log.String("method", "mainLoop"))
					time.Sleep(time.Second)
					continue
				}
				r.api.GetData()
				time.Sleep(time.Second)
			}
		}
	}

	go mainLoop(r.config.ctx)
}
