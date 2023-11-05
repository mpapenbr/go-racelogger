package processor

import (
	"reflect"
	"time"

	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	"golang.org/x/exp/slices"
)

// CarDriverProc is the main processor for managing driver and team data
type CarDriverProc struct {
	api *irsdk.Irsdk
	// maps carIdx to current driver of the car
	lookup             map[int32]yaml.Drivers
	byCarIdLookup      map[int32][]yaml.Drivers
	byCarClassIdLookup map[int32][]yaml.Drivers

	// maps carIdx to all drivers of the team
	teams  map[int32][]yaml.Drivers
	output chan model.CarData
}

func NewCarDriverProc(api *irsdk.Irsdk, output chan model.CarData) *CarDriverProc {

	return newCarDriverProcInternal(api, output)
}

// use this for testing with custom yaml content
func newCarDriverProcInternal(api *irsdk.Irsdk, output chan model.CarData) *CarDriverProc {
	ret := CarDriverProc{api: api, output: output}
	ret.init(api.GetLatestYaml())
	return &ret
}

func (d *CarDriverProc) init(y *yaml.IrsdkYaml) {
	d.lookup = make(map[int32]yaml.Drivers)
	d.byCarIdLookup = make(map[int32][]yaml.Drivers)
	d.byCarClassIdLookup = make(map[int32][]yaml.Drivers)

	d.teams = make(map[int32][]yaml.Drivers)

	for _, v := range y.DriverInfo.Drivers {
		if !isRealDriver(v) {
			continue
		}

		newEntry := v
		d.lookup[int32(v.CarIdx)] = newEntry
		teamMembers := []yaml.Drivers{newEntry}
		d.teams[int32(v.CarIdx)] = teamMembers
		if vCar, ok := d.byCarIdLookup[int32(v.CarID)]; !ok {
			d.byCarIdLookup[int32(v.CarID)] = []yaml.Drivers{newEntry}
		} else {
			vCar = append(vCar, newEntry)
		}

		if vCar, ok := d.byCarClassIdLookup[int32(v.CarClassID)]; !ok {
			d.byCarClassIdLookup[int32(v.CarClassID)] = []yaml.Drivers{newEntry}
		} else {
			vCar = append(vCar, newEntry)
		}
	}

}

func (d *CarDriverProc) GetCurrentDriver(carIdx int32) yaml.Drivers {
	return d.lookup[carIdx]
}

// gets called when main processor detects new driver data
func (d *CarDriverProc) Process(y *yaml.IrsdkYaml) {
	currentDriverNames := make(map[int]string)
	for _, v := range y.DriverInfo.Drivers {
		if !isRealDriver(v) {
			continue
		}
		newEntry := reflect.ValueOf(v).Interface().(yaml.Drivers)
		if _, ok := d.lookup[int32(v.CarIdx)]; !ok {
			// we have a new driver, create it
			d.lookup[int32(v.CarIdx)] = newEntry
			teamMembers := []yaml.Drivers{newEntry}
			d.teams[int32(v.CarIdx)] = teamMembers
		} else {
			teamMembers := d.teams[int32(v.CarIdx)]
			if !slices.ContainsFunc(teamMembers, func(ld yaml.Drivers) bool {
				return ld.UserID == v.UserID
			}) {
				teamMembers = append(teamMembers, newEntry)
			}
		}
		currentDriverNames[int(v.CarIdx)] = v.UserName
	}

	carEntries := []model.CarEntry{}
	for k, v := range d.lookup {
		car := model.Car{
			CarIdx:       int(k),
			CarNumber:    v.CarNumber,
			CarNumberRaw: v.CarNumberRaw,
			CarClassID:   v.CarClassID,
			CarID:        v.CarID,
			Name:         v.CarScreenNameShort,
		}
		team := model.Team{
			ID:     v.TeamID,
			Name:   v.TeamName,
			CarIdx: int(k),
		}

		drivers := []model.Driver{}
		for _, member := range d.teams[int32(k)] {
			drivers = append(drivers, model.Driver{
				CarIdx:      int(k),
				ID:          member.UserID,
				Name:        member.UserName,
				IRating:     member.IRating,
				Initials:    member.Initials,
				LicLevel:    member.LicLevel,
				LicSubLevel: member.LicSubLevel,
				LicString:   member.LicString,
				AbbrevName:  member.AbbrevName,
			})
		}
		entry := model.CarEntry{Car: car, Team: team, Drivers: drivers}
		carEntries = append(carEntries, entry)
	}
	sessionTime := justValue(d.api.GetValue("SessionTime"))
	data := model.CarData{
		Type:      int(model.MTCar),
		Timestamp: float64(time.Now().UnixMilli()),
		Payload: model.CarPayload{
			Cars:           collectCars(y.DriverInfo.Drivers),
			CarClasses:     collectCarClasses(y.DriverInfo.Drivers),
			Entries:        carEntries,
			CurrentDrivers: currentDriverNames,
			SessionTime:    sessionTime.(float64),
		},
	}
	d.output <- data
}
