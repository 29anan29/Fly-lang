// fuzz_test.go：FuzzParse 模糊测试（go test -fuzz FuzzParse，种子 = testdata 全量）。
package parser

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzParse(f *testing.F) {
	seeds := []string{
		"safe x = input()\nprint(x)\n",
		"only os:\n  import os\n  os.system('ls')\n",
		"cage(max_time=\"5s\", max_memory=\"100MB\"):\n  while True: pass\n",
		"guard x > 0:\n  y = x\n",
		"seal class Admin:\n  def __init__(self): self.role = 'admin'\n",
		"lock X = 1\nlock Y\n",
		"trace func f(x: int) -> int:\n  return x\n",
		"f\"prefix {x + 1} suffix\"\n",
		"import a.b as c\nfrom x import y as z\n",
		"class A(B, C):\n  def m(self, *args, **kw): return super().m(*args, **kw)\n",
		"[x for x in range(10) if x % 2]\n{x: y for x, y in zip(a, b)}\n",
		"try:\n  raise ValueError('x')\nexcept (TypeError, ValueError) as e:\n  pass\nfinally:\n  pass\n",
		"",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	if matches, err := filepath.Glob("../../testdata/**/*.fly"); err == nil {
		for _, m := range matches {
			if b, err := os.ReadFile(m); err == nil {
				f.Add(string(b))
			}
		}
	}
	f.Fuzz(func(t *testing.T, src string) {
		p := New(src)
		p.ParseModule()
		if p.Error() != nil {
			return
		}
	})
}
