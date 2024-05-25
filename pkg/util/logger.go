package util

import (
	"fmt"
	"os"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
)

func SetupLogger(cfg *config.CliArgs) *log.Logger {
	var logger *log.Logger
	logFile := os.Stdout
	var err error
	if cfg.LogFile != "" {
		logFile, err = os.Create(cfg.LogFile)
		if err != nil {
			fmt.Printf("Error creating log file: %s\nLogging to stdout", err)
			logFile = os.Stdout
		}
	}
	switch cfg.LogFormat {
	case "json":
		logger = log.New(
			logFile,
			parseLogLevel(cfg.LogLevel, log.InfoLevel),
			log.WithCaller(true),
			log.AddCallerSkip(1))
	default:
		logger = log.DevLogger(
			logFile,
			parseLogLevel(cfg.LogLevel, log.DebugLevel),
			log.WithCaller(true),
			log.AddCallerSkip(1))
	}

	log.ResetDefault(logger)
	return logger
}

func parseLogLevel(l string, defaultVal log.Level) log.Level {
	level, err := log.ParseLevel(l)
	if err != nil {
		return defaultVal
	}
	return level
}
