package config

var (
	URL                     string  // URL of WAMP server
	Realm                   string  // Realm for the racelog endpoints
	Password                string  // Password for Dataprovider access
	WaitForServices         string  // duration to wait for other services to be ready
	WaitForData             string  // duration to wait for data to be available
	LogLevel                string  // sets the log level (zap log level values)
	LogFormat               string  // text vs json
	SpeedmapPublishInterval string  // duration to publish speedmap data
	SpeedmapSpeedThreshold  float64 // do not record speed below this threshold pct (0-1.0)
	MaxSpeed                float64 // do not process  speeds above this value (km/h)
)
