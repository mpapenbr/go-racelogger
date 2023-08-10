package version

import (
	"fmt"
	"runtime"
)

// GitCommit returns the git commit that was compiled.
// Note: will be replaced by goreleaser
var GitCommit string

// Version returns the main version number that is being run at the moment.
// Note: will be replaced by goreleaser
var Version = "1.0.0-DEV"

// BuildDate returns the date the binary was built
// Note: will be replaced by goreleaser
var BuildDate = ""

// GoVersion returns the version of the go runtime used to compile the binary
var GoVersion = runtime.Version()

// OsArch returns the os and arch used to build the binary
var OsArch = fmt.Sprintf("%s %s", runtime.GOOS, runtime.GOARCH)

// FullVersion can be used for more detailed version info
var FullVersion = fmt.Sprintf("%s Build %s (Commit %s) Go %s [%s]",
	Version, BuildDate, GitCommit, GoVersion, OsArch)
