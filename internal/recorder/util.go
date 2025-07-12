package recorder

import "github.com/mpapenbr/goirsdk/irsdk"

//nolint:whitespace // editor/linter issue
func computeNameAndDescription(cliNames, cliDescr []string, idx int) (
	name, description string,
) {
	name = ""
	description = ""
	if len(cliNames) > 0 {
		nameIdx := idx
		if nameIdx >= len(cliNames) {
			nameIdx = len(cliNames) - 1 // use last name if index is out of range
		}
		name = cliNames[nameIdx]
	}

	if len(cliDescr) > 0 {
		descrIdx := idx
		if descrIdx >= len(cliDescr) {
			descrIdx = len(cliDescr) - 1 // use last description if index is out of range
		}
		description = cliDescr[descrIdx]
	}
	return name, description
}

func HasValidAPIData(api *irsdk.Irsdk) bool {
	api.GetData()
	return len(api.GetValueKeys()) > 0 && hasPlausibleYaml(api)
}

// the yaml data is considered valid if certain plausible values are present.
// for example: the track length must be > 0, track sectors are present
func hasPlausibleYaml(api *irsdk.Irsdk) bool {
	ret := true
	y, err := api.GetYaml()
	if err != nil {
		return false
	}
	if y.WeekendInfo.NumCarTypes == 0 {
		ret = false
	}
	if y.WeekendInfo.TrackID == 0 {
		ret = false
	}
	if len(y.SplitTimeInfo.Sectors) == 0 {
		ret = false
	}
	if len(y.SessionInfo.Sessions) == 0 {
		ret = false
	}
	return ret
}
