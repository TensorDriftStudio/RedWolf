package version

import (
	"fmt"
	"runtime"
)

var (
	// Version is the current SemVer release of RedWolf
	Version = "v1.1.0"
	// GitCommit is injected via -ldflags during build
	GitCommit = "dev"
	// BuildDate is injected via -ldflags during build
	BuildDate = "unknown"
	// Edition describes the software distribution tier
	Edition = "RedWolf Enterprise Bare-Metal Edition"
)

// Info encapsulates all build and release metadata.
type Info struct {
	Version   string `json:"version"`
	GitCommit string `json:"gitCommit"`
	BuildDate string `json:"buildDate"`
	Edition   string `json:"edition"`
	GoVersion string `json:"goVersion"`
	Platform  string `json:"platform"`
}

// Get returns the structured version metadata.
func Get() Info {
	return Info{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		Edition:   Edition,
		GoVersion: runtime.Version(),
		Platform:  fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}
}
