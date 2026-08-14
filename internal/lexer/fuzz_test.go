// fuzz_test.go：FuzzLexer 模糊测试（go test -fuzz FuzzLexer）。
package lexer

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzLexer(f *testing.F) {
	seeds := []string{
		"safe x = input()\nprint(x)\n",
		"only os:\n  import os\n  os.system('ls')\n",
		"cage(max_time=\"5s\", max_memory=\"100MB\"):\n  while True: pass\n",
		"guard x > 0:\n  y = x\n",
		"seal class Admin:\n  def __init__(self): self.role = 'admin'\n",
		"f\"prefix {x + 1} suffix\"\n",
		"'''docstring'''\n# comment\nx = 3.14e-10\n",
		"if True:\n\tpass\nelif x := 1:\n\tpass\nelse:\n\tpass\n",
		"async def f():\n  await g()\n",
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
		l := New(src)
		for {
			tok := l.Next()
			if tok.Type == EOF || l.Err() != nil {
				break
			}
		}
	})
}
