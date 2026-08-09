package parser

import (
	"testing"
)

func parseOK(t *testing.T, src string) {
	t.Helper()
	p := New(src)
	p.ParseModule()
	if d := p.Error(); d != nil {
		t.Fatalf("解析失败: %s", d)
	}
}

func parseErr(t *testing.T, src string) {
	t.Helper()
	p := New(src)
	p.ParseModule()
	if d := p.Error(); d == nil {
		t.Fatalf("期望解析失败，实际通过")
	}
}

func TestStatements(t *testing.T) {
	parseOK(t, "x = 1\n")
	parseOK(t, "a = b = c = 1\n")
	parseOK(t, "a, b = 1, 2\n")
	parseOK(t, "a, *rest = lst\n")
	parseOK(t, "a[0], d.k = 1, 2\n")
	parseOK(t, "x += 1\ny **= 2\n")
	parseOK(t, "del a, b[0]\n")
	parseOK(t, "import os.path as p, json\n")
	parseOK(t, "from .utils import helper as h, *\n")
	parseOK(t, "pass\nbreak\ncontinue\n")
	parseOK(t, "return\nreturn x, y\nraise\nraise E() from cause\n")
}

func TestFunctions(t *testing.T) {
	parseOK(t, "def f():\n    pass\n")
	parseOK(t, "def f(a, b=1, *args, c=2, **kw):\n    return a\n")
	parseOK(t, "def f(a: int, b: str = 'x') -> bool:\n    pass\n")
	parseOK(t, "def f(*, x):\n    pass\n")
	parseOK(t, "@decorator\n@other(1)\ndef f():\n    pass\n")
}

func TestClasses(t *testing.T) {
	parseOK(t, "class A:\n    pass\n")
	parseOK(t, "class A(B, C.mix):\n    x = 1\n    def m(self):\n        pass\n")
	parseOK(t, "@dec\nclass A:\n    pass\n")
}

func TestControlFlow(t *testing.T) {
	parseOK(t, "if x:\n    pass\nelif y:\n    pass\nelse:\n    pass\n")
	parseOK(t, "for i in range(10):\n    pass\nelse:\n    pass\n")
	parseOK(t, "while x < 10:\n    break\nelse:\n    pass\n")
	parseOK(t, "try:\n    pass\nexcept ValueError as e:\n    pass\nexcept (TypeError, KeyError):\n    pass\nelse:\n    pass\nfinally:\n    pass\n")
}

func TestExpressions(t *testing.T) {
	parseOK(t, "x = 1 + 2 * 3 ** 4 // 5 % 6\n")
	parseOK(t, "x = a or b and not c\n")
	parseOK(t, "x = a < b <= c is not d in e not in f\n")
	parseOK(t, "x = a if b else c if d else e\n")
	parseOK(t, "x = f(a, b=1, *args, **kw)\n")
	parseOK(t, "x = a.b[0][1:2:3][:].c\n")
	parseOK(t, "x = [i for i in range(10) if i % 2]\n")
	parseOK(t, "x = [i * j for i in r1 for j in r2 if i if j]\n")
	parseOK(t, "x = {1, 2, 3}\nx = {'a': 1, 'b': 2}\nx = {}\n")
	parseOK(t, "x = (1,)\nx = (1, 2)\nx = () \n")
	parseOK(t, "x = -y ** 2\nx = ~a & b | c ^ d << e >> f\n")
	parseOK(t, "x = 'a' 'b' f'c {x}'\n")
	parseOK(t, "x = ...\n")
}

func TestSemicolons(t *testing.T) {
	parseOK(t, "a = 1; b = 2; c = 3\n")
	parseOK(t, "if x: a = 1; b = 2\n")
	parseOK(t, "a = 1;\n")
}

func TestFlyKeywordsRejected(t *testing.T) {
	parseErr(t, "cage(max_time='1s'):\n    pass\n")
}

func TestOnlyStmt(t *testing.T) {
	parseOK(t, "only (json, math):\n    pass\n")
	parseOK(t, "only (json):\n    def parse(raw):\n        return json.loads(raw)\n")
	parseErr(t, "only json:\n    pass\n")
}

func TestSealStmt(t *testing.T) {
	parseOK(t, "seal class Admin:\n    role = 'admin'\n")
	parseErr(t, "seal def f():\n    pass\n")
}

func TestTraceStmt(t *testing.T) {
	parseOK(t, "trace(level='WARN', args=True, ret=True):\n    def delete_user(uid):\n        db.delete(uid)\n")
	parseOK(t, "trace():\n    def f():\n        pass\n")
	parseErr(t, "trace(level=1):\n    pass\n")
	parseErr(t, "trace(foo=1):\n    pass\n")
}

func TestTaintDeclStmt(t *testing.T) {
	parseOK(t, "safe uid\n")
	parseOK(t, "safe uid, name\n")
	parseOK(t, "def f():\n    mask password\n")
	parseOK(t, "mask token, key\n")
	parseErr(t, "safe 123\n")
	parseErr(t, "mask\n")
}

func TestLockStmt(t *testing.T) {
	parseOK(t, "lock SECRET = 'abc'\n")
	parseOK(t, "x = 1\nlock x\n")
	parseErr(t, "lock = 1\n")
	parseErr(t, "lock\n")
}

func TestGuardStmt(t *testing.T) {
	parseOK(t, "def f(age):\n    guard age: int, 0 < age < 150\n")
	parseOK(t, "def f(username):\n    guard username: str, len(username) > 0\n")
	parseOK(t, "def f(b):\n    guard b != 0\n")
	parseOK(t, "def f(x):\n    guard x\n")
	parseOK(t, "def f(x, y):\n    guard x, y > 0\n")
	parseErr(t, "guard x:\n")
}

func TestUnsupportedKeywords(t *testing.T) {
	parseErr(t, "with open('f') as fp:\n    pass\n")
	parseErr(t, "assert x\n")
	parseErr(t, "global x\n")
	parseErr(t, "x = lambda y: y\n")
}

func TestSyntaxErrors(t *testing.T) {
	parseErr(t, "x = \n")
	parseErr(t, "def f(:\n    pass\n")
	parseErr(t, "x = (1 +\n")
	parseErr(t, "if x\n    pass\n")
	parseErr(t, "for i range(3):\n    pass\n")
	parseErr(t, "x = [1, 2\n")
	parseErr(t, "x = {1: 2\n")
	parseErr(t, "def f():\n    x = 1\n      y = 2\n")
	parseErr(t, "x = a. \n")
	parseErr(t, "x = f(a b)\n")
}

func TestEmptyModule(t *testing.T) {
	p := New("")
	m := p.ParseModule()
	if p.Error() != nil {
		t.Fatalf("空文件报错: %s", p.Error())
	}
	if len(m.Stmts) != 0 {
		t.Fatalf("空文件应有 0 条语句")
	}
}

func TestDocstring(t *testing.T) {
	p := New("def f():\n    \"doc\"\n    return 1\n")
	m := p.ParseModule()
	if p.Error() != nil {
		t.Fatalf("docstring 解析失败: %s", p.Error())
	}
	if len(m.Stmts) != 1 {
		t.Fatalf("应有 1 条语句")
	}
}
