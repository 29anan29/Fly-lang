# Fly-Lang 威胁模型与安全论证

> 面向 AI Agent 与外部输入的 Python 代码，编译期安全转译的威胁模型。
> 原则：**诚实披露边界**——能保证什么、不能保证什么、为什么，全部写清。

## 1. 资产与信任边界

| 资产 | 保护目标 |
|------|----------|
| Agent 宿主进程 | 不被注入代码 / 不被任意命令执行 / 不被耗死 |
| 宿主数据 | 不被读取外泄（密钥、用户数据、内部状态） |
| 外部系统 | 不被代理 SSRF/命令执行攻击 |
| 产物可用性 | 通过编译的产物不崩溃（动态错误统一兜底） |

信任边界：**源码与产物之间是编译期信任**（源码中的风险代码进不了产物）；
**第三方模块内部是供应链信任**（超出转译器能力，按 pip 生态惯例处理）。

## 2. 8 个关键字的编译期检查逻辑

| 关键字 | 编译期检查（checker） | 运行时注入（gen） | 拦截时机 |
|--------|----------------------|-------------------|----------|
| `safe` | 污点数据流：`input()/request.*/os.environ/safe 参数` → 赋值/容器/属性/参数传播 → 危险汇点（pickle.loads、eval/exec/compile、os.system、subprocess.*、SQL execute）；`int()/float()/bool()` 清洗即放行；import 别名溯源（`pickle as p`、`from pickle import loads as l`） | 无（纯编译期） | 编译失败 |
| `only` | 白名单块：访问黑名单名称（33 项，含 pickle/marshal/os/subprocess/eval）编译报错 | `__builtins__ = _FlyOnly(mods)` 白名单代理（运行时兜底） | 编译失败 + 运行时兜底 |
| `lock` | 常量再赋值/删除、`globals()['X']`/`vars()['X']`/`setattr` 反射读写锁定名 | 无 | 编译失败 |
| `mask` | 敏感数据流入输出上下文（print/logging/f-string 参数） | 无 | 编译失败 |
| `cage` | 无（纯 gen+runtime） | `@_fly_cage(max_time, max_memory)`：SIGALRM + RLIMIT_AS | 运行时 |
| `guard` | 函数签名/参数注解一致性检查 | 函数入口断言，失败抛 GuardError（带行列号） | 编译失败 + 运行时 |
| `seal` | seal 类实例直接属性赋值/删除拦截 | `__setattr__`/`__delattr__` 重写（初始化令牌放行） | 编译失败 + 运行时 |
| `trace` | 保留前缀 `_fly_` 冲突、非法级别 | 函数改名 + logging 进出日志 | 编译失败 + 运行时 |

**设计原则**：`safe/mask/lock` 是纯编译期（零运行时残留——产物里没有兜底代码，
攻击者改产物无法恢复被删掉的检查）；`only/seal/trace/cage/guard` 是编译期检查 +
运行时注入（兜底存在于产物本身，改产物会破坏功能但兜底随之消失或被改写）。

## 3. 污点数据流模型（形式化概要）

```
源点 S:   input() / request.* / os.environ / safe 声明的参数
传播 P:   赋值 → 容器(list/dict/set) → 属性 → 下标 → f-string → 函数参数/返回值
清洗 C:   int()/float()/bool() 调用后污点清除（显式净化）
汇点 K:   pickle.loads/load/Unpickler, eval/exec/compile,
          os.system/popen/spawn*, subprocess.*, SQL execute, 输出上下文(print/logging)
```

规则（按保守序）：
- **R1 直连**：`t ∈ Taint(S) ∧ t 作为实参传入 K` → 报错
- **R2 传播**：`x = t; x ∈ Taint(S)`；`x ∈ Taint(S) → x[expr] ∈ Taint(S)`；
  `x ∈ Taint(S) → x.attr ∈ Taint(S)`；`f"{t}" ∈ Taint(S)`
- **R3 别名溯源**：`import pickle as p → p 的模块属性溯源到 pickle`；
  `from pickle import loads as l → l 溯源到 pickle.loads`
- **R4 清洗**：`t' = int(t) → Taint(t') = ∅`（显式清洗 = 开发者承诺）
- **R5 非路径敏感**：分支内赋值即污染（`if c: x = t; K(x)` 报错——偏保守）

**形式化定位**：污点系统是**类型流敏感（flow-sensitive）+ 上下文敏感（通过
参数-返回签名建模）**的保守近似，无路径敏感（R5）。误报偏安全侧，漏报只可能
出现在 §6 列出的边界。

## 4. 攻击者视角（威胁场景）

### T1 反序列化 RCE（pickle）
外部输入直接 `pickle.loads/load/Unpickler` → `__reduce__` 任意代码执行。
**防御**：`safe` 污点汇点拦截（含别名/容器传播）。演示见
`examples/cve-pickle/`（5 幕：编译期 + 沙箱双层）。**剩余风险**：B1 跨文件、
B7 间接引用。

### T2 动态执行注入（eval/exec/compile）
**防御**：污点汇点含 eval/exec/compile。

### T3 命令注入（os.system/subprocess/shell）
**防御**：污点汇点含 os.system/popen/spawn*/subprocess；`only` 白名单可封禁。

### T4 敏感数据外泄（mask）
**防御**：mask 污点 + 输出上下文拦截。**剩余风险**：写文件/网络发送不属
静态输出上下文，用 only 白名单封禁（B4）。

### T5 资源耗尽（cage）
**防御**：cage 函数级 max_time/max_memory（SIGALRM + RLIMIT_AS）。

### T6 数据投毒（外部输入未校验）
**防御**：`guard` 运行时断言 + 显式清洗。

### T7 反射篡改（lock/seal 对抗）
**防御**：lock 编译期反射拦截；seal 运行时令牌拦截。

## 5. 对抗措施有效性评估（实测表）

以下全部经 `fly check` 实测（`go test ./...` 反例测试锁定）：

| 对抗方式 | 结果 | 测试 |
|----------|------|------|
| `from pickle import loads as l; l(x)` 别名 | ✅ 拦截 | errors/pickle_rce.fly |
| `import pickle as p; p.loads(x)` | ✅ 拦截 | 同上 |
| `__reduce__` 改换成其他 callable | ✅ 拦截（汇点是 loads，与载荷无关） | 同上 |
| 把 x 放进 list/dict 再取出 | ✅ 拦截（容器传播） | b3 实测 |
| `int(x)` 清洗后再用 | ✅ 放行（显式清洗语义正确） | taint.go R4 |
| `eval("x")` 动态拼接 | ✅ 拦截 | errors/safe_eval.fly |
| `c.x = 污点; eval(c.x)` 属性赋值传播 | ✅ 拦截（属性名级精确跟踪） | redteam_test.go 属性赋值传播 |
| `obj = pickle.loads; obj(x)`（间接引用） | ✅ 不可达（`import pickle` 本身 E0066） | redteam_test.go 间接引用 |
| `getattr(pickle, "loads")(x)` | ✅ 拦截（getattr 在危险内建名单） | redteam_test.go getattr |
| `globals()['x']` 动态读取后投递 | ⚠️ 部分（lock 拦锁定名；普通名不拦） | lock_globals.fly |

> 注：表内 ✅ 行均有自动化回归锁定（`internal/checker/redteam_test.go` 表驱动 + testdata 反例），
> 行为漂移会直接导致测试失败。原 B7 间接引用与 B4 getattr 动态反射两项，已因
> `pickle` 导入拦截（E0066）与 `getattr` 加入危险内建名单而不可达/被拦截（2026-08 实测）。

## 6. 边界（不保证项——诚实清单）

- **B1 跨文件污点**：单文件编译模型，`import` 的其他 .fly 文件不参与污点分析。
  缓解：`only` 白名单 + 各文件独立 check。
- **B2 通配导入**：`from x import *` 引入的名字未定义检查豁免。
- **B3 第三方模块内部**：模块自身代码是供应链信任，不静态审查。
- **B4 动态反射**：`getattr(obj, 动态字符串)`、`globals()[name]` 的动态形式。
  缓解：`only` 白名单封禁 + 人工评审。
- **B5 业务逻辑**：鉴权、权限、加密等业务语义不在安全关键字职责内。
- **B6 运行时兜底是"统一报错"**：不阻止逻辑错误本身，只保证错误被捕获并带行列号。
- **B7 间接引用**：~~`obj = pickle.loads; obj(x)` 一等函数值流转不溯源~~ —— 2026-08 起
  **不可达**：`pickle` 导入本身被 E0066 拦截，危险 callable 无法通过导入获取（redteam_test.go 锁定）。
  其他模块上的一等函数流转（如 `requests` 包装）仍属 B3 供应链信任。
- **B8 运行时代码注入**：`cgitb/importlib/` 等从外部读取并执行源码的路径。
  缓解：`only` + 文件系统只读沙箱。

## 7. 与静态分析工具的定位差异

| 维度 | Fly-Lang | Bandit | Semgrep | Ruff (S 规则) |
|------|----------|--------|---------|----------------|
| 定位 | **语言**：安全语义内建，检查是编译的一部分 | 插件：pip 项目扫描器 | 模式匹配引擎 | lint 器安全规则 |
| 检查时机 | 编译期（`fly check`/`fly build`/LSP） | CI 单独步骤 | CI 单独步骤 | 编辑/CI |
| 拦截强度 | **硬失败**：编译不过，漏洞版本无法部署 | 软警告：可 `# noqa`、可跳过、误报多 | 规则可禁用 | 可 ignore |
| 污点跨写法 | 别名/容器/属性/参数传播（§3 R1-R5） | 部分规则（黑名单 API 调用） | 需手写 taint 规则 | 无 taint |
| 误报率取向 | 保守偏安全（R5） | 高（需人工 triage） | 取决于规则质量 | 低但覆盖面窄 |
| 运行时兜底 | 8 关键字注入 + `_fly_*` 护栏 | 无 | 无 | 无 |
| 形式化模型 | §3 污点规则 R1-R5 + 边界 §6 | 无 | 无 | 无 |
| 与运行时沙箱 | 配套 fly-sandbox（seccomp） | 无 | 无 | 无 |

**一句话**：Bandit/Semgrep/Ruff 是"扫描提醒"，Fly-Lang 是"编译失败"。
前者帮助你发现风险，后者让风险**无法进入产物**。

## 8. 结论（一句话）

**Fly-Lang 把"风险代码必须编译失败"作为承诺**：通过 `fly check` 的源码中，
外部输入与危险操作之间必须存在显式清洗；被证明无法静态保证的路径（B1/B4/B7）
如实列出，并用 `only` 白名单与 fly-sandbox seccomp 做纵深兜底。

## 9. 外部评估的常见误解与澄清

> 2026-08 外部评审（AI 评估）提出的批评中，三条基于对项目实际设计的误读，
> 在此如实记录并回应；其余真实问题见 §10。

### 9.1 "转译产物是纯 Python，运行时没有任何强制机制" —— 误读

产物恒注入 `runtime` + `sandbox` 两节（`_FlySandbox` 内建代理 + `_fly_attr/_fly_get/_fly_set`
反射黑名单 + 受限 `__import__`（BLOCKED 名单）+ 模块属性拦截 + `_fly_sb_audit` 审计）。
安全保证是**双层**：编译期拦截（纵深第一层）+ 运行时兜底（第二层）。真正的承诺形态是
"沙箱内 Python 子集"（与 RestrictedPython 同类），而非"纯编译期类型系统"。

澄清后的边界：运行时兜底与产物同体——攻击者**改写产物**即可绕过运行时层；
因此编译期拦截（safe/mask/lock 零残留）才是不可篡改的保证，运行时层是对
"未改产物但输入恶意"场景的兜底。

### 9.2 "只能检查 Fly 关键字覆盖的代码路径" —— 误读

`checker/escape.go` 是对**整个 AST 全局**的沙箱逃逸扫描（危险内建名出现在任何读取
位置都报 E0063，与代码块内有无关键字无关）。关键字是**显式安全策略**，escape 是
**隐式全局兜底**。纯 Python 风格代码同样过全局扫描。

### 9.3 "天花板远低于 TypeScript" —— 类比不成立

TS 的保证上限由**类型擦除**决定：类型错误在运行时毫无存在感。Fly 的关键差异是
**运行时沙箱是真实强制层**（注入的 `_FlySandbox` 代码不可被业务代码关闭），
保证形态更接近 WASM 的边界模型而非 TS 的编译期检查模型。代价是用户需接受
"我的代码在沙箱里跑"的心智模型，这是文档与教学要持续投入的方向。

## 10. 外部评估确认的真实问题与整改（已排入 ROADMAP.md）

| # | 问题 | 状态 | 整改（路线图条目） |
| :- | :--- | :--- | :--- |
| G1 | 第三方库内部不透明（B3）+ C 扩展完全不经过管线 | ✅ 已填（试点） | S6 requests 受控包装（examples/third_party/safe_http.fly）+ 信任模型文档（docs/第三方库安全包装.md）；C 扩展仍为已知边界 |
| G2 | 8 关键字组合语义无形式化规范（guard+lock 同变量、only 内 seal 等交互） | ✅ 已填 | S4 交互矩阵文档（docs/关键字交互矩阵.md）+ interaction_test.go 22 组合锁定 |
| G3 | `fly update` 自更新通道无签名验证——安全语言的自更新是信任链关键 | ✅ 已填 | S2 ed25519 签名：tools/sign + verify.go + release.yml；篡改/缺签拒绝安装 |
| G4 | 零残留关键字（safe/mask/lock/guard）在产物中无痕迹，审计/复盘困难 | ✅ 已填 | S5 `--keep-annotations`：产物保留 `# fly-safe:` 等 6 类审计注释 |
| G5 | 无 fuzz / 红队对抗测试——反例测试只覆盖已知模式 | ✅ 已填 | S3 三个 fuzz 目标（lexer/parser/compile，修复 3 个 panic）+ redteam_test.go 19 项对抗；新增属性赋值污点传播 |
| G6 | 错误信息可用性依赖真实用户反馈迭代 | 持续 | 报错清单.md 每码含示例/修复；P6 消息快照 |
| G7 | 类型提示生态割裂（PEP 484 语义冲突的疑问） | ✅ 已澄清 | 文档声明：Fly 不替代类型检查（guard 是值校验）；交互矩阵 §组合规则 3 明确 guard 不构成清洗 |

**整改顺序**：S2（信任链）与 S3（测试盲区）优先于 S4/S5——安全工具自身的
更新通道与对抗测试比功能扩展优先。
