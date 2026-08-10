package compile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGolden(t *testing.T) {
	files, err := filepath.Glob("../../testdata/golden/*.fly")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		base := strings.TrimSuffix(f, ".fly")
		golden := base + ".py"
		t.Run(filepath.Base(base), func(t *testing.T) {
			code, errs, err := BuildFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if len(errs) > 0 {
				for _, d := range errs {
					t.Errorf("%s", FormatError(f, d))
				}
				t.FailNow()
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatal(err)
			}
			if code != string(want) {
				t.Errorf("转译输出与 golden 不一致\n--- got ---\n%s\n--- want ---\n%s", code, want)
			}
		})
	}
}

func TestErrors(t *testing.T) {
	files, err := filepath.Glob("../../testdata/errors/*.fly")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			errs, err := CheckFile(f)
			if err != nil {
				t.Fatal(err)
			}
			if len(errs) == 0 {
				t.Errorf("%s 期望编译报错，实际通过", f)
			}
		})
	}
}

func TestErrorSnapshots(t *testing.T) {
	files, err := filepath.Glob("../../testdata/errors/*.fly")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			errs, err := CheckFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var got strings.Builder
			base := filepath.Base(f)
			for _, d := range errs {
				line := FormatError(f, d)
				line = line[strings.LastIndex(line, base):]
				got.WriteString(line)
				got.WriteString("\n")
			}
			want, err := os.ReadFile(strings.TrimSuffix(f, ".fly") + ".err")
			if err != nil {
				t.Fatal(err)
			}
			if got.String() != string(want) {
				t.Errorf("错误消息与快照不一致\n--- got ---\n%s--- want ---\n%s", got.String(), want)
			}
		})
	}
}
