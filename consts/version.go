package consts

// These will be injected via -ldflags at build time
var (
	gitSha string = "unknown"
	gitTag string = "unknown"
)

func GetBuildInfo() map[string]string {
	return map[string]string{
		"revision": gitSha,
		"version":  gitTag,
	}
}
