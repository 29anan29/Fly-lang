package compile

import (
	"os"
	"os/exec"
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

// TestRuntimeGuard 验证 gen 注入的运行时兜底：通过 check 的代码，动态错误
// 以 FlyRuntimeError（携带源码行列号）统一抛出，而非裸 Python 异常。
func TestRuntimeGuard(t *testing.T) {
	src := `d = {}
k = "missing"
print(d[k])
`
	errs := CheckSource(src)
	if len(errs) > 0 {
		t.Fatalf("期望 check 通过: %v", errs)
	}
	code, errs, err := BuildSource(src)
	if len(errs) > 0 || err != nil {
		t.Fatalf("期望 build 通过: %v %v", errs, err)
	}
	if !strings.Contains(code, "_fly_get(d") {
		t.Fatalf("期望注入 _fly_get 兜底:\n%s", code)
	}
}

// TestRuntimeCatch 转译后实跑，验证动态错误转 FlyRuntimeError 且带行列号。
func TestRuntimeCatch(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 不可用")
	}
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"divzero", "x = 0\nprint(1 / x)\n", "FlyRuntimeError: src:2:9: 运算 truediv 失败"},
		{"keyerr", "d = {}\nprint(d['x'])\n", "FlyRuntimeError: src:2:8: 下标访问失败"},
		{"attrerr", "s = 'a'\nprint(s.missing)\n", "FlyRuntimeError: src:2:9: 属性访问 missing 失败"},
		{"cast", "print(int('xx'))\n", "FlyRuntimeError: src:1:10: 类型转换失败"},
		{"iter", "n = None\nfor x in n:\n    pass\n", "FlyRuntimeError: src:2:1: 不可迭代"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, errs, err := BuildSource(c.src)
			if len(errs) > 0 || err != nil {
				t.Fatalf("build 失败: %v %v", errs, err)
			}
			cmd := exec.Command("python3", "-c", code)
			out, _ := cmd.CombinedOutput()
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("期望输出含 %q，实际:\n%s", c.want, out)
			}
		})
	}
}

// TestSandboxEscape 验证运行时沙箱兜底：编译期无法静态确定的逃逸
// （变量下标反射名、白名单外模块）在运行时被 _fly_* / _fly_sb_import 拦截。
func TestSandboxEscape(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 不可用")
	}
	cases := []struct {
		name string
		src  string
		want string
	}{
		{"subclass_key", "x = []\nk = \"__subclasses__\"\nprint(x[k])\n", "沙箱: 禁止反射下标访问 __subclasses__"},
		{"class_key", "x = []\nk = \"__class__\"\nprint(x[k])\n", "沙箱: 禁止反射下标访问 __class__"},
		{"dict_key", "x = {}\nk = \"__dict__\"\nprint(x[k])\n", "沙箱: 禁止反射下标访问 __dict__"},
		{"setattr_key", "x = []\nk = \"__class__\"\nx[k] = 1\n", "沙箱: 禁止反射下标赋值 __class__"},
		{"func_param_key", "def f(k):\n    x = []\n    return x[k]\n\nprint(f(\"__bases__\"))\n", "沙箱: 禁止反射下标访问 __bases__"},
		{"escape_unicode_key", "x = []\nk = \"\\u005f_class__\"\nprint(x[k])\n", "沙箱: 禁止反射下标访问 __class__"},
		{"whiteout_mod", "import tkinter\nprint(\"loaded\")\n", "沙箱: 模块 tkinter 不在白名单"},
		{"modattr_indirect", "import logging\nm = logging\nprint(m.os)\n", "沙箱: 禁止访问模块属性 os"},
		{"modattr_indirect_attrgetter", "import operator\nm = operator\nprint(m.attrgetter)\n", "沙箱: 禁止访问模块属性 attrgetter"},
		{"allow_mod", "import math\nfrom math import sqrt as sq\nprint(sq(16))\n", "4.0"},
		{"allow_dangerous_name_var", "os = 5\nprint(os)\n", "5"},
		{"allow_modattr_safe", "import math\nimport json\nprint(math.floor(2.9), json.dumps([1]))\n", "2 [1]"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, errs, err := BuildSource(c.src)
			if len(errs) > 0 || err != nil {
				t.Fatalf("期望 check 通过（运行时兜底场景）: %v %v", errs, err)
			}
			cmd := exec.Command("python3", "-c", code)
			out, _ := cmd.CombinedOutput()
			if !strings.Contains(string(out), c.want) {
				t.Fatalf("期望输出含 %q，实际:\n%s", c.want, out)
			}
			if strings.Contains(c.want, "沙箱: ") && !strings.Contains(string(out), "[fly-sandbox] audit: ") {
				t.Fatalf("沙箱拦截未产生审计日志，实际:\n%s", out)
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
				line := strings.ReplaceAll(FormatError(f, d), f, base)
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
