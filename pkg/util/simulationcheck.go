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
