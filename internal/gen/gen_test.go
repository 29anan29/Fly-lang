// gen_test.go：gen 单元测试（golden 全量比对在 internal/compile/compile_test.go）。
package gen

import (
	"strings"
	"testing"

	"pyfly/internal/parser"
)

func generateSrc(t *testing.T, src string) string {
	t.Helper()
	p := parser.New(src)
	m := p.ParseModule()
	if d := p.Error(); d != nil {
		t.Fatalf("解析失败: %s", d)
	}
	return Generate(m)
}

// guard 嵌套在 only/trace/cage 内时 GuardError 必须注入（needs_guard 递归）。
func TestGuardInsideOnlyTraceCageInjectsGuardError(t *testing.T) {
	cases := []string{
		"only (json):\n    guard x: int, x > 0\n",
		"trace(level=\"INFO\"):\n    guard x: int, x > 0\n",
		"cage(max_time=\"1s\"):\n    guard x: int, x > 0\n",
	}
	for i, src := range cases {
		out := generateSrc(t, src)
		if !strings.Contains(out, "class GuardError") {
			t.Errorf("case %d: 期望注入 class GuardError，实际产物: %s", i, out)
		}
	}
}

// 增强赋值左值/下标只求值一次（重复求值回归：a[f()] += 1 只调用一次 f()）。
func TestAugAssignSingleEval(t *testing.T) {
	out := generateSrc(t, "def pickidx():\n    return 0\na = [1]\na[pickidx()] += 1\n")
	if !strings.Contains(out, "_fly_ab_2 = pickidx()") {
		t.Errorf("期望下标被提升为临时变量且只求值一次，实际: %s", out)
	}
	if strings.Contains(out, "_fly_get(a, pickidx()") {
		t.Errorf("读取路径必须复用临时变量，不得二次求值 pickidx(): %s", out)
	}
	if !strings.Contains(out, "_fly_get(_fly_aa_1, _fly_ab_2,") {
		t.Errorf("期望读取复用 _fly_ab_c 临时变量，实际: %s", out)
	}
	out = generateSrc(t, "class C:\n    pass\nc = C()\nc.x = 1\nc.x += 1\n")
	if strings.Contains(out, "c.x") {
		t.Errorf("属性增强赋值不应重复求值 c，实际: %s", out)
	}
}
