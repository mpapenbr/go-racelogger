package config

var (
	URL             string // URL of WAMP server
	Realm           string // Realm for the racelog endpoints
	Password        string // Password for Dataprovider access
	WaitForServices string // duration to wait for other services to be ready
	WaitForData     string // duration to wait for data to be available
	LogLevel        string // sets the log level (zap log level values)
	LogFormat       string // text vs json

)
