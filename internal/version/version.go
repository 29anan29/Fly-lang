package version

var (
	Version = "dev"
	Commit  = ""
	Repo    = "29anan29/Fly-lang"
)

func String() string {
	if Commit != "" && len(Commit) >= 7 {
		return Version + " (" + Commit[:7] + ")"
	}
	return Version
}

func TrimTag(tag string) string {
	if len(tag) > 1 && (tag[0] == 'v' || tag[0] == 'V') {
		return tag[1:]
	}
	return tag
}
