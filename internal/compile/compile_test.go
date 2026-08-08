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
