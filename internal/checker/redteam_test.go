package checker

import (
	"strings"
	"testing"

	"pyfly/internal/parser"
)

// redTeamCase：对抗方式回归表（对应 docs/THREAT-MODEL.md §5 实测表）。
// want 非空 = 必须拦截该错误子串；want 为空 = 已知边界（允许放行，由沙箱兜底，见 note）。
// 任何一项行为漂移（该拦的不拦、该放行的开始报错）都会在此暴露。
type redTeamCase struct {
	name string
	src  string
	want string
	note string
}

func TestRedTeamEscape(t *testing.T) {
	cases := []redTeamCase{
		{
			name: "pickle 别名 import 溯源",
			src:  "import pickle as p\nsafe data\np.loads(data)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 R3 别名溯源",
		},
		{
			name: "from import 别名溯源",
			src:  "from pickle import loads as l\nsafe data\nl(data)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 R3",
		},
		{
			name: "列表容器传播",
			src:  "safe data\na = [data]\nb = a[0]\neval(b)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 R2 容器传播",
		},
		{
			name: "字典容器传播",
			src:  "safe data\nd = {}\nd['k'] = data\neval(d['k'])\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 R2",
		},
		{
			name: "属性赋值传播",
			src:  "safe data\nclass C:\n    pass\nc = C()\nc.x = data\neval(c.x)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 R2 x.attr 传播（属性名级精确跟踪）",
		},
		{
			name: "int 显式清洗放行",
			src:  "safe data\nn = int(data)\nprint(n)\n",
			want: "",
			note: "THREAT-MODEL §5 R4 清洗即开发者承诺（eval 本身被 E0063 全局禁，此处用 print/mask 无关汇点验证清洗）",
		},
		{
			name: "eval 动态拼接",
			src:  "safe x\ny = '1' + x\neval(y)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 动态拼接",
		},
		{
			name: "间接引用一等函数（B7 已堵死）",
			src:  "import pickle\no = pickle.loads\no(data)\n",
			want: "禁止导入危险模块 pickle",
			note: "B7 间接引用依赖先拿到 pickle.loads；pickle 导入本身被 E0066 拦，路径不可达（THREAT-MODEL §5 已更新）",
		},
		{
			name: "getattr 动态反射",
			src:  "x = getattr\n",
			want: "禁止访问内建 getattr",
			note: "getattr 在 escapeBuiltins 名单，全局拦截（比 THREAT-MODEL §5 早期记录更强）",
		},
		{
			name: "globals 动态读锁名",
			src:  "lock X = 1\nglobals()['X']\n",
			want: "lock 变量 X 不可通过 globals() 反射读取",
			note: "lock 反射拦截 + escape 名单双层",
		},
		{
			name: "f-string 内联危险内建",
			src:  "x = 1\nf\"{eval('1')}\"\n",
			want: "禁止调用内建 eval",
			note: "THREAT-MODEL §5 f-string 二次解析",
		},
		{
			name: "危险内建读取位置拦截",
			src:  "x = vars\n",
			want: "禁止访问内建 vars",
			note: "escape.go 读取位置也拦（顶层逃逸靠编译期）",
		},
		{
			name: "危险子模块属性",
			src:  "import random\nrandom.os\n",
			want: "禁止访问模块属性",
			note: "modBinds 白名单模块上的危险子模块",
		},
		{
			name: "from import 危险导入项",
			src:  "from random import os\n",
			want: "禁止导入危险模块 os",
			note: "from random import os 的导入项是危险模块（E0066 同源）",
		},
		{
			name: "from import 私有导入项",
			src:  "from random import _os\n",
			want: "禁止导入危险模块 _os",
			note: "from random import _os：_os 是 random 内部绑定的 os 模块（下划线私有规则）",
		},
		{
			name: "模块私有属性链",
			src:  "import random\nrandom._os.system\n",
			want: "禁止访问模块属性 _os",
			note: "random._os 属性链：白名单模块上的下划线私有属性拦截",
		},
		{
			name: "attrgetter 危险模块",
			src:  "import operator\noperator.attrgetter('os')\n",
			want: "禁止访问模块属性 attrgetter",
			note: "attrgetter/itemgetter 编译期拦截",
		},
		{
			name: "__builtins__ 访问",
			src:  "x = __builtins__\n",
			want: "禁止访问 __builtins__",
			note: "E0065",
		},
		{
			name: "异常帧逃逸",
			src:  "try:\n    pass\nexcept Exception as e:\n    x = e.__traceback__\n",
			want: "禁止反射访问",
			note: "帧逃逸黑名单",
		},
		{
			name: "sys 模块导入拦截",
			src:  "import sys\n",
			want: "禁止导入危险模块 sys",
			note: "sys 在 BLOCKED 名单（import 即 E0066）",
		},
		{
			name: "os.system 汇点",
			src:  "safe cmd\nimport os\nos.system(cmd)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 T3",
		},
		{
			name: "subprocess 汇点",
			src:  "safe cmd\nimport subprocess\nsubprocess.run(cmd, shell=True)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 T3",
		},
		{
			name: "SQL execute 汇点",
			src:  "safe q\nconn.execute(q)\n",
			want: "危险汇点",
			note: "THREAT-MODEL §5 T6",
		},
		{
			name: "mask 输出拦截",
			src:  "mask p\np = 1\nprint(p)\n",
			want: "敏感数据",
			note: "mask 声明即敏感源，流入 print 输出上下文",
		},
		{
			name: "only 块内 __builtins__ 访问",
			src:  "only (json):\n    x = __builtins__\n",
			want: "不在白名单",
			note: "only 白名单代理的 __import__ 绕过入口，编译期直接拦",
		},
		{
			name: "only 块内 escape 危险模块导入",
			src:  "only (json):\n    import gc\n",
			want: "不在白名单",
			note: "gc 不在 onlyDeny 原名单但在 escapeModules（BLOCKED），编译期联合拦截",
		},
		{
			name: "only 块内参数默认值危险名",
			src:  "only (json):\n    def f(x=os):\n        return x\n",
			want: "不在白名单",
			note: "refCollector 必须遍历参数默认值",
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
					t.Fatalf("期望放行（%s），实际报错: %v", tc.note, errs)
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
