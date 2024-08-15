package config

var (
	URL      string // Deprecated: URL of WAMP server
	Realm    string // Deprecated: Realm for the racelog endpoints
	Password string // Deprecated: Password for Dataprovider access

	LogLevel  string // sets the log level (zap log level values)
	LogFormat string // text vs json

)

//nolint:lll // better readability
type CliArgs struct {
	Addr                    string  // ism gRPC address
	Insecure                bool    // connect to gRPC server without TLS
	TLSSkipVerify           bool    // skip TLS verification
	TLSCert                 string  // path to TLS certificate
	TLSKey                  string  // path to TLS key
	TLSCa                   string  // path to TLS CA
	LogLevel                string  // sets the log level (zap log level values)
	LogFormat               string  // text vs json
	LogFile                 string  // log file to write to
	Token                   string  // token for authentication
	WaitForServices         string  // duration to wait for other services to be ready
	WaitForData             string  // duration to wait for data to be available
	SpeedmapPublishInterval string  // duration to publish speedmap data
	SpeedmapSpeedThreshold  float64 // do not record speed below this threshold pct (0-1.0)
	MaxSpeed                float64 // do not process  speeds above this value (km/h)
	DoNotPersist            bool    // do not persist the recorded data (used for debugging)
	MsgLogFile              string  // write grpc messages to this file
	EnsureLiveData          bool    // if true, replay will be set to live data on connection
	EnsureLiveDataInterval  string  // interval to set replay mode to live mode
}

var cliArgs = NewCliArgs()

func DefaultCliArgs() *CliArgs {
	return cliArgs
}

func NewCliArgs() *CliArgs {
	return &CliArgs{}
}
