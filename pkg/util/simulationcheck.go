package util

import (
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

	log.Debug("Using timout ", log.String("timeout", timeout.String()))

	log.Info("Waiting for iRacing Simulation")
	deadline := time.Now().Add(timeout)
	goon := time.Now().Before(deadline)
	for goon {
		if irsdk.CheckIfSimIsRunning() {
			log.Info("iRacing Simulation is running")
			return true
		}
		time.Sleep(time.Second * 2)
		goon = time.Now().Before(deadline)
	}
	return false
}
