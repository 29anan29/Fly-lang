package ast

type ErrorInfo struct {
	Code    string
	Title   string
	Help    string
	Note    string
	Example string
}

var codeForFormat = map[string]string{
	"期望 %s，实际为 %s":                          "E0001",
	"期望表达式，实际为 %q":                          "E0002",
	"期望语句结束，实际为 %q":                         "E0003",
	"关键字 %s 暂不支持":                           "E0004",
	"关键字参数名必须是标识符":                          "E0005",
	"非法赋值目标":                                "E0006",
	"装饰器后必须跟随 def 或 class":                  "E0007",
	"字典/集合推导式暂不支持":                          "E0008",
	"字典解包 {} 暂不支持":                          "E0009",
	"lock 需要一个变量名":                          "E0010",
	"seal 后必须跟随 class 定义":                   "E0011",
	"level 必须是字符串（如 \"WARN\"）":              "E0012",
	"args 必须是 True 或 False":                 "E0013",
	"ret 必须是 True 或 False":                  "E0014",
	"trace 参数 %s 未知（支持 level/args/ret）":     "E0015",
	"cage 参数 %s 必须是字符串（如 max_time=\"5s\"）":  "E0016",
	"max_time 格式非法：%q（支持 500ms/5s/2m/1h）":   "E0017",
	"max_memory 格式非法：%q（支持 64KB/100MB/2GB）": "E0018",
	"cage 参数 %s 未知（支持 max_time/max_memory）": "E0019",
	"cage 需至少指定 max_time 或 max_memory 之一":   "E0020",

	"未闭合的括号":       "E0021",
	"行首不支持制表符缩进":   "E0022",
	"缩进级别与上层不一致":   "E0023",
	"字符串未闭合":       "E0024",
	"指数部分缺少数字: %s": "E0025",
	"非法数字字面量 %s":   "E0026",
	"意外的字符 %q":     "E0027",
	"意外的字符 '%s'":   "E0027",
	"多余的右括号 '%s'":  "E0028",
	"多余的右括号 ')'":   "E0028",
	"多余的右括号 ']'":   "E0028",
	"多余的右括号 '}'":   "E0028",

	"名字 %s 重复定义（第一次在第 %d 行）": "E0029",
	"函数参数 %s 重复定义":           "E0030",

	"未净化的外部输入 %s 流入 %s（危险汇点）": "E0031",
	"敏感数据 %s 不可流入 %s（输出上下文）":  "E0032",

	"lock 变量 %s 未定义":             "E0033",
	"lock 变量 %s 不可删除":            "E0034",
	"lock 变量 %s 不可再赋值":           "E0035",
	"lock 变量 %s 不可通过反射修改":        "E0036",
	"lock 变量 %s 不可通过 setattr 修改": "E0037",
	"lock 变量 %s 不可通过 %s() 反射读取":  "E0038",

	"guard 变量 %s 未定义":             "E0039",
	"guard 类型必须是简单类型名（如 int、str）": "E0040",
	"guard 类型 %s 与参数注解 %s 不一致":    "E0041",
	"guard 至少需要一个类型或条件":           "E0042",
	"guard 条件中引用了未定义的名字 %s":       "E0043",

	"only 白名单不能为空（需至少一个模块，如 only (json):）": "E0044",
	"only 块禁止访问 %s（不在白名单 %v）":              "E0045",

	"seal 类实例 %s 的属性 %s 不可修改": "E0046",

	"trace 级别 %s 非法（支持 DEBUG/INFO/WARNING/ERROR/CRITICAL）": "E0047",
	"trace 块内函数名 %s 不能以 _fly_ 开头（保留前缀）":                    "E0048",
	"trace 块内参数名 %s 不能以 _fly_ 开头（保留前缀）":                    "E0049",

	"未定义的名字 %s（safe 需要先赋值）":        "E0050",
	"未定义的名字 %s（mask 需要先赋值）":        "E0051",
	"未定义的名字 %s":                    "E0052",
	"函数 %s 需要至少 %d 个参数（实际 %d 个）":   "E0053",
	"函数 %s 最多接受 %d 个位置参数（实际 %d 个）": "E0054",
	"函数 %s 没有名为 %s 的参数":            "E0055",
	"函数 %s 参数 %s 重复传值":             "E0056",
	"常量表达式除数为零":                    "E0057",
	"运算符 %s 不支持 %s 与 %s":           "E0058",
	"运算符 ~ 不支持 %s":                 "E0059",
	"运算符 %s 不支持 %s":                "E0060",
	"in 右侧不支持 %s":                  "E0061",
	"%s 不可下标访问":                    "E0062",
}

var errorInfo = map[string]ErrorInfo{
	"E0001": {"E0001", "语法不匹配", "检查该位置的期望符号，补全或修正语法结构", "docs/报错清单.md#E0001",
		"error[E0001]: 语法不匹配\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = (1 +\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: 期望 :，实际为 换行。补全表达式或语法符号"},
	"E0002": {"E0002", "期望表达式", "该位置需要一个表达式", "docs/报错清单.md#E0002",
		"error[E0002]: 期望表达式\n" +
			"  --> example.fly:2:1\n" +
			"   |\n" +
			"   2 | x =\n" +
			"   | ^\n" +
			"   |\n" +
			"   = help: 期望表达式，实际为 换行。在赋值号右侧补上表达式"},
	"E0003": {"E0003", "语句未正确结束", "检查语句结束符（换行/分号）与缩进", "docs/报错清单.md#E0003",
		"error[E0003]: 语句未正确结束\n" +
			"  --> example.fly:1:7\n" +
			"   |\n" +
			"   1 | x = 1 2\n" +
			"   |       ^\n" +
			"   |\n" +
			"   = help: 期望语句结束，实际为 数字字面量 2。移除多余内容或用分号/换行分隔"},
	"E0004": {"E0004", "关键字未实现", "替换为受支持的语法（如 with 改为 try/finally）", "docs/报错清单.md#E0004",
		"error[E0004]: 关键字未实现\n" +
			"  --> example.fly:1:1\n" +
			"   |\n" +
			"   1 | with open(\"f\") as f:\n" +
			"   | ^^^^\n" +
			"   |\n" +
			"   = help: 关键字 with 暂不支持。改用 try/finally 显式管理资源"},
	"E0005": {"E0005", "非法关键字参数名", "关键字参数名必须是标识符", "docs/报错清单.md#E0005",
		"error[E0005]: 非法关键字参数名\n" +
			"  --> example.fly:1:8\n" +
			"   |\n" +
			"   1 | f(**1)\n" +
			"   |        ^\n" +
			"   |\n" +
			"   = help: 关键字参数名必须是标识符。改为 f(x=1) 形式"},
	"E0006": {"E0006", "非法赋值目标", "赋值目标必须是变量、属性或下标表达式", "docs/报错清单.md#E0006",
		"error[E0006]: 非法赋值目标\n" +
			"  --> example.fly:1:1\n" +
			"   |\n" +
			"   1 | 1 = x\n" +
			"   | ^\n" +
			"   |\n" +
			"   = help: 非法赋值目标。等号左侧必须是可赋值的名字"},
	"E0007": {"E0007", "装饰器用法错误", "装饰器后必须跟随 def 或 class 定义", "docs/报错清单.md#E0007",
		"error[E0007]: 装饰器用法错误\n" +
			"  --> example.fly:2:1\n" +
			"   |\n" +
			"   2 | @deco\n" +
			"   | ^^^^^\n" +
			"   |\n" +
			"   = help: 装饰器后必须跟随 def 或 class。在 @deco 后定义函数或类"},
	"E0008": {"E0008", "推导式未支持", "改用循环逐项构建容器", "docs/报错清单.md#E0008",
		"error[E0008]: 推导式未支持\n" +
			"  --> example.fly:1:1\n" +
			"   |\n" +
			"   1 | {k: v for k, v in d}\n" +
			"   | ^^^^^^^^^^^^^^^^^^^^^^\n" +
			"   |\n" +
			"   = help: 字典/集合推导式暂不支持。改用 for 循环逐项构建容器"},
	"E0009": {"E0009", "字典解包未支持", "改用显式的 dict 合并表达式", "docs/报错清单.md#E0009",
		"error[E0009]: 字典解包未支持\n" +
			"  --> example.fly:1:4\n" +
			"   |\n" +
			"   1 | {**d}\n" +
			"   |    ^\n" +
			"   |\n" +
			"   = help: 字典解包 {} 暂不支持。改用显式的 dict 合并表达式"},
	"E0010": {"E0010", "lock 缺少变量名", "写法：lock 变量名 或 lock 变量名 = 值", "docs/报错清单.md#E0010",
		"error[E0010]: lock 缺少变量名\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | lock = 42\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: lock 需要一个变量名。写法：lock 变量名 或 lock 变量名 = 值"},
	"E0011": {"E0011", "seal 缺少 class", "seal 只能修饰 class 定义", "docs/报错清单.md#E0011",
		"error[E0011]: seal 缺少 class\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | seal def f():\n" +
			"   |     ^^^\n" +
			"   |\n" +
			"   = help: seal 后必须跟随 class 定义。seal 只能修饰类"},
	"E0012": {"E0012", "trace 参数类型错误", "level 必须写为字符串，如 trace(level=\"WARN\"):", "docs/报错清单.md#E0012",
		"error[E0012]: trace 参数类型错误\n" +
			"  --> example.fly:1:13\n" +
			"   |\n" +
			"   1 | trace(level=WARN):\n" +
			"   |             ^^^^\n" +
			"   |\n" +
			"   = help: level 必须是字符串（如 \"WARN\"）。改为 trace(level=\"WARN\"):"},
	"E0013": {"E0013", "trace 参数类型错误", "args/ret 必须写为 True 或 False", "docs/报错清单.md#E0013",
		"error[E0013]: trace 参数类型错误\n" +
			"  --> example.fly:1:13\n" +
			"   |\n" +
			"   1 | trace(args=1):\n" +
			"   |             ^\n" +
			"   |\n" +
			"   = help: args 必须是 True 或 False。改为 trace(args=True):"},
	"E0014": {"E0014", "trace 参数类型错误", "args/ret 必须写为 True 或 False", "docs/报错清单.md#E0014",
		"error[E0014]: trace 参数类型错误\n" +
			"  --> example.fly:1:11\n" +
			"   |\n" +
			"   1 | trace(ret=1):\n" +
			"   |           ^\n" +
			"   |\n" +
			"   = help: ret 必须是 True 或 False。改为 trace(ret=True):"},
	"E0015": {"E0015", "未知 trace 参数", "仅支持 level/args/ret 三个参数", "docs/报错清单.md#E0015",
		"error[E0015]: 未知 trace 参数\n" +
			"  --> example.fly:1:8\n" +
			"   |\n" +
			"   1 | trace(debug=True):\n" +
			"   |        ^^^^^\n" +
			"   |\n" +
			"   = help: trace 参数 debug 未知（支持 level/args/ret）。删除或改名为支持的参数"},
	"E0016": {"E0016", "cage 参数类型错误", "cage 参数必须写为字符串，如 max_time=\"5s\"", "docs/报错清单.md#E0016",
		"error[E0016]: cage 参数类型错误\n" +
			"  --> example.fly:1:11\n" +
			"   |\n" +
			"   1 | cage(max_time=5):\n" +
			"   |           ^\n" +
			"   |\n" +
			"   = help: cage 参数 max_time 必须是字符串（如 max_time=\"5s\"）"},
	"E0017": {"E0017", "max_time 格式非法", "格式：500ms/5s/2m/1h", "docs/报错清单.md#E0017",
		"error[E0017]: max_time 格式非法\n" +
			"  --> example.fly:1:12\n" +
			"   |\n" +
			"   1 | cage(max_time=\"5x\"):\n" +
			"   |            ^^^^^\n" +
			"   |\n" +
			"   = help: max_time 格式非法：\"5x\"（支持 500ms/5s/2m/1h）。改为 max_time=\"5s\""},
	"E0018": {"E0018", "max_memory 格式非法", "格式：64KB/100MB/2GB", "docs/报错清单.md#E0018",
		"error[E0018]: max_memory 格式非法\n" +
			"  --> example.fly:1:14\n" +
			"   |\n" +
			"   1 | cage(max_memory=\"5\"):\n" +
			"   |              ^^^\n" +
			"   |\n" +
			"   = help: max_memory 格式非法：\"5\"（支持 64KB/100MB/2GB）。改用带单位的写法"},
	"E0019": {"E0019", "未知 cage 参数", "仅支持 max_time/max_memory", "docs/报错清单.md#E0019",
		"error[E0019]: 未知 cage 参数\n" +
			"  --> example.fly:1:8\n" +
			"   |\n" +
			"   1 | cage(max_cpu=\"1\"):\n" +
			"   |        ^^^^^^^\n" +
			"   |\n" +
			"   = help: cage 参数 max_cpu 未知（支持 max_time/max_memory）"},
	"E0020": {"E0020", "cage 缺少限制", "至少指定 max_time 或 max_memory 之一", "docs/报错清单.md#E0020",
		"error[E0020]: cage 缺少限制\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | cage():\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: cage 需至少指定 max_time 或 max_memory 之一"},

	"E0021": {"E0021", "括号未闭合", "补全匹配的右括号 ) ] }", "docs/报错清单.md#E0021",
		"error[E0021]: 括号未闭合\n" +
			"  --> example.fly:1:1\n" +
			"   |\n" +
			"   1 | x = (1 + 2\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: 未闭合的括号。补全右括号 )"},
	"E0022": {"E0022", "禁止制表符缩进", "将行首制表符替换为空格缩进", "docs/报错清单.md#E0022",
		"error[E0022]: 禁止制表符缩进\n" +
			"  --> example.fly:2:1\n" +
			"   |\n" +
			"   2 | \\tx = 1\n" +
			"   | ^\n" +
			"   |\n" +
			"   = help: 行首不支持制表符缩进。改用空格缩进"},
	"E0023": {"E0023", "缩进级别不一致", "检查并统一该块的空格缩进", "docs/报错清单.md#E0023",
		"error[E0023]: 缩进级别不一致\n" +
			"  --> example.fly:3:1\n" +
			"   |\n" +
			"   3 |   pass\n" +
			"   | ^\n" +
			"   |\n" +
			"   = help: 缩进级别与上层不一致。检查并统一该块的空格缩进"},
	"E0024": {"E0024", "字符串未闭合", "为字符串补上结束引号", "docs/报错清单.md#E0024",
		"error[E0024]: 字符串未闭合\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = \"abc\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: 字符串未闭合。为字符串补上结束引号"},
	"E0025": {"E0025", "数字字面量非法", "补全指数部分，如 1e5", "docs/报错清单.md#E0025",
		"error[E0025]: 数字字面量非法\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = 1e\n" +
			"   |     ^^\n" +
			"   |\n" +
			"   = help: 指数部分缺少数字: 1e。补全指数部分，如 1e5"},
	"E0026": {"E0026", "数字字面量非法", "修正数字写法，如删除无效的前导零", "docs/报错清单.md#E0026",
		"error[E0026]: 数字字面量非法\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = 0123\n" +
			"   |     ^^^^\n" +
			"   |\n" +
			"   = help: 非法数字字面量 0123。修正数字写法，如删除无效的前导零"},
	"E0027": {"E0027", "意外字符", "删除或替换该字符", "docs/报错清单.md#E0027",
		"error[E0027]: 意外字符\n" +
			"  --> example.fly:1:3\n" +
			"   |\n" +
			"   1 | x = @\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: 意外的字符 '@'。删除或替换该字符"},
	"E0028": {"E0028", "多余的右括号", "删除多余的右括号", "docs/报错清单.md#E0028",
		"error[E0028]: 多余的右括号\n" +
			"  --> example.fly:1:8\n" +
			"   |\n" +
			"   1 | x = (1))\n" +
			"   |        ^\n" +
			"   |\n" +
			"   = help: 多余的右括号 ')'。删除多余的右括号"},

	"E0029": {"E0029", "名字重复定义", "删除或重命名其中一个定义", "docs/报错清单.md#E0029",
		"error[E0029]: 名字重复定义\n" +
			"  --> example.fly:3:1\n" +
			"   |\n" +
			"   3 | def add():\n" +
			"   | ^^^\n" +
			"   |\n" +
			"   = help: 名字 add 重复定义（第一次在第 1 行）。删除或重命名其中一个定义"},
	"E0030": {"E0030", "参数重复定义", "函数参数名必须唯一", "docs/报错清单.md#E0030",
		"error[E0030]: 参数重复定义\n" +
			"  --> example.fly:1:7\n" +
			"   |\n" +
			"   1 | def f(a, a):\n" +
			"   |       ^\n" +
			"   |\n" +
			"   = help: 函数参数 a 重复定义。函数参数名必须唯一"},

	"E0031": {"E0031", "污点数据流入危险汇点", "对该值先做净化（白名单/类型校验）再流入汇点，或改用 only 白名单约束", "SECURITY.md 污点分析一节",
		"error[E0031]: 污点数据流入危险汇点\n" +
			"  --> example.fly:4:18\n" +
			"   |\n" +
			"   4 |     obj = pickle.loads(data)\n" +
			"   |                  ^^^^^^\n" +
			"   |\n" +
			"   = help: 未净化的外部输入 data 流入 pickle.loads（危险汇点）。对该值先做净化（白名单/类型校验）再流入汇点，或改用 only 白名单约束"},
	"E0032": {"E0032", "敏感数据流入输出上下文", "对该值脱敏（如遮蔽/哈希）后再输出，或移除该输出调用", "SECURITY.md 污点分析一节",
		"error[E0032]: 敏感数据流入输出上下文\n" +
			"  --> example.fly:5:5\n" +
			"   |\n" +
			"   5 |     print(token)\n" +
			"   |     ^^^^^\n" +
			"   |\n" +
			"   = help: 敏感数据 token 不可流入 print（输出上下文）。对该值脱敏（如遮蔽/哈希）后再输出，或移除该输出调用"},

	"E0033": {"E0033", "lock 变量未定义", "先声明 lock 变量，或检查拼写", "docs/报错清单.md#E0033",
		"error[E0033]: lock 变量未定义\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 | lock SECRET\n" +
			"   |     ^^^^^^\n" +
			"   |\n" +
			"   = help: lock 变量 SECRET 未定义。先声明 lock 变量，或检查拼写"},
	"E0034": {"E0034", "lock 变量不可删除", "lock 变量生命周期受保护，删除将破坏不变量", "docs/报错清单.md#E0034",
		"error[E0034]: lock 变量不可删除\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 | del SECRET\n" +
			"   |     ^^^^^^\n" +
			"   |\n" +
			"   = help: lock 变量 SECRET 不可删除。lock 变量生命周期受保护"},
	"E0035": {"E0035", "lock 变量不可再赋值", "lock 变量只读；如需可变请改用普通变量", "docs/报错清单.md#E0035",
		"error[E0035]: lock 变量不可再赋值\n" +
			"  --> example.fly:3:5\n" +
			"   |\n" +
			"   3 | SECRET = \"other\"\n" +
			"   |     ^^^^^^\n" +
			"   |\n" +
			"   = help: lock 变量 SECRET 不可再赋值。lock 变量只读；如需可变请改用普通变量"},
	"E0036": {"E0036", "禁止反射修改 lock", "lock 变量不可通过反射（globals()[...]）修改", "docs/报错清单.md#E0036",
		"error[E0036]: 禁止反射修改 lock\n" +
			"  --> example.fly:4:13\n" +
			"   |\n" +
			"   4 |     globals()[\"SECRET\"] = 1\n" +
			"   |             ^^^^^^^^\n" +
			"   |\n" +
			"   = help: lock 变量 SECRET 不可通过反射修改。lock 变量不可通过反射（globals()[...]）修改"},
	"E0037": {"E0037", "禁止 setattr 修改 lock", "lock 变量不可通过 setattr 修改", "docs/报错清单.md#E0037",
		"error[E0037]: 禁止 setattr 修改 lock\n" +
			"  --> example.fly:5:13\n" +
			"   |\n" +
			"   5 |     setattr(mod, \"SECRET\", 1)\n" +
			"   |             ^^^^\n" +
			"   |\n" +
			"   = help: lock 变量 SECRET 不可通过 setattr 修改"},
	"E0038": {"E0038", "禁止反射读取 lock", "lock 变量不可通过反射（globals()/vars()/locals()）读取", "docs/报错清单.md#E0038",
		"error[E0038]: 禁止反射读取 lock\n" +
			"  --> example.fly:1:1\n" +
			"   |\n" +
			"   1 | lock SECRET = \"abc\"\n" +
			"   | ^^^^\n" +
			"   |\n" +
			"   = help: lock 变量 SECRET 不可通过 globals() 反射读取"},

	"E0039": {"E0039", "guard 变量未定义", "先定义该变量（或参数）再使用 guard", "docs/报错清单.md#E0039",
		"error[E0039]: guard 变量未定义\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 |     guard age: int, 0 < age < 150\n" +
			"   |     ^^^^^\n" +
			"   |\n" +
			"   = help: guard 变量 age 未定义。先定义该变量（或参数）再使用 guard"},
	"E0040": {"E0040", "guard 类型必须是简单类型名", "如 int、str、float、list、dict", "docs/报错清单.md#E0040",
		"error[E0040]: guard 类型必须是简单类型名\n" +
			"  --> example.fly:2:12\n" +
			"   |\n" +
			"   2 | guard x: list[int]\n" +
			"   |            ^^^\n" +
			"   |\n" +
			"   = help: guard 类型必须是简单类型名（如 int、str）。list[int] 请改写字面量条件"},
	"E0041": {"E0041", "guard 类型与注解不一致", "统一 guard 类型与参数注解的类型", "docs/报错清单.md#E0041",
		"error[E0041]: guard 类型与注解不一致\n" +
			"  --> example.fly:2:8\n" +
			"   |\n" +
			"   2 | guard x: str  # 参数注解为 int\n" +
			"   |        ^\n" +
			"   |\n" +
			"   = help: guard 类型 str 与参数注解 int 不一致。统一两者类型"},
	"E0042": {"E0042", "guard 缺少类型或条件", "至少指定一个类型或条件约束", "docs/报错清单.md#E0042",
		"error[E0042]: guard 缺少类型或条件\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 | guard x:\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: guard 至少需要一个类型或条件。补充类型或条件约束"},
	"E0043": {"E0043", "guard 条件引用未定义名字", "先定义条件中引用的名字", "docs/报错清单.md#E0043",
		"error[E0043]: guard 条件引用未定义名字\n" +
			"  --> example.fly:2:14\n" +
			"   |\n" +
			"   2 | guard x: 0 < x < limit\n" +
			"   |              ^^^^^\n" +
			"   |\n" +
			"   = help: guard 条件中引用了未定义的名字 limit。先定义该名字"},

	"E0044": {"E0044", "only 白名单为空", "至少列出一个模块，如 only (json):", "docs/报错清单.md#E0044",
		"error[E0044]: only 白名单为空\n" +
			"  --> example.fly:1:6\n" +
			"   |\n" +
			"   1 | only ():\n" +
			"   |      ^\n" +
			"   |\n" +
			"   = help: only 白名单不能为空（需至少一个模块，如 only (json):）"},
	"E0045": {"E0045", "only 块外访问被禁止", "将该名字加入 only 白名单，或移除该访问", "docs/报错清单.md#E0045",
		"error[E0045]: only 块外访问被禁止\n" +
			"  --> example.fly:2:9\n" +
			"   |\n" +
			"   2 |     os.system(\"rm -rf /\")\n" +
			"   |         ^^^^^^\n" +
			"   |\n" +
			"   = help: only 块禁止访问 os（不在白名单 [json math]）。将该名字加入 only 白名单，或移除该访问"},

	"E0046": {"E0046", "seal 属性不可修改", "seal 类实例属性在实例化后不可再赋值", "docs/报错清单.md#E0046",
		"error[E0046]: seal 属性不可修改\n" +
			"  --> example.fly:7:5\n" +
			"   |\n" +
			"   7 |     u.name = \"other\"\n" +
			"   |     ^^^^^^\n" +
			"   |\n" +
			"   = help: seal 类实例 u 的属性 name 不可修改。seal 类实例属性在实例化后不可再赋值"},

	"E0047": {"E0047", "trace 级别非法", "使用 DEBUG/INFO/WARNING/ERROR/CRITICAL 之一", "docs/报错清单.md#E0047",
		"error[E0047]: trace 级别非法\n" +
			"  --> example.fly:1:13\n" +
			"   |\n" +
			"   1 | trace(level=\"X\"):\n" +
			"   |             ^\n" +
			"   |\n" +
			"   = help: trace 级别 X 非法（支持 DEBUG/INFO/WARNING/ERROR/CRITICAL）。改用其中之一"},
	"E0048": {"E0048", "保留前缀冲突", "函数名不能以 _fly_ 开头（运行时保留前缀）", "docs/报错清单.md#E0048",
		"error[E0048]: 保留前缀冲突\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 |     def _fly_clean():\n" +
			"   |     ^^^^^^^^^^^^^^^^\n" +
			"   |\n" +
			"   = help: trace 块内函数名 _fly_clean 不能以 _fly_ 开头（保留前缀）。改名去前缀"},
	"E0049": {"E0049", "保留前缀冲突", "参数名不能以 _fly_ 开头（运行时保留前缀）", "docs/报错清单.md#E0049",
		"error[E0049]: 保留前缀冲突\n" +
			"  --> example.fly:2:12\n" +
			"   |\n" +
			"   2 |     def f(_fly_x):\n" +
			"   |            ^^^^^\n" +
			"   |\n" +
			"   = help: trace 块内参数名 _fly_x 不能以 _fly_ 开头（保留前缀）。改名去前缀"},

	"E0050": {"E0050", "safe 变量未定义", "在 safe 声明前先赋值", "docs/报错清单.md#E0050",
		"error[E0050]: safe 变量未定义\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 | safe name\n" +
			"   |     ^^^^\n" +
			"   |\n" +
			"   = help: 未定义的名字 name（safe 需要先赋值）。在 safe 声明前先赋值"},
	"E0051": {"E0051", "mask 变量未定义", "在 mask 声明前先赋值", "docs/报错清单.md#E0051",
		"error[E0051]: mask 变量未定义\n" +
			"  --> example.fly:2:5\n" +
			"   |\n" +
			"   2 | mask secret\n" +
			"   |     ^^^^^^\n" +
			"   |\n" +
			"   = help: 未定义的名字 secret（mask 需要先赋值）。在 mask 声明前先赋值"},
	"E0052": {"E0052", "名字未定义", "先定义该名字，或检查拼写", "docs/报错清单.md#E0052",
		"error[E0052]: 名字未定义\n" +
			"  --> example.fly:3:5\n" +
			"   |\n" +
			"   3 |     return result\n" +
			"   |            ^^^^^^\n" +
			"   |\n" +
			"   = help: 未定义的名字 result。先定义该名字，或检查拼写"},
	"E0053": {"E0053", "参数个数不足", "补齐函数的必需参数", "docs/报错清单.md#E0053",
		"error[E0053]: 参数个数不足\n" +
			"  --> example.fly:4:5\n" +
			"   |\n" +
			"   4 |     add(1)\n" +
			"   |     ^^^^^^\n" +
			"   |\n" +
			"   = help: 函数 add 需要至少 2 个参数（实际 1 个）。补齐必需参数"},
	"E0054": {"E0054", "位置参数过多", "改用关键字参数传参", "docs/报错清单.md#E0054",
		"error[E0054]: 位置参数过多\n" +
			"  --> example.fly:4:5\n" +
			"   |\n" +
			"   4 |     add(1, 2, 3)\n" +
			"   |     ^^^^^^^^^^^^\n" +
			"   |\n" +
			"   = help: 函数 add 最多接受 2 个位置参数（实际 3 个）。改用关键字参数传参"},
	"E0055": {"E0055", "未知参数名", "检查参数名拼写", "docs/报错清单.md#E0055",
		"error[E0055]: 未知参数名\n" +
			"  --> example.fly:4:9\n" +
			"   |\n" +
			"   4 |     add(1, x=2)\n" +
			"   |         ^\n" +
			"   |\n" +
			"   = help: 函数 add 没有名为 x 的参数。检查参数名拼写"},
	"E0056": {"E0056", "参数重复传值", "同一参数只能传值一次", "docs/报错清单.md#E0056",
		"error[E0056]: 参数重复传值\n" +
			"  --> example.fly:4:9\n" +
			"   |\n" +
			"   4 |     add(1, a=2)\n" +
			"   |         ^\n" +
			"   |\n" +
			"   = help: 函数 add 参数 a 重复传值。同一参数只能传值一次"},
	"E0057": {"E0057", "常量表达式除零", "修正除数，避免除零", "docs/报错清单.md#E0057",
		"error[E0057]: 常量表达式除零\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = 10 // 0\n" +
			"   |     ^^\n" +
			"   |\n" +
			"   = help: 常量表达式除数为零。修正除数，避免除零"},
	"E0058": {"E0058", "运算符类型不支持", "确保操作数类型匹配运算符", "docs/报错清单.md#E0058",
		"error[E0058]: 运算符类型不支持\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = \"a\" + 1\n" +
			"   |     ^^^\n" +
			"   |\n" +
			"   = help: 运算符 + 不支持 str 与 int。确保操作数类型匹配运算符"},
	"E0059": {"E0059", "运算符类型不支持", "按位取反仅支持整数", "docs/报错清单.md#E0059",
		"error[E0059]: 运算符类型不支持\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = ~\"a\"\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: 运算符 ~ 不支持 str。按位取反仅支持整数"},
	"E0060": {"E0060", "运算符类型不支持", "确保操作数类型匹配运算符", "docs/报错清单.md#E0060",
		"error[E0060]: 运算符类型不支持\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = -\"a\"\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: 运算符 - 不支持 str。确保操作数类型匹配运算符"},
	"E0061": {"E0061", "in 右侧类型不支持", "in 右侧必须是可迭代容器", "docs/报错清单.md#E0061",
		"error[E0061]: in 右侧类型不支持\n" +
			"  --> example.fly:1:7\n" +
			"   |\n" +
			"   1 | x = 1 in 42\n" +
			"   |       ^^\n" +
			"   |\n" +
			"   = help: in 右侧不支持 int。in 右侧必须是可迭代容器"},
	"E0062": {"E0062", "类型不可下标访问", "仅列表/字典/字符串等容器支持下标", "docs/报错清单.md#E0062",
		"error[E0062]: 类型不可下标访问\n" +
			"  --> example.fly:1:5\n" +
			"   |\n" +
			"   1 | x = 1[0]\n" +
			"   |     ^\n" +
			"   |\n" +
			"   = help: int 不可下标访问。仅列表/字典/字符串等容器支持下标"},
}

func CodeForFormat(format string) string {
	return codeForFormat[format]
}

func InfoForCode(code string) (ErrorInfo, bool) {
	info, ok := errorInfo[code]
	return info, ok
}
