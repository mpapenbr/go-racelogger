//nolint:gocritic // by design
package processor

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"time"

	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/mpapenbr/goirsdk/yaml"
	"github.com/mpapenbr/iracelog-service-manager-go/pkg/model"
	"golang.org/x/exp/slices"

	"github.com/mpapenbr/go-racelogger/log"
)

var ErrUnknownValueWithUnit = errors.New("Unknown value with unit format")

const (
	INVALID   = "INVALID"
	PREP      = "PREP"
	PARADE    = "PARADE"
	GREEN     = "GREEN"
	YELLOW    = "YELLOW"
	CHECKERED = "CHECKERED"
	WHITE     = "WHITE"
)

// returns time as unix seconds and microseconds as decimal part
func float64Timestamp(t time.Time) float64 {
	return float64(t.Unix()) + (float64(t.UnixMicro()%1e6))/float64(1e6)
}

func justValue(v any, _ error) any {
	return v
}

func getRaceState(api *irsdk.Irsdk) string {
	state, _ := api.GetIntValue("SessionState")
	flags, _ := api.GetIntValue("SessionFlags")
	return computeFlagState(state, int64(flags))
}

//nolint:cyclop,gocritic,nestif // ok this way
func computeFlagState(state int32, flags int64) string {
	if state == int32(irsdk.StateRacing) {
		if flags&int64(irsdk.FlagStartHidden) == int64(irsdk.FlagStartHidden) {
			return GREEN
		} else if flags>>16&int64(irsdk.FlagGreen) == int64(irsdk.FlagGreen) {
			return GREEN
		} else if flags>>16&int64(irsdk.FlagYello) == int64(irsdk.FlagYello) {
			return YELLOW
		} else if flags>>16&int64(irsdk.FlagCheckered) == int64(irsdk.FlagCheckered) {
			return CHECKERED
		} else if flags>>16&int64(irsdk.FlagWhite) == int64(irsdk.FlagWhite) {
			return WHITE
		}
	} else if state == int32(irsdk.StateCheckered) {
		return CHECKERED
	} else if state == int32(irsdk.StateCoolDown) {
		return CHECKERED
	} else if state == int32(irsdk.StateGetInCar) {
		return PREP
	} else if state == int32(irsdk.StateParadeLaps) {
		return PARADE
	} else if state == int32(irsdk.StateInvalid) {
		return INVALID
	}
	return "NONE"
}

// returns true if we should record data
func shouldRecord(api *irsdk.Irsdk) bool {
	return slices.Contains([]string{"GREEN", "YELLOW", "CHECKERED"}, getRaceState(api))
}

//nolint:gocritic // this is ok
func isRealDriver(d yaml.Drivers) bool {
	return d.IsSpectator == 0 && d.CarIsPaceCar == 0 && len(d.UserName) > 0
}

func getProcessableCarIdxs(drivers []yaml.Drivers) []int {
	ret := []int{}

	for _, d := range drivers {
		if isRealDriver(d) {
			ret = append(ret, d.CarIdx)
		}
	}
	return ret
}

func gate(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func deltaDistance(a, b float64) float64 {
	if a >= b {
		return a - b
	} else {
		return a + 1 - b
	}
}

func GetMetricUnit(s string) (float64, error) {
	re := regexp.MustCompile(`(?P<value>[0-9.-]+)\s*(?P<unit>.*)`)

	if !re.MatchString(s) {
		log.Error("invalid data with unit", log.String("data", s))
		return 0, ErrUnknownValueWithUnit
	}
	matches := re.FindStringSubmatch(s)
	value := matches[re.SubexpIndex("value")]
	unit := matches[re.SubexpIndex("unit")]
	if f, err := strconv.ParseFloat(value, 64); err == nil {
		if slices.Contains([]string{"m", "km", "kph", "C"}, unit) {
			return f, nil
		}
		switch unit {
		case "mi":
			return f * 1.60934, nil
		default:
			return f, nil
		}
	} else {
		return 0, err
	}
}

func GetTrackLengthInMeters(s string) (float64, error) {
	if f, err := GetMetricUnit(s); err == nil {
		return f * 1000, nil
	} else {
		return 0, err
	}
}

func carClassesLookup(drivers []yaml.Drivers) map[int]model.CarClass {
	lookup := make(map[int]model.CarClass)
	for _, d := range drivers {
		if isRealDriver(d) {
			if _, ok := lookup[d.CarClassID]; !ok {
				name := d.CarClassShortName
				if name == "" {
					name = fmt.Sprintf("CarClass %d", d.CarClassID)
				}
				lookup[d.CarClassID] = model.CarClass{ID: d.CarClassID, Name: name}
			}
		}
	}
	return lookup
}

func collectCarClasses(drivers []yaml.Drivers) []model.CarClass {
	lookup := carClassesLookup(drivers)
	ret := []model.CarClass{}
	for _, v := range lookup {
		ret = append(ret, v)
	}
	return ret
}

// collects the car informations from irdsk DriverInfo.Drivers
// (which is passed in as drivers argument)
// Kind of confusing:
//   - adjustments are made to a car, but the attributes are prefixed 'CarClass'
//   - CarClassDryTireSetLimit is delivered as percent, but it contains the number
//     of tire sets available
//   - CarClassMaxFuelPct value is 0.0-1.0
func collectCars(drivers []yaml.Drivers) []model.CarInfo {
	classLookup := carClassesLookup(drivers)
	ret := []model.CarInfo{}
	carLookup := make(map[int]model.CarInfo)
	for _, d := range drivers {
		if isRealDriver(d) {
			if _, ok := carLookup[d.CarID]; !ok {
				carLookup[d.CarID] = model.CarInfo{
					CarID:         d.CarID,
					CarClassID:    d.CarClassID,
					CarClassName:  classLookup[d.CarClassID].Name,
					Name:          d.CarScreenName,
					NameShort:     d.CarScreenNameShort,
					FuelPct:       justValue(GetMetricUnit(d.CarClassMaxFuelPct)).(float64),
					PowerAdjust:   justValue(GetMetricUnit(d.CarClassPowerAdjust)).(float64),
					WeightPenalty: justValue(GetMetricUnit(d.CarClassWeightPenalty)).(float64),
					DryTireSets:   int(justValue(GetMetricUnit(d.CarClassDryTireSetLimit)).(float64)),
				}
			}
		}
	}
	//nolint:gocritic // this is ok
	for _, v := range carLookup {
		ret = append(ret, v)
	}
	return ret
}
