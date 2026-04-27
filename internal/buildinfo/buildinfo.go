package buildinfo

type BuildInfo struct {
	Version   string
	Commit    string
	BuildDate string
}

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)

func Info() BuildInfo {
	return BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
	}
}
