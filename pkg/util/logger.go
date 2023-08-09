package util

import (
	"os"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/config"
)

func SetupLogger() *log.Logger {
	var logger *log.Logger
	switch config.LogFormat {
	case "json":
		logger = log.New(
			os.Stderr,
			parseLogLevel(config.LogLevel, log.InfoLevel),
			log.WithCaller(true),
			log.AddCallerSkip(1))
	default:
		logger = log.DevLogger(
			os.Stderr,
			parseLogLevel(config.LogLevel, log.DebugLevel),
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
