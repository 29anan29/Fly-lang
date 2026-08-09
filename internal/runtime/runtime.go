package runtime

import (
	_ "embed"
	"strings"
)

//go:embed fly_runtime.py
var FlyRuntime string

// Section 按 "# fly:section:<name>" 标记提取运行时片段；未知节返回空串。
func Section(name string) string {
	var b strings.Builder
	in := false
	for _, ln := range strings.Split(FlyRuntime, "\n") {
		if strings.HasPrefix(ln, "# fly:section:") {
			in = strings.TrimPrefix(ln, "# fly:section:") == name
			continue
		}
		if in {
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}
