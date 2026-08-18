package checker

import (
	"os"
	"path/filepath"
	"testing"

	"pyfly/internal/parser"
)

// FuzzSecurityCheck 专项覆盖安全边界路径：escape 全局扫描、污点传播、
// only/lock/guard/cage/seal 检查。任何输入都不应 panic——panic 即安全缺口
// （攻击者可用畸形输入打崩编译管线，绕过诊断）。
func FuzzSecurityCheck(f *testing.F) {
	seeds := []string{
		"x = ().__class__.__bases__[0].__subclasses__()\n",
		"__builtins__[\"__import__\"](\"os\")\n",
		"only json:\n  __builtins__.__import__('os')\n",
		"only os:\n  import os\n  os.system('ls')\n",
		"only json, math:\n  def f(a=b):\n    return a\n",
		"def f(a=os.system):\n  pass\n",
		"import gc\ngc.collect()\n",
		"from random import _os\n_os.system('x')\n",
		"a[input()] += 1\n",
		"obj.attr += eval('1')\n",
		"cage(max_time=\"1s\"):\n  guard x > 0:\n    y = x\n",
		"only json:\n  cage(max_time=\"1s\"):\n    trace:\n      z = 1\n",
		"f\"{eval('1')}\"\n",
		"getattr(obj, \"__class__\")\n",
		"mask p = getpass()\nprint(p)\n",
		"lock X = 1\nX = 2\n",
		"seal class A:\n  def __init__(self):\n    self.r = 1\n",
		"safe x = input()\nprint(x)\n",
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
		p := parser.New(src)
		m := p.ParseModule()
		if p.Error() != nil {
			return
		}
		Check(m)
	})
}