package log

import (
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/yaml.v3"
)

type Config struct {
	DefaultLevel string            `yaml:"defaultLevel"`
	Loggers      map[string]string `yaml:"loggers"`
	Zap          zap.Config        `yaml:"zap"`
	Filters      []string          `yaml:"filters"`
}

type NamedLoggerConfig struct {
	Level string `yaml:"level"`
}

func DefaultDevConfig() *Config {
	z := zap.NewDevelopmentConfig()
	z.EncoderConfig.EncodeTime = zapcore.TimeEncoderOfLayout("15:04:05.000")
	return &Config{
		DefaultLevel: "info",
		Zap:          z,
	}
}

func DefaultProdConfig() *Config {
	return &Config{
		DefaultLevel: "info",
		Zap:          zap.NewProductionConfig(),
	}
}

func LoadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	cfg := Config{
		Zap: zap.NewProductionConfig(),
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
