package processor

import (
	"reflect"
	"slices"

	carv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/car/v1"
	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	driverv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/driver/v1"
	racestatev1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/racestate/v1"
	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/mpapenbr/goirsdk/yaml"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// CarDriverProc is the main processor for managing driver and team data
type CarDriverProc struct {
	api *irsdk.Irsdk
	// maps carIdx to current driver of the car
	lookup             map[int32]yaml.Drivers
	byCarIDLookup      map[int32][]yaml.Drivers
	byCarClassIDLookup map[int32][]yaml.Drivers

	// maps carIdx to all drivers of the team
	teams map[int32][]yaml.Drivers
	// holds the mapping driverName by carIdx from the latest processing
	latestDriverNames map[int32]string
	output            chan *racestatev1.PublishDriverDataRequest
	reportChangeFunc  func(carIdx int)
	gpd               *GlobalProcessingData
}

//nolint:whitespace // can't get different linters happy
func NewCarDriverProc(
	api *irsdk.Irsdk,
	output chan *racestatev1.PublishDriverDataRequest,
	gpd *GlobalProcessingData,
) *CarDriverProc {
	return newCarDriverProcInternal(api, output, gpd)
}

// use this for testing with custom yaml content
//
//nolint:whitespace // can't get different linters happy
func newCarDriverProcInternal(
	api *irsdk.Irsdk,
	output chan *racestatev1.PublishDriverDataRequest,
	gpd *GlobalProcessingData,
) *CarDriverProc {
	ret := CarDriverProc{api: api, output: output, gpd: gpd}
	ret.init(api.GetLatestYaml())
	return &ret
}

//nolint:gocritic // by design
func (d *CarDriverProc) init(y *yaml.IrsdkYaml) {
	d.lookup = make(map[int32]yaml.Drivers)
	d.byCarIDLookup = make(map[int32][]yaml.Drivers)
	d.byCarClassIDLookup = make(map[int32][]yaml.Drivers)
	d.latestDriverNames = make(map[int32]string)

	d.teams = make(map[int32][]yaml.Drivers)

	for _, v := range y.DriverInfo.Drivers {
		if !isRealDriver(v) {
			continue
		}

		newEntry := v
		d.lookup[int32(v.CarIdx)] = newEntry
		teamMembers := []yaml.Drivers{newEntry}
		d.teams[int32(v.CarIdx)] = teamMembers
		if _, ok := d.byCarIDLookup[int32(v.CarID)]; !ok {
			d.byCarIDLookup[int32(v.CarID)] = []yaml.Drivers{newEntry}
		} else {
			d.byCarIDLookup[int32(v.CarID)] = append(d.byCarIDLookup[int32(v.CarID)], newEntry)
		}

		if _, ok := d.byCarClassIDLookup[int32(v.CarClassID)]; !ok {
			d.byCarClassIDLookup[int32(v.CarClassID)] = []yaml.Drivers{newEntry}
		} else {
			d.byCarClassIDLookup[int32(v.CarClassID)] = append(
				d.byCarClassIDLookup[int32(v.CarClassID)], newEntry)
		}
		d.latestDriverNames[int32(v.CarIdx)] = v.UserName
		d.reportChange(int32(v.CarIdx))
	}
}

func (d *CarDriverProc) reportChange(carID int32) {
	if d.reportChangeFunc != nil {
		d.reportChangeFunc(int(carID))
	}
}

func (d *CarDriverProc) SetReportChangeFunc(reportChangeFunc func(carIdx int)) {
	d.reportChangeFunc = reportChangeFunc
}

func (d *CarDriverProc) GetCurrentDriver(carIdx int32) yaml.Drivers {
	return d.lookup[carIdx]
}

// gets called when main processor detects new driver data
//
//nolint:funlen,gocritic,errcheck// keep things together and simple
func (d *CarDriverProc) Process(y *yaml.IrsdkYaml) {
	currentDriverNames := make(map[uint32]string)
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

				d.teams[int32(v.CarIdx)] = append(d.teams[int32(v.CarIdx)], newEntry)
			}
		}
		if d.latestDriverNames[int32(v.CarIdx)] != v.UserName {
			d.latestDriverNames[int32(v.CarIdx)] = v.UserName
			d.reportChange(int32(v.CarIdx))
		}
		currentDriverNames[uint32(v.CarIdx)] = v.UserName
	}

	carEntries := make([]*carv1.CarEntry, len(d.lookup))
	i := 0
	for k, v := range d.lookup {
		// x := &carv1.Car{}
		car := carv1.Car{
			CarIdx:       uint32(k),
			CarNumber:    v.CarNumber,
			CarNumberRaw: int32(v.CarNumberRaw),
			CarClassId:   int32(v.CarClassID),
			CarId:        uint32(v.CarID),
			Name:         v.CarScreenNameShort,
		}
		team := driverv1.Team{
			Id:     uint32(v.TeamID),
			Name:   v.TeamName,
			CarIdx: uint32(k),
		}

		drivers := []*driverv1.Driver{}
		for _, member := range d.teams[k] {
			drivers = append(drivers, &driverv1.Driver{
				CarIdx:      uint32(k),
				Id:          int32(member.UserID),
				Name:        member.UserName,
				IRating:     int32(member.IRating),
				Initials:    member.Initials,
				LicLevel:    int32(member.LicLevel),
				LicSubLevel: int32(member.LicSubLevel),
				LicString:   member.LicString,
				AbbrevName:  member.AbbrevName,
			})
		}
		entry := carv1.CarEntry{Car: &car, Team: &team, Drivers: drivers}
		carEntries[i] = &entry
		i++
	}
	sessionTime := float32(readFloat64(d.api, "SessionTime"))
	sessionNum := justValue(d.api.GetIntValue("SessionNum")).(int32)

	data := racestatev1.PublishDriverDataRequest{
		Event: &commonv1.EventSelector{
			Arg: &commonv1.EventSelector_Key{
				Key: d.gpd.EventDataInfo.Key,
			},
		},
		Timestamp: timestamppb.Now(),

		Cars:           collectCars(y.DriverInfo.Drivers),
		CarClasses:     collectCarClasses(y.DriverInfo.Drivers),
		Entries:        carEntries,
		CurrentDrivers: currentDriverNames,
		SessionTime:    sessionTime,
		SessionNum:     uint32(sessionNum),
	}
	d.output <- &data
}
