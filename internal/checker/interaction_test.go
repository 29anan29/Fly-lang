package checker

import (
	"strings"
	"testing"

	"pyfly/internal/parser"
)

// interactionCase：关键字交互组合回归（对应 docs/关键字交互矩阵.md）。
// 目标：任何两个/三个关键字组合的语义漂移都会在此暴露。
type interactionCase struct {
	name string
	src  string
	want string // 空 = 期望放行
	note string
}

func TestKeywordInteractions(t *testing.T) {
	cases := []interactionCase{
		// ---- guard × 其余（guard 为声明式：guard <条件> / guard <name>: <type>）----
		{
			name: "guard+lock",
			src:  "lock X = 1\nguard X > 0\nX = 2\n",
			want: "lock 变量 X 不可再赋值",
			note: "guard 声明后修改 lock 变量仍被拦",
		},
		{
			name: "guard+safe 条件引用",
			src:  "safe data\nguard len(data) > 0\nprint(len(data))\n",
			want: "",
			note: "guard 条件可读 safe 变量（值校验不污点）",
		},
		{
			name: "guard+safe 汇点",
			src:  "safe data\nguard data != ''\neval(data)\n",
			want: "危险汇点",
			note: "guard 不构成清洗，safe 污点仍达 eval 汇点",
		},
		{
			name: "guard+mask",
			src:  "mask p\np = 'secret'\nguard len(p) > 0\nprint(p)\n",
			want: "敏感数据",
			note: "guard 声明后输出 mask 数据仍拦",
		},
		{
			name: "guard+trace",
			src:  "x = 1\nguard x > 0\ntrace(level=\"DEBUG\"):\n    def f():\n        return x\n",
			want: "",
			note: "guard 与 trace 同文件共存互不干扰",
		},
		{
			name: "guard 类型声明+seal 类共存",
			src:  "seal class Admin:\n    def __init__(self, n):\n        self.name = n\nage = 25\nguard age: int\nadmin = Admin('a')\n",
			want: "",
			note: "guard 类型声明与 seal 类定义共存",
		},

		// ---- only × 其余 ----
		{
			name: "only 内 seal 实例赋值",
			src:  "seal class Admin:\n    def __init__(self, n):\n        self.name = n\nadmin = Admin('a')\nonly (json):\n    admin.role = 'x'\n",
			want: "不可修改",
			note: "only 块内 seal 实例属性赋值仍被拦",
		},
		{
			name: "only 内 lock 赋值",
			src:  "lock X = 1\nonly (json):\n    X = 2\n",
			want: "lock 变量 X 不可再赋值",
			note: "only 块内修改 lock 变量仍被拦",
		},
		{
			name: "only 内 safe 汇点",
			src:  "safe data\nonly (json):\n    eval(data)\n",
			want: "危险汇点",
			note: "only 块内 safe 污点仍达 eval（白名单不豁免污点）",
		},
		{
			name: "only 内危险名访问",
			src:  "only (json):\n    import os\n",
			want: "only 块禁止访问",
			note: "only 白名单外 import 被拦",
		},
		{
			name: "only+safe 白名单内操作",
			src:  "only (json):\n    x = json.dumps({'a': 1})\n",
			want: "",
			note: "白名单模块正常使用放行",
		},

		// ---- safe × mask 独立性 ----
		{
			name: "safe 变量 print 放行",
			src:  "safe data\nprint(len(data))\n",
			want: "",
			note: "safe 只管危险汇点，不管输出上下文",
		},
		{
			name: "mask 变量流入 print 拦截",
			src:  "mask p\np = 'secret'\nprint(p)\n",
			want: "敏感数据",
			note: "mask 输出上下文拦截",
		},

		// ---- seal × 其余 ----
		{
			name: "seal 类 + trace 方法",
			src:  "seal class Admin:\n    def __init__(self, n):\n        self.name = n\n    trace(level=\"INFO\"):\n        def get(self):\n            return self.name\n",
			want: "",
			note: "seal 类内 trace 方法可共存",
		},
		{
			name: "seal 属性赋值拦截",
			src:  "seal class Admin:\n    def __init__(self, n):\n        self.name = n\nadmin = Admin('a')\nadmin.name = 'b'\n",
			want: "不可修改",
			note: "seal 实例属性静态拦截",
		},

		// ---- lock × 其余 ----
		{
			name: "lock+mask 变量叠加",
			src:  "mask p\np = 'secret'\nlock p\nprint(p)\n",
			want: "敏感数据",
			note: "mask 变量可再 lock：lock 锁赋值，mask 拦输出，双重约束叠加",
		},
		{
			name: "lock+safe 变量叠加",
			src:  "safe data\nlock data\neval(data)\n",
			want: "危险汇点",
			note: "safe 变量可再 lock：lock 锁赋值，safe 拦汇点，双重约束叠加",
		},
		{
			name: "lock 变量流入 mask 输出",
			src:  "lock X = 'v'\nmask X\nprint(X)\n",
			want: "敏感数据",
			note: "lock 变量再声明 mask 合法：lock 后输出受 mask 约束",
		},

		// ---- 三元组 ----
		{
			name: "guard+safe+mask 三合一",
			src:  "safe data\nmask p\np = data\nguard len(p) > 0\neval(p)\nprint(p)\n",
			want: "危险汇点",
			note: "p 双污点：eval 拦 safe（E0031）、print 拦 mask（E0032），互不掩蔽",
		},
		{
			name: "only 内 seal+trace",
			src:  "seal class Admin:\n    def __init__(self, n):\n        self.name = n\nadmin = Admin('a')\nonly (json):\n    admin.role = 'x'\ntrace(level=\"INFO\"):\n    def get(self):\n        return self.name\n",
			want: "不可修改",
			note: "only 内 seal 赋值拦截优先",
		},
		{
			name: "cage 内 only+safe",
			src:  "safe data\ncage(max_time=\"1s\"):\n    def f():\n        only (json):\n            eval(data)\n",
			want: "危险汇点",
			note: "cage 内 only 块内 safe 污点仍达汇点",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := parser.New(tc.src)
			m := p.ParseModule()
			if d := p.Error(); d != nil {
				t.Fatalf("解析失败: %s", d)
			}
			errs := Check(m)
			if tc.want == "" {
				if len(errs) != 0 {
					t.Fatalf("期望放行（%s），实际: %v", tc.note, errs)
				}
				return
			}
			for _, e := range errs {
				if strings.Contains(e.Msg, tc.want) {
					return
				}
			}
			t.Fatalf("期望拦截 %q（%s），实际: %v", tc.want, tc.note, errs)
		})
	}
}
