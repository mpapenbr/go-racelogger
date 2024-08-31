//nolint:gocritic // by design
package processor

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"

	carv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/car/v1"
	commonv1 "buf.build/gen/go/mpapenbr/iracelog/protocolbuffers/go/iracelog/common/v1"
	"github.com/mpapenbr/goirsdk/irsdk"
	"github.com/mpapenbr/goirsdk/yaml"

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

func justValue(v any, _ error) any {
	return v
}

func getRaceState(api *irsdk.Irsdk) string {
	state, _ := api.GetIntValue("SessionState")
	flags, _ := api.GetIntValue("SessionFlags")
	return computeFlagState(state, int64(flags))
}

func isBitSet(v, bit int64) bool {
	return v&bit == bit
}

//nolint:cyclop,gocritic,nestif // ok this way
func computeFlagState(state int32, flags int64) string {
	if state == int32(irsdk.StateRacing) {
		if isBitSet(flags, int64(irsdk.FlagStartGo)) {
			return GREEN
		} else if isBitSet(flags, int64(irsdk.FlagStartHidden)) {
			if isBitSet(flags, int64(irsdk.FlagCaution)) ||
				isBitSet(flags, int64(irsdk.FlagCautionWaving)) {

				return YELLOW
			}
		}
		return GREEN
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

func carClassesLookup(drivers []yaml.Drivers) map[int]*carv1.CarClass {
	lookup := make(map[int]*carv1.CarClass)
	for _, d := range drivers {
		if isRealDriver(d) {
			if _, ok := lookup[d.CarClassID]; !ok {
				name := d.CarClassShortName
				if name == "" {
					name = fmt.Sprintf("CarClass %d", d.CarClassID)
				}
				lookup[d.CarClassID] = &carv1.CarClass{Id: uint32(d.CarClassID), Name: name}
			}
		}
	}
	return lookup
}

func collectCarClasses(drivers []yaml.Drivers) []*carv1.CarClass {
	lookup := carClassesLookup(drivers)
	ret := []*carv1.CarClass{}
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
//
//nolint:lll // readability
func collectCars(drivers []yaml.Drivers) []*carv1.CarInfo {
	classLookup := carClassesLookup(drivers)
	ret := []*carv1.CarInfo{}
	carLookup := make(map[int]*carv1.CarInfo)
	for _, d := range drivers {
		if isRealDriver(d) {
			if _, ok := carLookup[d.CarID]; !ok {
				carLookup[d.CarID] = &carv1.CarInfo{
					CarId:         uint32(d.CarID),
					CarClassId:    int32(d.CarClassID),
					CarClassName:  classLookup[d.CarClassID].Name,
					Name:          d.CarScreenName,
					NameShort:     d.CarScreenNameShort,
					FuelPct:       float32(justValue(GetMetricUnit(d.CarClassMaxFuelPct)).(float64)),
					PowerAdjust:   float32(justValue(GetMetricUnit(d.CarClassPowerAdjust)).(float64)),
					WeightPenalty: float32(justValue(GetMetricUnit(d.CarClassWeightPenalty)).(float64)),
					DryTireSets:   int32(justValue(GetMetricUnit(d.CarClassDryTireSetLimit)).(float64)),
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

// checks if relevant driver info changed
// we need this to detect new drivers and driver changes in team races
func HasDriverChange(current, last *yaml.DriverInfo) bool {
	if len(current.Drivers) != len(last.Drivers) {
		return true
	}
	createLookup := func(data []yaml.Drivers) map[int]string {
		ret := make(map[int]string)
		for i := 0; i < len(data); i++ {
			ret[data[i].CarIdx] = data[i].UserName
		}
		return ret
	}
	currentLookup := createLookup(current.Drivers)
	lastLookup := createLookup(last.Drivers)
	changeDetected := false
	for k, v := range currentLookup {
		if lastLookup[k] != v {
			log.Debug("Driver change detected",
				log.Int("carIdx", k),
				log.String("current", v),
				log.String("last", lastLookup[k]),
			)
			changeDetected = true
		}
	}
	return changeDetected
}

func readUint32(api *irsdk.Irsdk, key string) uint32 {
	val, err := api.GetIntValue(key)
	if err != nil {
		log.Error("error reading var", log.String("key", key), log.ErrorField(err))
	}
	return uint32(val)
}

func readInt32(api *irsdk.Irsdk, key string) int32 {
	val, err := api.GetIntValue(key)
	if err != nil {
		log.Error("error reading var", log.String("key", key), log.ErrorField(err))
	}
	return val
}

func readFloat32(api *irsdk.Irsdk, key string) float32 {
	val, err := api.GetFloatValue(key)
	if err != nil {
		log.Error("error reading var", log.String("key", key), log.ErrorField(err))
	}
	return val
}

func readFloat64(api *irsdk.Irsdk, key string) float64 {
	val, err := api.GetDoubleValue(key)
	if err != nil {
		log.Error("error reading var", log.String("key", key), log.ErrorField(err))
	}
	return val
}

func convertTrackWetness(api *irsdk.Irsdk) commonv1.TrackWetness {
	val, _ := api.GetIntValue("TrackWetness")
	switch val {
	case irsdk.TrackWetnessUnknown:
		return commonv1.TrackWetness_TRACK_WETNESS_UNSPECIFIED
	case irsdk.TrackWetnessDry:
		return commonv1.TrackWetness_TRACK_WETNESS_DRY
	case irsdk.TrackWetnessMostlyDry:
		return commonv1.TrackWetness_TRACK_WETNESS_MOSTLY_DRY
	case irsdk.TrackWetnessVeryLightlyWet:
		return commonv1.TrackWetness_TRACK_WETNESS_VERY_LIGHTLY_WET
	case irsdk.TrackWetnessLightlyWet:
		return commonv1.TrackWetness_TRACK_WETNESS_LIGHTLY_WET
	case irsdk.TrackWetnessModeratelyWet:
		return commonv1.TrackWetness_TRACK_WETNESS_MODERATELY_WET
	case irsdk.TrackWetnessVeryWet:
		return commonv1.TrackWetness_TRACK_WETNESS_VERY_WET
	case irsdk.TrackWetnessExtremeWet:
		return commonv1.TrackWetness_TRACK_WETNESS_EXTREMELY_WET
	}
	return commonv1.TrackWetness_TRACK_WETNESS_UNSPECIFIED
}
