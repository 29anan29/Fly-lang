# PyFly 实现方案

Fly = Python 3.10+ 安全增强超集，用 Go 实现转译器（Fly → Python，类似 TypeScript → JavaScript）。
新增 8 个安全关键字，编译期检查 + 展开删除，输出纯净 Python。

## 语言定位

- Fly 源码是 Python 3.10+ 超集，转译后为合法 Python，无残留语法
- 8 个关键字：`safe` `only` `lock` `mask` `cage` `guard` `seal` `trace`
- 编译期报错输出到终端，退出码 1；`fly check` 仅检查不输出代码

## 编译管线

```
.fly → Lexer → Parser(AST) → 语义分析(checker) → 编译期报错 → 代码生成 → .py
                 │                              │
                 └── lock/guard/mask/safe 拦截 ──┴── only/seal/trace/cage 展开注入
```

## 项目结构（Go 标准库，零第三方依赖）

```
flylang/
├── go.mod
├── cmd/fly/main.go            # CLI: fly build <in.fly> [-o out.py] / fly check / fly run
├── internal/
│   ├── lexer/                 # Python 3.10+ 子集 + 8 关键字 token
│   ├── ast/                   # 节点定义 + 位置信息（报错行列号）
│   ├── parser/                # 递归下降；only:/cage():/seal class/mask x 等新语法
│   ├── checker/
│   │   ├── symbol.go          # 符号表、作用域、lock 只读拦截、import 收集
│   │   ├── taint.go           # 赋值 def-use 链：safe(源→危险汇点) mask(敏感→输出上下文)
│   │   ├── whitelist.go       # only 块内名字 vs 白名单
│   │   └── guard.go           # guard 条件合法性验证
│   ├── gen/
│   │   ├── python.go          # 缩进化 Python 代码发射
│   │   └── inject.go          # seal/trace/cage/only 运行时注入
│   ├── version/               # Version/Commit/Repo（ldflags -X 注入）
│   ├── update/                # 自更新：GitHub Releases API + socks5/http 代理（手写 SOCKS5）
│   └── runtime/fly_runtime.py # go:embed；GuardError/ResourceExhaustedError、白名单代理、冻结容器
├── tools/icon/                # 图标生成器（输出 assets/icon.png）
├── assets/                    # logo.svg + icon.png
├── .github/workflows/release.yml  # 打 v* tag → deb/pkg/dmg/zip/SFX + GitHub Release
├── editor/vscode-fly/         # VSCode 插件（TypeScript）
│   ├── package.json           # contributes: languages/grammar/commands/configuration
│   ├── syntaxes/fly.tmLanguage.json  # TextMate 语法高亮（Python 子集 + 8 关键字）
│   ├── src/extension.ts       # 激活逻辑
│   ├── src/diagnostics.ts     # 调 `fly check` 解析报错 → Problem Panel
│   └── src/commands.ts        # build/run/update 命令
└── testdata/                  # .fly 正反例（# fly:error 标记期望编译错误）
```

## VSCode 插件

轻量方案，不引入 LSP（编辑器体验先行，LSP 留待后续）：

- **语法高亮**：TextMate grammar（`syntaxes/fly.tmLanguage.json`），基于 Python 语法注入 8 个关键字样式（`safe`/`mask`/`lock`/`guard` 与 `only`/`cage`/`seal`/`trace` 可区分色）
- **编译期诊断**：`diagnostics.ts` 保存时或手动触发，后台执行 `fly check <file.fly>`，解析 `error: file:line:col: msg` 输出到 Problems 面板
- **命令**：`Fly: Build to Python`（转译当前文件并显示/保存 .py）、`Fly: Check`（手动检查）
- **配置**：`fly.path`（fly 可执行文件路径，默认 `fly`）、`fly.checkOnSave`（默认 true）
- **未装 fly 时的降级**：提示安装并给出构建命令

开发与打包：`npm install` → `npm run compile` → F5 调试（Extension Development Host）；打包用 `vsce package`。

## 关键字落地分工

| 关键字 | 编译期动作 | 运行时残留 |
| :--- | :--- | :--- |
| `safe` | 污点追踪：I/O 源（request/input）→ `safe` 声明点；未净化变量传入 eval/exec/os.system/SQL 拼接等汇点 → 报错 | 无 |
| `only` | 白名单校验：块内名字/import 与白名单比对，出现 os/subprocess 等 → 报错 | 注入 `__builtins__` 白名单代理，重写块内 import |
| `lock` | 符号表只读标记：拒绝再赋值、AugAssign、setattr、`globals()['X']` 反射读取 | 生成冻结常量容器 |
| `mask` | 反向追踪：mask 变量出现在 print/logging/f-string 输出上下文 → 报错 | 无 |
| `cage` | 解析 max_time/max_memory 参数，按块包装 | 生成装饰器：`signal.SIGALRM` 计时 + `resource.setrlimit` 限内存，抛 `TimeoutError`/`ResourceExhaustedError` |
| `guard` | 验证约束为布尔表达式、变量与断言匹配 | 展开为 `if not (...): raise GuardError(...)` |
| `seal` | 标记类、收集类属性 | 类体内注入 `__setattr__`/`__delattr__`，非初始化阶段增删改一律抛错 |
| `trace` | 解析 level/args/ret 参数 | 函数入口/出口插入 logging 调用，含参数与返回值 |

## 报错格式

```
error: <file>.fly:12:5: lock 变量 SECRET_KEY 不可再赋值
```

行列为 AST 节点位置。

## 测试策略

- 单元：lexer/parser 各语法节点
- checker：testdata 表驱动反例测试（`# fly:error` 注释标记期望错误，编译必须报错）
- 集成：golden 测试对比转译输出
- 行为：生成的 .py 用 Python 3.10 实跑验证 cage/seal/trace 运行时行为

## 发布与自更新

- 打 `v*` tag → `.github/workflows/release.yml`：Linux（deb/tar.gz，amd64+arm64）、macOS（pkg/dmg universal）、Windows（zip + 7z SFX installer），`gh release create` 自动发布
- `fly update`：查 GitHub Releases latest → 按 `fly-<os>-<arch>.tar.gz|.zip` 匹配下载 → 校验 → 原子替换自身；`--check` 有新版本退出码 2；`--proxy` 支持 `http(s)://` 与 `socks5://[user:pass@]host:port`（手写 SOCKS5，零依赖）
- 版本注入：`-ldflags "-X flylang/internal/version.Version=... -X ...Commit=... -X ...Repo=29anan29/Fly-lang"`，未注入时显示 `dev`

## 开发过程（P0 → P6）

每阶段退出标准（DoD）：`go vet ./...` 与 `go test ./...` 全绿；新功能有正反例测试；生成的 .py 用 Python 3.10 实跑通过。

### P0 基础设施（CLI + Lexer + Parser + AST）

**目标**：端到端骨架跑通——任意不含关键字的子集 .fly 可无损转译并执行。

任务：
1. `go.mod` + 目录骨架（cmd/internal/testdata）
2. `internal/ast/pos.go`：Position + Diagnostic 错误类型（`error: <file>:<line>:<col>: <msg>`）
3. `internal/lexer/`：token 定义（Python 3.10+ 子集 + 8 关键字）、缩进栈、f-string、字符串转义、数字/注释
4. `internal/ast/`：节点全集（模块/函数/类/if/for/while/import/赋值/表达式/调用/f-string/异常）
5. `internal/parser/`：递归下降，parse 出模块树；解析错误也带行列号
6. `internal/gen/`：最小 AST→Python 编码器（只做格式重排，不注入）
7. `cmd/fly/main.go`：`build`（默认写同目录 .py、`-o` 指定）/ `check`（只检查退出码 1）/ `run`（临时文件跑 python3）
8. testdata 首批正例 + golden 测试框架（输出快照对比）

验收：`hello.fly` 转译后 `python3` 实跑输出一致；错误样例报错格式正确。

### P1 符号表 + lock + guard（纯编译期先行）

**目标**：编译器第一次"拦截"能力——`lock` 冻结常量、`guard` 展开断言，零运行时残留。

任务：
1. `checker/symbol.go`：作用域符号表（模块/函数/类）、名字收集、import 表
2. `lock` 拦截：对 lock 名的 Assign/AugAssign/`setattr`/`globals()['X']`/`vars()['X']` 一律报错
3. `guard` 验证：变量必须已定义、条件为布尔表达式；gen 展开为 `if not (...): raise GuardError(...)`
4. checker 错误聚合（一次报多个错误，最多 N 条上限）
5. testdata 反例：lock 篡改、guard 类型不匹配等，用 `# fly:error` 标记

验收：`lock SECRET = "x"` 后任何再赋值/反射读取编译失败；`guard age: int, 0 < age < 150` 展开正确。

### P2 污点引擎（safe / mask）

**目标**：数据流分析落地——`safe`（源→危险汇点）与 `mask`（敏感→输出上下文）双向污点。

任务：
1. `checker/taint.go`：def-use 链（赋值/函数调用参数/返回值/容器元素/属性传播）
2. 源点表：`request.*`、`input`、`os.environ` 等 I/O 源
3. 汇点表：`eval`/`exec`/`os.system`/`subprocess.*`/SQL 字符串拼接
4. `safe x` 声明点语义：声明的污点变量被清洗（`int(x)` 等）后解除污点；未清洗流入汇点→报错
5. `mask x`：变量流入 `print`/`logging.*`/f-string/字符串格式化→报错；允许哈希/比较等非输出上下文
6. 反例测试：`safe uid` 后 `eval(uid)` 报错、`mask password` 后 `print(password)` 报错；正例（清洗后/比较后）不误报

验收：文档示例 1（safe）与 4（mask）的注释行全部编译报错，正文行全部通过。

### P3 注入型关键字（only / seal / trace）✅ 完成

**目标**：代码生成开始"改写"——白名单块、类冻结、审计日志。

任务：
1. `only (mods):` 块：解析白名单、块内名字/import 比对（os/subprocess/eval 等→报错）；gen 重写块内 import + 注入 `__builtins__` 白名单代理 ✅（checker/only.go 黑名单 + gen `_FlyOnly` 代理 + `_fly_patch_builtins` 函数包装）
2. `seal class`：收集类属性；gen 在类体注入 `__setattr__`/`__delattr__`（非 `__init__` 阶段增删改抛错）；实例属性赋值（`admin.role = "user"`）编译期可静态识别则直接报错 ✅（checker 静态拦截 + 运行时 `_fly_seal_initializing` 门控）
3. `trace(level=, args=, ret=)`：函数入口/出口插入 logging 调用（参数与返回值）✅（gen 包装函数 + `_fly_trace_impl_` 原函数保留）
4. 反例：only 块内 `os.system` 报错、seal 类实例赋值报错；正例：合法模块调用通过 ✅（testdata/errors/only_seal.fly、testdata/golden/p3.fly）
5. fly_runtime.py 起步：GuardError 等异常类型 ✅（internal/runtime/fly_runtime.py go:embed，按 `# fly:section:` 标记提取注入 guard/only/trace 三节）

验收：文档示例 2/7/8 的注释行全部报错；生成的 .py 结构正确且 Python 3.10 语法合法。✅

### P4 资源约束（cage + runtime 库）✅ 完成

**目标**：运行时防御闭环，`cage` 限制时间/内存并抛规范异常。

任务：
1. `cage(max_time=, max_memory=):` 解析参数（"5s"/"100MB" 单位换算）✅（parser 解析 max_time: ms/s/m/h、max_memory: B/KB/MB/GB(+iB)，运行前换算为秒/字节）
2. gen 包装：装饰器生成（`signal.SIGALRM` 超时 + `resource.setrlimit` 限内存 + 清理恢复）✅（`@_fly_cage(...)` 装饰器注入 cage 块内函数；setitimer 计时 + RLIMIT_AS 软限压降，finally 恢复）
3. fly_runtime.py 完善：`TimeoutError`/`ResourceExhaustedError` 包装、资源恢复逻辑 ✅（cage 节：`_fly_cage` 装饰器 + `ResourceExhaustedError` + `_fly_timeout_handler`；MemoryError→ResourceExhaustedError 转换；只降软限避免无权限硬限恢复失败）
4. 行为测试：`while True` 超时抛错、大数组分配抛 ResourceExhaustedError（python3 实跑）✅（testdata/p4_cage.fly 双场景触发 + 嵌套 cage/重复调用/正常快路径验证）

验收：文档示例 5 转译后实跑，超时与超内存两种场景均触发并抛对应异常。✅

### P5 VSCode 插件（已升级为 LSP 版）

**目标**：编辑器体验——高亮 + LSP 编译期诊断。

任务：
1. `internal/lsp`：零依赖 JSON-RPC over stdio 服务器（Content-Length 帧、请求/通知分发、错误聚合）
2. 能力：initialize/initialized/shutdown/exit、didOpen/didChange(full)/didSave/didClose、publishDiagnostics（`compile.CheckSource` 内存编译，120ms 防抖）、hover（8 关键字文档 + 所在行）、自定义 `fly/forceCheck`
3. `cmd/fly` 新增 `lsp` 子命令（stdio）
4. `editor/vscode-fly/` 插件：vscode-languageclient v10 连接 `fly lsp`，TextMate 高亮保留，命令（Build/Run/Update/ForceCheck）
5. 配置 `fly.path`/`fly.proxy`；`fly.checkOnSave` 移除（LSP 常驻）
6. 验证：lsp 单测（管道模拟完整会话）+ `vsce package` + `code --install-extension`

验收：F5 调试下 .fly 文件实时诊断（与 `fly check` 同管线）；hover 出关键字说明；`fly lsp` 冒烟会话（initialize/didOpen/shutdown/exit）通过。

### P5.5 发布生态（CI 打包 + 自更新 + 图标）

**目标**：一键发布多平台安装包，`fly update` 原地升级。

任务：
1. `tools/icon/` 图标生成器（多边形光栅化 → assets/icon.png）
2. `internal/version`：ldflags 注入版本/提交/仓库
3. `internal/update`：GitHub API 查版本、AssetFor 按平台匹配、tar.gz/zip 解包原子替换、socks5 代理（含认证）手写实现 + 表驱动测试
4. `cmd/fly` 新增 `version`/`update` 子命令（--check 退出码 2 / --force / --proxy）
5. `.github/workflows/release.yml`：deb/tar.gz/pkg/dmg/zip/SFX 五类产物 + Release 发布
6. VSCode 插件补 `Fly: Check for Updates`（复用 `--check`/`--force` + `fly.proxy` 配置）

验收：本地 dpkg-deb 冒烟打包通过；`go test ./...` 全绿。

### P6 打磨与收尾 ✅ 完成

任务：
1. golden 测试全量覆盖 + 错误消息快照测试 ✅（TestErrorSnapshots：24 个 errors 反例的 `.err` 快照逐行比对，与 LSP 同一 FormatError 管线）
2. 边界用例：嵌套作用域、闭包、递归、f-string 内 mask 变量、only 嵌套 ✅（golden/edge.fly 正例防误报：shadowing/闭包/递归/嵌套 only/safe 清洗；errors/edge_nesting.fly 反例：闭包内 mask 泄露、嵌套 only 内 os、深层嵌套 mask）
3. 文档：README（安装/用法/8 关键字速查）、报错清单整理 ✅（docs/报错清单.md：全部错误消息 + 运行时异常表；README 链接）
4. `go vet`、gofmt 全绿；`vsce package` 产出 .vsix ✅（插件 v0.3.0，本地打包 12.56 KB 通过）

验收：CI 式一键命令 `go test ./... && go vet ./...` 全绿；对方案.md 全部 8 个示例做最终回归。✅（testdata/acceptance/all8_ok.fly 正例全部通过并 python3 实跑；errors/all8_err.fly 反例 9 条错误全部命中）

里程碑：P0-P6 全部完成，发布 v0.3.0（正式版 release 渠道）。

### P7 编译期完备检查（check 通过 = 无原生运行时崩溃）✅ 完成

编辑器 check 优化 + 编译前检查强化，两阶段保证：

1. **静态检查扩展**（checker/runtime.go）：
   - 未定义名称：模块级顺序敏感（引用须先赋值/import），函数体宽松（参数+局部+外层+内置）
   - 本地函数参数个数：不足/过多/未知关键字/重复传值
   - 常量除零：`/`、`//`、`%` 字面量 0
   - 重复定义：同作用域函数/类/import 撞名、参数重复
   - 字面量类型操作：二元运算/一元/下标/in 的类型兼容表
2. **运行时兜底**（gen 注入 `_fly_*` 护栏 + FlyRuntimeError）：
   - `_fly_binop`（含除零）/`_fly_unary`/`_fly_get`/`_fly_set`/`_fly_attr`/`_fly_setattr`/`_fly_cmp`/`_fly_iter`/`_fly_cast`（int/float）
   - 动态错误统一 `FlyRuntimeError: src:行:列: 描述`，不再裸抛 Python 异常
   - only 代理放行 `_fly_` 前缀 + FlyRuntimeError/GuardError
3. 编辑器同步：LSP 与 `fly check` 同一 CheckSource 管线，新检查自动生效；`fly build` 编译前检查本就同管线

验收：新增 undefined_name/dup_def/literal_type/arg_count 反例 + 快照；golden 全量（含转译注入回归）；TestRuntimeCatch 实跑验证 5 类动态错误全部转 FlyRuntimeError；`go test ./... && go vet ./...` 全绿。

## 里程碑（与阶段对应）

P0 基础设施 ✅ → P1 lock/guard ✅ → P2 safe/mask ✅ → P3 only/seal/trace → P4 cage+runtime → P5 VSCode 插件 → P5.5 发布生态 → P6 打磨 → P7 编译期完备检查

### 当前进度

- P1（已完成）：`internal/checker/`（symbol.go 作用域符号表 + lock.go 冻结常量拦截 + guard.go 条件验证）、parser 支持 `lock`/`guard` 语法（含 `guard x > 0` 纯条件形式）、gen 展开 guard 为 `if not (...): raise GuardError(...)` 并注入 `GuardError` 类、错误聚合上限 20 条、LSP 诊断同步；golden（lock/guard）+ 9 个 errors 反例 + checker 单测全绿
- P2（已完成）：`checker/taint.go` 污点引擎（顺序数据流：赋值/容器/属性/下标传播、清洗 `int`/`float`/`bool`、函数返回污点、f-string 变量提取）；源点（`input()`/`request.*`/`os.environ`）；汇点（`eval`/`exec`/`compile`/`os.system`/`subprocess.*`/SQL `execute`；`print`/`logging.*` 输出）；`safe`/`mask` 零运行时残留；golden safe_mask + 7 个 errors 反例 + 污点单测全绿；`方案.md` 示例 1/4 验收通过
