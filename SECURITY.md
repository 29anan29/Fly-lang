# 安全策略（Security Policy）

PyFly 的目标是编译期阻止不安全代码（注入、篡改、泄露、资源滥用），因此安全问题优先处理。

## 支持的版本

| 版本 | 安全修复支持 |
| :--- | :--- |
| 最新 release | 是 |
| 旧版本 | 否，请升级到最新版（`fly update`） |

## 上报漏洞

**请勿公开披露安全问题**（不要开公开 Issue）。请通过以下方式上报：

- 邮箱：anwang13@outlook.com

请在邮件中说明：

1. 受影响版本（`fly version` 输出）
2. 操作系统与 Go/Python 版本
3. 复现步骤与最小代码样例
4. 影响评估（能绕过哪些安全关键字、泄露什么数据）

## 处理流程

1. 收到上报后 72 小时内确认并回复
2. 评估影响，修复后发布新版本（补丁版本）
3. 修复发布后默认披露（提前披露例外情况可协商）

---

# 安全模型（8 个安全关键字的语义保障）

## 编译期检查

| 关键字 | 保障 | 机制 |
|--------|------|------|
| `safe` | 外部输入不流入危险汇点 | 污点数据流：源点（input/request/os.environ）经赋值/容器/属性/参数传播，到达 pickle.loads/load/Unpickler、eval/exec/compile、os.system/subprocess、SQL execute 等汇点即编译报错；`int()/float()/bool()` 清洗后放行；import 别名（`from pickle import loads as l`、`import pickle as p`）溯源到源模块，不可绕过 |
| `only` | 模块白名单 | 块内仅可访问白名单模块与安全内置；访问白名单外名称编译报错 |
| `lock` | 常量冻结 | 定义后不可再赋值/删除，反射（globals/locals/setattr）不可改 |
| `mask` | 敏感数据防泄漏 | mask 标记数据不可流入输出上下文（print/logging/f-string 参数） |
| `cage` | 资源约束 | 函数级 max_time/max_memory，超限抛 ResourceExhaustedError |
| `guard` | 输入校验 | 函数入口注入运行时断言，失败抛 GuardError（带行列号） |
| `seal` | 类固化 | seal 类的实例属性不可增删改（编译期 + 运行时令牌） |
| `trace` | 强制审计 | 函数级 logging 注入，调用链可审计 |

## 运行时兜底

通过 `fly check` 的代码，所有**动态错误**（除零/下标越界/KeyError/属性缺失/
类型转换失败/不可迭代/运算失败）不再裸抛 Python 异常，统一转为
`FlyRuntimeError: src:行:列: 描述`——错误消息携带源码位置，AI 可定位、可修复。

## 威胁模型与边界

完整威胁模型（含污点规则 R1-R5 形式化、对抗实测表、与 Bandit/Semgrep/Ruff
定位差异）见 [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md)。

- **防御目标**：外部输入引发的注入/RCE/泄漏（pickle、eval、shell、SQL）；
  敏感数据外泄（mask）；资源滥用（cage）。
- **诚实边界**（完整清单见 THREAT-MODEL §6，B1-B8）：
  - B1 跨文件污点流不跟踪（单文件编译模型）；B2 `from x import *` 不检查
  - B3 第三方模块**内部**代码是正常 Python（供应链信任，超出转译器范围）
  - B4 动态反射（`getattr(obj, 动态名)`）、B7 一等函数间接引用（`obj = pickle.loads; obj(x)`）
    不溯源——用 `only` 白名单 + fly-sandbox 兜底
  - B5 业务逻辑漏洞（如鉴权遗漏）不是安全关键字的责任范围
  - B6 运行时兜底保证"统一报错"而非"不报错"；cage 超限仍会抛异常
  - 转译后的 `.py` 是最终执行产物，代码在用户本机运行，编译期检查无法防御
    所有运行时攻击。涉及安全边界绕过的问题请按上文上报。
- **性能成本**：转译注入的 `_fly_*` 护栏在热循环有约 1-3 倍开销（vs CPython，
  见 [docs/bench/bench.md](docs/bench/bench.md)），安全与性能的取舍明确公开。

## 验证

- 反例测试：`testdata/errors/*.fly`（编译期必须报错的全部场景）
- 快照锁定：错误消息与转译产物 golden 对比（`go test ./...`）
- CVE 对照演示：`bash examples/cve-pickle/run-demo.sh`（5 幕：编译期 + 沙箱双层）
