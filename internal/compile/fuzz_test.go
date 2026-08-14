package compile

import (
	"os"
	"path/filepath"
	"testing"
)

// FuzzCheckSource 覆盖完整管线：lexer → parser → checker（含 escape 全局扫描、污点、only/lock/guard 等）。
// 安全声明：任何输入都不应 panic；panic 即安全缺口（攻击者可用畸形输入打崩编译器）。
func FuzzCheckSource(f *testing.F) {
	seeds := []string{
		"safe x = input()\nprint(x)\n",
		"only os:\n  import os\n  os.system('ls')\n",
		"cage(max_time=\"5s\", max_memory=\"100MB\"):\n  while True: pass\n",
		"guard x > 0:\n  y = x\n",
		"seal class Admin:\n  def __init__(self): self.role = 'admin'\n",
		"lock X = 1\nX = 2\n",
		"mask p = getpass()\nprint(p)\n",
		"eval(input())\n",
		"import pickle\npickle.loads(input())\n",
		"x = input()\nf\"{x}\"\n",
		"from builtins import eval as e\ne('1')\n",
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
		CheckSource(src)
	})
}
