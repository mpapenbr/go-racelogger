package util

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/mpapenbr/go-racelogger/pkg/config"
)

//nolint:nestif // false positive
func ConnectGrpc(cfg *config.CliArgs) (*grpc.ClientConn, error) {
	if cfg.Insecure {
		return grpc.NewClient(cfg.Addr,
			grpc.WithTransportCredentials(insecure.NewCredentials()))
	} else {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS13, // Set the minimum TLS version to TLS 1.3
		}
		if cfg.TLSCert != "" && cfg.TLSKey != "" {
			cert, err := tls.LoadX509KeyPair(cfg.TLSCert, cfg.TLSKey)
			if err != nil {
				return nil, err
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		if cfg.TLSCa != "" {
			caCert, err := os.ReadFile(cfg.TLSCa)
			if err != nil {
				return nil, err
			}
			caCertPool := x509.NewCertPool()
			if ok := caCertPool.AppendCertsFromPEM(caCert); !ok {
				return nil, fmt.Errorf("failed to append server certificate")
			}
			tlsConfig.RootCAs = caCertPool
		}
		if cfg.TLSSkipVerify {
			tlsConfig.InsecureSkipVerify = true
		}
		return grpc.NewClient(cfg.Addr,
			grpc.WithTransportCredentials(credentials.NewTLS(tlsConfig)))
	}
}
