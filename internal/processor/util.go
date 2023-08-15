package processor

import (
	"errors"
	"regexp"
	"strconv"

	"github.com/mpapenbr/go-racelogger/log"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk"
	"github.com/mpapenbr/go-racelogger/pkg/irsdk/yaml"
	"golang.org/x/exp/slices"
)

var ErrUnknownValueWithUnit = errors.New("Unknown value with unit format")

func justValue(v any, _ error) any {
	return v
}

func getRaceState(api *irsdk.Irsdk) string {
	state, _ := api.GetIntValue("SessionState")
	flags, _ := api.GetIntValue("SessionFlags")
	return computeFlagState(state, int64(flags))
}

func computeFlagState(state int32, flags int64) string {
	if state == int32(irsdk.StateRacing) {
		if flags&int64(irsdk.FlagStartHidden) == int64(irsdk.FlagStartHidden) {
			return "GREEN"
		} else if flags>>16&int64(irsdk.FlagGreen) == int64(irsdk.FlagGreen) {
			return "GREEN"
		} else if flags>>16&int64(irsdk.FlagYello) == int64(irsdk.FlagYello) {
			return "YELLOW"
		} else if flags>>16&int64(irsdk.FlagCheckered) == int64(irsdk.FlagCheckered) {
			return "CHECKERED"
		} else if flags>>16&int64(irsdk.FlagWhite) == int64(irsdk.FlagWhite) {
			return "WHITE"
		}
	} else if state == int32(irsdk.StateCheckered) {
		return "CHECKERED"
	} else if state == int32(irsdk.StateCoolDown) {
		return "CHECKERED"
	} else if state == int32(irsdk.StateGetInCar) {
		return "PREP"
	} else if state == int32(irsdk.StateParadeLaps) {
		return "PARADE"
	} else if state == int32(irsdk.StateInvalid) {
		return "INVALID"
	}
	return "NONE"

}

// returns true if we should record data
func shouldRecord(api *irsdk.Irsdk) bool {
	return slices.Contains([]string{"GREEN", "YELLOW", "CHECKERED"}, getRaceState(api))

}

func isRealDriver(d yaml.Drivers) bool {
	return d.IsSpectator == 0 && d.CarIsPaceCar == 0
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

func GetMetricUnit(s string) (float64, error) {
	re := regexp.MustCompile("(?P<value>[0-9.-]+)\\s+(?P<unit>.*)")

	if !re.Match([]byte(s)) {
		log.Error("invalid data with unit", log.String("data", s))
		return 0, ErrUnknownValueWithUnit
	}
	matches := re.FindStringSubmatch(s)
	value := matches[re.SubexpIndex("value")]
	unit := matches[re.SubexpIndex("unit")]
	if f, err := strconv.ParseFloat(value, 64); err != nil {

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
