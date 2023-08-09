package util

import (
	"strings"

	"golang.org/x/mod/semver"
)

const (
	RequiredServerVersion string = "v0.6.0"
)

func CheckServerVersion(toCheck string) bool {
	if !strings.HasPrefix(toCheck, "v") {
		toCheck = "v" + toCheck
	}
	res := semver.Compare(toCheck, RequiredServerVersion)
	return res >= 0
}
