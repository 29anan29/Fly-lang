package checker

import (
	"strings"
	"testing"

	"flylang/internal/parser"
)

func checkSrc(t *testing.T, src string) []string {
	t.Helper()
	p := parser.New(src)
	m := p.ParseModule()
	if d := p.Error(); d != nil {
		t.Fatalf("解析失败: %s", d)
	}
	errs := Check(m)
	var msgs []string
	for _, e := range errs {
		msgs = append(msgs, e.Msg)
	}
	return msgs
}

func wantErr(t *testing.T, src string, want string) {
	t.Helper()
	msgs := checkSrc(t, src)
	for _, m := range msgs {
		if strings.Contains(m, want) {
			return
		}
	}
	t.Fatalf("期望错误包含 %q，实际: %v", want, msgs)
}

func noErr(t *testing.T, src string) {
	t.Helper()
	if msgs := checkSrc(t, src); len(msgs) > 0 {
		t.Fatalf("期望无错误，实际: %v", msgs)
	}
}

func TestLockNoErrors(t *testing.T) {
	noErr(t, "lock SECRET = 'abc'\nx = SECRET\nprint(SECRET)\n")
	noErr(t, "x = 1\nlock x\n")
	noErr(t, "lock a = 1\nlock b = 2\ndef f():\n    return a + b\n")
}

func TestLockMutations(t *testing.T) {
	wantErr(t, "lock X = 1\nX = 2\n", "lock 变量 X 不可再赋值")
	wantErr(t, "lock X = 1\nX += 1\n", "lock 变量 X 不可再赋值")
	wantErr(t, "lock X = 1\nX, y = 1, 2\n", "lock 变量 X 不可再赋值")
	wantErr(t, "lock X = 1\ndel X\n", "lock 变量 X 不可删除")
	wantErr(t, "lock X = 1\nfor X in [1, 2]:\n    pass\n", "lock 变量 X 不可再赋值")
}

func TestLockFunctionShadow(t *testing.T) {
	wantErr(t, "lock SECRET = 'a'\ndef hack():\n    SECRET = 'b'\n", "lock 变量 SECRET 不可再赋值")
}

func TestLockReflection(t *testing.T) {
	wantErr(t, "lock S = 'x'\nprint(globals()['S'])\n", "不可通过 globals() 反射读取")
	wantErr(t, "lock S = 'x'\nprint(vars()['S'])\n", "不可通过 vars() 反射读取")
	wantErr(t, "lock S = 'x'\nsetattr(S, 'k', 1)\n", "不可通过 setattr 修改")
	wantErr(t, "lock S = 'x'\nsetattr(globals(), 'S', 'y')\n", "不可通过 setattr 修改")
	wantErr(t, "lock S = 'x'\nglobals()['S'] = 'y'\n", "不可通过反射修改")
	noErr(t, "lock S = 'x'\nprint(S)\n")
	noErr(t, "print(globals()['OTHER'])\n")
}

func TestLockUndefinedBare(t *testing.T) {
	wantErr(t, "lock NOPE\n", "lock 变量 NOPE 未定义")
	noErr(t, "def f():\n    lock z = 1\n    return z\n")
}

func TestGuardNoErrors(t *testing.T) {
	noErr(t, "def f(age: int):\n    guard age: int, 0 < age < 150\n    return age\n")
	noErr(t, "def f(username):\n    guard username: str, len(username) > 0\n    return username\n")
	noErr(t, "def f(b):\n    guard b != 0\n    return b\n")
	noErr(t, "def f(x):\n    guard x\n    return x\n")
	noErr(t, "def f(age):\n    guard age: int\n    return age\n")
	noErr(t, "limit = 10\ndef f(age):\n    guard age: int, 0 < age < limit\n    return age\n")
	noErr(t, "def f(items):\n    guard len(items) > 0\n    return items[0]\n")
	noErr(t, "def f(x):\n    if x:\n        guard x > 0\n    return x\n")
}

func TestGuardErrors(t *testing.T) {
	wantErr(t, "def f():\n    guard age: int\n", "guard 变量 age 未定义")
	wantErr(t, "def f(age: str):\n    guard age: int\n", "guard 类型 int 与参数注解 str 不一致")
	wantErr(t, "def f(age):\n    guard age: 0 < age\n", "guard 类型必须是简单类型名")
	wantErr(t, "def f(age):\n    guard age: list[int]\n", "guard 类型必须是简单类型名")
	wantErr(t, "def f(age):\n    guard age: int, 0 < age < limit\n", "guard 条件中引用了未定义的名字 limit")
	wantErr(t, "def f(age):\n    guard age: int, 0 < age < unknown(x)\n", "guard 条件中引用了未定义的名字 unknown")
}

func TestGuardClassMethod(t *testing.T) {
	noErr(t, "class User:\n    def set_age(self, age: int):\n        guard age: int, 0 < age < 150\n        self.age = age\n")
}

func TestErrorAggregationCap(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("lock X = 1\n")
	for i := 0; i < 30; i++ {
		sb.WriteString("X = ")
		sb.WriteString(string(rune('a' + i%26)))
		sb.WriteString("\n")
	}
	msgs := checkSrc(t, sb.String())
	if len(msgs) != maxErrs {
		t.Fatalf("期望错误数 = %d，实际 %d", maxErrs, len(msgs))
	}
}

func TestSafeTaint(t *testing.T) {
	wantErr(t, "safe uid\neval(uid)\n", "未净化的外部输入 uid 流入 eval")
	wantErr(t, "safe uid\nexec(uid)\n", "未净化的外部输入 uid 流入 exec")
	wantErr(t, "safe uid\nos.system(uid)\n", "未净化的外部输入 uid 流入 os.system")
	wantErr(t, "safe cmd\nsubprocess.run(cmd)\n", "未净化的外部输入 cmd 流入 subprocess.run")
	wantErr(t, "safe sql\ncursor.execute(sql)\n", "未净化的外部输入 sql 流入 execute")
	wantErr(t, "safe q\ndb.execute(q)\n", "未净化的外部输入 q 流入 execute")
}

func TestSafeSanitize(t *testing.T) {
	noErr(t, "safe uid\nclean = int(uid)\neval(clean)\n")
	noErr(t, "safe uid\nclean = float(uid)\nreturn clean\n")
	noErr(t, "uid = request.args.get('id')\nsafe uid\nclean = int(uid)\ndb.query(clean)\n")
}

func TestSafeFlow(t *testing.T) {
	wantErr(t, "safe uid\na = uid\nb = a\neval(b)\n", "未净化的外部输入")
	wantErr(t, "safe uid\na = [uid]\nb = a[0]\neval(b)\n", "未净化的外部输入")
	wantErr(t, "safe x\ns = x + 'y'\neval(s)\n", "未净化的外部输入")
}

func TestMaskTaint(t *testing.T) {
	wantErr(t, "mask pw\nprint(pw)\n", "敏感数据 pw 不可流入 print")
	wantErr(t, "def login(password):\n    mask password\n    print(password)\n", "敏感数据 password 不可流入 print")
	wantErr(t, "mask token\nlogging.info(token)\n", "敏感数据 token 不可流入 logging")
	wantErr(t, "mask pw\nlogging.error('bad %s', pw)\n", "敏感数据 pw 不可流入 logging")
}

func TestMaskAllowed(t *testing.T) {
	noErr(t, "def login(password):\n    mask password\n    hashed = hash(password)\n    return hashed\n")
	noErr(t, "def login(password):\n    mask password\n    if password == 'secret':\n        return True\n    return False\n")
	noErr(t, "def login(password):\n    mask password\n    return len(password) > 8\n")
}

func TestMaskFlow(t *testing.T) {
	wantErr(t, "mask pw\nmsg = 'pw: ' + pw\nprint(msg)\n", "敏感数据")
	wantErr(t, "def login(pw):\n    mask pw\n    user = pw\n    print(user)\n", "敏感数据")
}

func TestFStringMask(t *testing.T) {
	wantErr(t, "mask pw\nprint(f'密码: {pw}')\n", "敏感数据 pw 不可流入 print")
	wantErr(t, "def login(password):\n    mask password\n    print(f'密码: {password}')\n", "敏感数据 password 不可流入 print")
	noErr(t, "def f(n):\n    print(f'count: {n}')\n")
}

func TestFunctionReturnTaint(t *testing.T) {
	wantErr(t, "def get_input():\n    safe uid\n    return uid\neval(get_input())\n", "未净化的外部输入")
	noErr(t, "def get_input():\n    safe uid\n    return int(uid)\neval(get_input())\n")
}

func TestTaintSourceInput(t *testing.T) {
	wantErr(t, "x = input()\neval(x)\n", "未净化的外部输入")
	noErr(t, "x = input()\nx = int(x)\neval(x)\n")
	wantErr(t, "s = os.environ['HOME']\neval(s)\n", "未净化的外部输入")
}
