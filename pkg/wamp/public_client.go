package wamp

import (
	"context"
	"log"
	"os"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/wamp"
)

type PublicClient struct {
	client *client.Client
}

func NewPublicClient(url, realm string) *PublicClient {
	logger := log.New(os.Stdout, "", 0)
	cfg := client.Config{Realm: realm, Logger: logger}
	// Connect wampClient session.
	wampClient, err := client.ConnectNet(context.Background(), url, cfg)
	if err != nil {
		logger.Fatal(err)
	}

	ret := &PublicClient{client: wampClient}
	return ret
}

func (pc *PublicClient) Close() {
	pc.client.Close()
}

func (pc *PublicClient) Client() *client.Client {
	return pc.client
}

func (pc *PublicClient) GetVersion() (string, error) {
	result, err := pc.client.Call(
		context.Background(),
		"racelog.public.get_version",
		nil,
		wamp.List{},
		nil,
		nil)
	if err != nil {
		return "", err
	}

	if len(result.Arguments) == 0 {
		return "", ErrNoResults
	}
	ret, _ := wamp.AsDict(result.Arguments[0])
	version := ret["ownVersion"]
	return version.(string), nil
}
