package analyze

import "testing"

func TestMetrics(t *testing.T) {
	src := `def f(x):
    # 注释行
    if x > 0:
        for i in range(3):
            try:
                g(x)
            except ValueError:
                pass
    return x


def g(y):
    return y + 1
`
	m := Analyze(src)
	if m == nil {
		t.Fatal("Analyze 返回 nil")
	}
	if m.Cyclomatic != 1+1+1+1+1 { // base + if + for + try + except
		t.Fatalf("循环复杂度 = %d，期望 6", m.Cyclomatic)
	}
	if m.FuncCount != 2 {
		t.Fatalf("函数数 = %d，期望 2", m.FuncCount)
	}
	if m.TryCount != 1 || m.RaiseCount != 0 {
		t.Fatalf("try=%d raise=%d", m.TryCount, m.RaiseCount)
	}
	if m.MaxParams != 1 {
		t.Fatalf("MaxParams = %d", m.MaxParams)
	}
	if m.CommentRate <= 0 {
		t.Fatalf("注释比例应为正: %v", m.CommentRate)
	}
	if len(m.Functions) != 2 {
		t.Fatalf("Functions 应含 2 个: %+v", m.Functions)
	}
	f := m.Functions[0]
	if f.Cyclo != 4 || f.Nest != 4 || f.Cognit != 13 {
		t.Fatalf("f 指标不符: %+v", f)
	}
	if s, _ := Score(m); s < 0 || s > 100 {
		t.Fatalf("评分越界: %v", s)
	}
}

func TestAnalyzeRejectsSyntaxError(t *testing.T) {
	if m := Analyze("def f(:\n    pass\n"); m != nil {
		t.Fatal("语法错误文件应返回 nil")
	}
}
