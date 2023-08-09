package internal

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"github.com/mpapenbr/go-racelogger/pkg/wamp"
)

type Racelogger struct {
	ctx          context.Context
	cancel       context.CancelFunc
	eventKey     string
	api          *irsdk.Irsdk
	dataprovider *wamp.DataProviderClient
	simIsRunning bool
}

func NewRaceLogger(ctx context.Context, cancel context.CancelFunc, eventKey string) *Racelogger {

	ret := &Racelogger{
		simIsRunning: false,
		eventKey:     eventKey,
		ctx:          ctx,
		cancel:       cancel,
		dataprovider: wamp.NewDataProviderClient(config.URL, config.Realm, config.Password)}
	ret.init()
	return ret
}

func (r *Racelogger) Close() {
	r.api.Close()
	r.dataprovider.Close()
}

func (r *Racelogger) init() {

	r.setupWatchdog(time.Second)
	r.setupDriverChangeDetector(time.Second)

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

	go postData(r.ctx)
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

	go postData(r.ctx)
}
