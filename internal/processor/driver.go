package processor

import (
	"reflect"

	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
)

// CarDriverProc is the main processor for managing driver and team data
type CarDriverProc struct {
	api *irsdk.Irsdk
	// maps carIdx to current driver of the car
	lookup map[int32]yaml.Drivers
	// maps carIdx to all drivers of the team
	teams  map[int32][]yaml.Drivers
	output chan model.CarData //TODO: not sure if we need this
}

func NewCarDriverProc(api *irsdk.Irsdk, output chan model.CarData) *CarDriverProc {
	y, _ := api.GetYaml()
	return newCarDriverProcInternal(api, output, y)
}

// use this for testing with custom yaml content
func newCarDriverProcInternal(api *irsdk.Irsdk, output chan model.CarData, y *yaml.IrsdkYaml) *CarDriverProc {
	ret := CarDriverProc{api: api, output: output}
	ret.init(y)
	return &ret
}

func (d *CarDriverProc) init(y *yaml.IrsdkYaml) {
	d.lookup = make(map[int32]yaml.Drivers)
	d.teams = make(map[int32][]yaml.Drivers)
	for _, v := range y.DriverInfo.Drivers {
		if !isRealDriver(v) {
			continue
		}

		newEntry := reflect.ValueOf(v).Interface().(yaml.Drivers)
		d.lookup[int32(v.CarIdx)] = newEntry
		teamMembers := []yaml.Drivers{newEntry}
		d.teams[int32(v.CarIdx)] = teamMembers
	}

}

func (d *CarDriverProc) GetCurrentDriver(carIdx int32) yaml.Drivers {
	return d.lookup[carIdx]
}

// gets called when main processor detects new driver data
func (d *CarDriverProc) Process(y *yaml.IrsdkYaml) {
	// do nothing
}
