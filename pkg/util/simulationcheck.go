package util

import (
	"context"
	"net/http"
	"time"

	"github.com/mpapenbr/goirsdk/irsdk"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
)

func WaitForSimulation(cfg *config.CliArgs) bool {
	timeout, err := time.ParseDuration(cfg.WaitForServices)
	if err != nil {
		log.Warn("Invalid duration value. Setting default 60s", log.ErrorField(err))
		timeout = 60 * time.Second
	}

	log.Info("Waiting for iRacing Simulation", log.String("timeout", timeout.String()))
	ticker := time.NewTicker(1 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return false
		case <-ticker.C:
			simAvail, err := irsdk.IsSimRunning(ctx, http.DefaultClient)
			if err != nil {
				log.Debug("Error connecting sim", log.ErrorField(err))
				break
			}
			if simAvail {
				ticker.Stop()
				return true
			}
		}
	}
}

func HasValidAPIData(api *irsdk.Irsdk) bool {
	api.GetData()
	return len(api.GetValueKeys()) > 0 && hasPlausibleYaml(api)
}

// the yaml data is considered valid if certain plausible values are present.
// for example: the track length must be > 0, track sectors are present
func hasPlausibleYaml(api *irsdk.Irsdk) bool {
	ret := true
	y, err := api.GetYaml()
	if err != nil {
		return false
	}
	if y.WeekendInfo.NumCarTypes == 0 {
		ret = false
	}
	if y.WeekendInfo.TrackID == 0 {
		ret = false
	}
	if len(y.SplitTimeInfo.Sectors) == 0 {
		ret = false
	}
	if len(y.SessionInfo.Sessions) == 0 {
		ret = false
	}
	return ret
}
