// version.go：版本注入与判定——ldflags 注入 Version/Commit/Repo；IsDev 识别 dev 版。
package version

import "strings"

var (
	Version = "dev"
	Commit  = ""
	Repo    = "29anan29/Fly-lang"
)

func String() string {
	if IsDev() {
		if Commit != "" && len(Commit) >= 7 {
			return Version + " (" + Commit[:7] + ")"
		}
		return Version
	}
	return Version + " (release)"
}

func IsDev() bool {
	v := strings.ToLower(Version)
	return v == "" || v == "dev" || strings.Contains(v, "-dev")
}

func TrimTag(tag string) string {
	if len(tag) > 1 && (tag[0] == 'v' || tag[0] == 'V') {
		return tag[1:]
	}
	return tag
}
