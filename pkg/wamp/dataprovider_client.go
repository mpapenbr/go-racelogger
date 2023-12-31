package wamp

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gammazero/nexus/v3/client"
	"github.com/gammazero/nexus/v3/wamp"
	"github.com/mpapenbr/goirsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/service"
)

type Dataprovider interface {
	// GetEvent(eventId int) (*internal.Event, error)
}

type DataProviderClient struct {
	client *client.Client
}

func NewDataProviderClient(url, realm, ticket string) *DataProviderClient {
	logger := log.New(os.Stdout, "", 0)

	cfg := client.Config{
		Realm:        realm,
		Logger:       logger,
		HelloDetails: wamp.Dict{"authid": "dataprovider"},
		AuthHandlers: map[string]client.AuthFunc{
			"ticket": func(*wamp.Challenge) (string, wamp.Dict) { return ticket, wamp.Dict{} },
		},
	}

	ret := &DataProviderClient{client: GetClientWithConfigNew(url, &cfg)}
	return ret
}

func (dpc *DataProviderClient) Close() {
	dpc.client.Close()
}

// registers a new provider
//
//nolint:gocritic,whitespace // by design
func (dpc *DataProviderClient) RegisterProvider(
	registerMsg service.RegisterEventRequest,
) error {
	ctx := context.Background()
	_, err := dpc.client.Call(ctx,
		"racelog.dataprovider.register_provider",
		nil,
		wamp.List{registerMsg},
		nil,
		nil)

	return err
}

// unregisters a provider
func (dpc *DataProviderClient) UnregisterProvider(eventKey string) error {
	ctx := context.Background()
	_, err := dpc.client.Call(
		ctx,
		"racelog.dataprovider.remove_provider",
		nil,
		wamp.List{eventKey},
		nil,
		nil)
	return err
}

//nolint:gocritic,whitespace // by design
func (dpc *DataProviderClient) SendExtraInfoFromChannel(
	eventKey string,
	rcv chan model.ExtraInfo,
) {
	go func() {
		for {
			s, more := <-rcv
			ctx := context.Background()

			_, err := dpc.client.Call(
				ctx,
				"racelog.dataprovider.store_event_extra_data",
				nil,
				wamp.List{eventKey, s},
				nil,
				nil)
			if err != nil {
				log.Fatal(err)
			}
			// fmt.Printf("chanValue: %v more: %v\n", s.Timestamp, more)
			// time.Sleep(100 * time.Millisecond)
			if !more {
				fmt.Println("closed channel signaled")
				return
			}
		}
	}()
}

// receives data via channel and publishes it on the
// racelog.public.live.state.<eventKey> topic
//
//nolint:gocritic,whitespace // by design
func (dpc *DataProviderClient) PublishStateFromChannel(
	eventKey string,
	rcv chan model.StateData,
) {
	go func() {
		for {
			s, more := <-rcv
			err := dpc.client.Publish(
				fmt.Sprintf("racelog.public.live.state.%s", eventKey),
				nil,
				wamp.List{s},
				nil)
			if err != nil {
				log.Fatal(err)
			}
			// fmt.Printf("chanValue: %v more: %v\n", s.Timestamp, more)
			// time.Sleep(100 * time.Millisecond)
			if !more {
				fmt.Println("closed channel signaled")
				return
			}
		}
	}()
}

func (dpc *DataProviderClient) PublishCarData(eventKey string, carData *model.CarData) {
	err := dpc.client.Publish(
		fmt.Sprintf("racelog.public.live.cardata.%s", eventKey),
		nil,
		wamp.List{carData},
		nil)
	if err != nil {
		log.Fatal(err)
	}
}

//nolint:gocritic,whitespace // by design
func (dpc *DataProviderClient) PublishCarDataFromChannel(
	eventKey string,
	rcv chan model.CarData,
) {
	go func() {
		for {
			s, more := <-rcv
			err := dpc.client.Publish(
				fmt.Sprintf("racelog.public.live.cardata.%s", eventKey),
				nil,
				wamp.List{s},
				nil)
			if err != nil {
				log.Fatal(err)
			}
			// fmt.Printf("chanValue: %v more: %v\n", s.Timestamp, more)
			// time.Sleep(100 * time.Millisecond)
			if !more {
				fmt.Println("closed channel signaled")
				return
			}
		}
	}()
}

//nolint:whitespace // by design
func (dpc *DataProviderClient) PublishDriverData(
	eventKey string,
	driverData *yaml.DriverInfo,
) {
	err := dpc.client.Publish(
		fmt.Sprintf("racelog.public.live.driverdata.%s", eventKey),
		nil,
		wamp.List{driverData},
		nil)
	if err != nil {
		log.Fatal(err)
	}
}

//nolint:gocritic,whitespace // by design
func (dpc *DataProviderClient) PublishSpeedmapDataFromChannel(
	eventKey string,
	rcv chan model.SpeedmapData,
) {
	go func() {
		for {
			s, more := <-rcv
			err := dpc.client.Publish(
				fmt.Sprintf("racelog.public.live.speedmap.%s",
					eventKey),
				nil,
				wamp.List{s},
				nil)
			if err != nil {
				log.Fatal(err)
			}
			// fmt.Printf("chanValue: %v more: %v\n", s.Timestamp, more)
			// time.Sleep(100 * time.Millisecond)
			if !more {
				fmt.Println("closed channel signaled")
				return
			}
		}
	}()
}
