<div align="center">

  <img src="assets/logo.svg" alt="Fly Logo" width="200" />

# PyFly

**Python 3.10+ 的安全受限超集转译器**（Rust 实现，Fly → Python，类似 TypeScript → JavaScript）

</div>

## 合伙宣言（AI × 人类）

PyFly 由人与 AI 结对开发：**人做决策，AI 写代码**。

- **代码主权在人**。AI 可以生成一万行代码，但每一行都要经过人的评审、测试与取舍才进入仓库。仓库里的每个 commit 背后，是人的判断。
- **AI 是不知疲倦的合伙人**。它负责体力活：翻译、重构、查缺补漏、穷举边界用例；把人的精力留给真正需要智慧的事：语义、边界、取舍、方向。
- **分工清楚**。变革性决策（语言设计、安全模型、架构方向）由人拍板；执行性产出（编码、测试、文档、CI）由 AI 先行完成，人验收。
- **可验证胜过自夸**。AI 的产出不靠信任，靠测试：golden 逐字节对比、反例快照、CI 全绿才合并。
- **AI 不署名，但要留痕**。commit 记录协作过程，让后人知道哪些代码由 AI 生成、如何被验证——这是对代码负责，也是对合作者负责。

---

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/29anan29/PyFly-lang)](https://github.com/29anan29/PyFly-lang/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/29anan29/PyFly-lang/release.yml?label=Release%20CI)](https://github.com/29anan29/PyFly-lang/actions)

Fly 是 Python 3.10+ 的安全受限超集：用 Rust 实现的转译器（CLI 全 Rust，checker/沙箱以独立守护进程提供），把 `.fly` 源码转译为 Python。任何合法 Python 中违反安全规则的模式（危险内建/反射链/危险模块，见 [docs/THREAT-MODEL.md §6.2 不兼容清单](docs/THREAT-MODEL.md#62-不兼容模式清单安全受限超集的精确边界)）会被编译期拦截，其余安全子集零改造可编译；新增 8 个安全关键字，在编译期静态检查 + 展开删除，零运行时残留语法；产物默认注入沙箱运行时（见"沙箱"一节）。

## 文档索引

| 文档 | 内容 |
| :--- | :--- |
| [方案.md](方案.md) | 语言设计（8 关键字语义、语法、示例） |
| [Plan.md](Plan.md) | 实现方案 |
| [ROADMAP.md](ROADMAP.md) | 总览唯一真源（L/R/P/S 阶段整合视图） |
| [SECURITY.md](SECURITY.md) | 安全策略、漏洞上报流程 |
| [CONTRIBUTING.md](CONTRIBUTING.md) | 贡献指南 |
| [docs/THREAT-MODEL.md](docs/THREAT-MODEL.md) | 威胁模型：污点规则 R1-R5、逃逸面 E1-E14、边界 B1-B8、与静态工具定位差异 |
| [docs/沙箱白皮书.md](docs/沙箱白皮书.md) | 进程级沙箱设计与隔离边界 |
| [docs/关键字交互矩阵.md](docs/关键字交互矩阵.md) | 8 关键字组合交互与优先级规则 |
| [docs/compat.md](docs/compat.md) | Python 语法兼容度与缺口 |
| [docs/长期规划.md](docs/长期规划.md) | 生态规划 L0-L5 |
| [docs/Rust迁移方案.md](docs/Rust迁移方案.md) · [docs/Rust迁移事项.md](docs/Rust迁移事项.md) · [docs/CLI-Rust转型方案.md](docs/CLI-Rust转型方案.md) | Rust 迁移方案与进度 |
| [docs/第三方库安全包装.md](docs/第三方库安全包装.md) | 第三方库受控包装（requests 试点） |
| [docs/安全测试报告.md](docs/安全测试报告.md) · [docs/报错清单.md](docs/报错清单.md) | 安全测试与错误码清单 |
| [docs/pip.md](docs/pip.md) | pip 安装方案 |
| [docs/demo/cve-pickle.md](docs/demo/cve-pickle.md) | CVE 对照演示（pickle RCE 编译期拦截） |

## 构建

```bash
cargo build --release                     # fly（target/release/fly）
go build -o target/release/fly-checkd ./cmd/fly-checkd      # 编译检查守护进程
go build -o target/release/fly-sandboxd ./cmd/fly-sandboxd  # 沙箱守护进程（Linux）
```

## 安装

**Linux**

```bash
sudo dpkg -i fly_<ver>_amd64.deb    # 或 arm64
```

**macOS**

```bash
sudo installer -pkg fly-<ver>.pkg -target /
```

**Windows**

```bash
# 运行 fly-<ver>-windows-amd64-installer.exe，或解压 fly-<ver>-windows-amd64.zip 并把 fly.exe 加入 PATH
```

**自更新**（已安装过 fly 后，`fly update` 会从 GitHub Releases 拉取对应平台的压缩包原地升级）：

```bash
fly update                  # 检查并更新到最新版（交互式：展示更新日志 + 询问是否安装）
fly update --check          # 仅检查，有新版本时退出码 2
fly update --force          # 强制更新（即使已是最新）
fly update --proxy socks5://user:pass@host:1080   # 走 SOCKS5/HTTP 代理
```

安装目录不可写时（如 `/usr/bin` 下的旧版），终端里直接执行 `fly update` 会自动通过 `sudo` 提权重试，无需手动 `sudo <路径>/fly update`。

## 用法

```
./fly build [选项] <file.fly>   转译为 Python（含沙箱运行时，见"沙箱"一节）
./fly check <file.fly>...       仅编译检查，支持多文件与目录递归（goroutine 并发，出错退出码 1）
./fly run <file.fly>            转译并在沙箱内执行（python3）
./fly fmt [-w|--check] <file.fly>...  格式化（token 流级空白重排，注释保留；--check 仅检查）
./fly analyze <file.fly>        代码质量评分（100 制：复杂度/嵌套/重复/注释/命名）
./fly lsp                       启动语言服务器（stdio JSON-RPC，供编辑器插件连接）
./fly version                   打印版本与提交号
./fly error <E码>               查询错误码示例报错与修复方法（如 fly error E0066）
./fly update [--check|--force|--proxy <url>]  自更新（见上）
```

build 选项：

```
-o <out.py>   指定输出文件（默认输出到根目录 build/，保留相对路径）
```

示例：

```bash
./fly build testdata/golden/hello.fly    # 输出到 build/testdata/golden/hello.py（保留相对路径）
./fly build testdata/golden/hello.fly -o out.py   # 或指定输出
python3 out.py
./fly check app.fly
./fly check src/ errors/lock_assign.fly # 目录递归 + 多文件并发检查
./fly run testdata/golden/basic.fly
```

示例项目（`example/`）：

```bash
# Web 版待办与灵感（纯标准库 http.server，零依赖）
./fly build example/flytodos/app.fly -o app.py && python3 app.py   # http://127.0.0.1:8765
# 桌面版（PyQt6：pip install PyQt6）
./fly build example/flytodos_qt/flytodos_qt.fly -o flytodos_qt.py && python3 flytodos_qt.py
```

## 沙箱（所有编译产物默认在沙箱内运行）

每个生成的 `.py` 恒注入 `runtime` + `sandbox` 两个运行时节，运行时兜底（内建代理/反射黑名单/受限导入）在任意 Python 3.10+ 环境生效；**OS 层进程隔离为平台原生实现**（`fly sandbox`）：

| 平台 | OS 层机制 | 能力 | 强度声明 |
| :--- | :--- | :--- | :--- |
| Linux | clone ns + Landlock + seccomp + rlimit | 文件系统只读白名单、网络全禁、syscall 白名单、资源上限 | 最强（内核级隔离） |
| macOS | Seatbelt（`sandbox-exec` SBPL 策略） | 文件写仅限临时/用户/当前目录、网络全禁、内存（RLIMIT_AS）/CPU 上限 | 用户态策略（Apple 标记 deprecated 但长期保留），不防御内核漏洞 |
| Windows | Job Object（kill-on-close + 内存/时间/进程数上限） | 进程树生命周期强制终止、总内存上限、墙钟上限 | 用户态限制，不防御文件系统/网络越权 |

编译期拦截与运行时兜底双层防护：

| 逃逸途径 | 编译期（checker，E码） | 运行时（注入代理） |
| :--- | :--- | :--- |
| 危险内建调用/别名（`eval`/`exec`/`open`/`getattr`/`globals`/`vars`/`input` 等 15 个） | 任何读取位置即报 E0063 | `_FlySandbox` 内建代理按名拦截 |
| 反射链（`__class__`/`__bases__`/`__mro__`/`__subclasses__`/`__dict__`/`__code__` 等 16 个） | 属性访问/字面量下标报 E0064 | `_fly_attr`/`_fly_get`/`_fly_set` 黑名单（含变量下标动态逃逸） |
| `__builtins__` 直接访问 | E0065 | 代理 + 反射黑名单 |
| 危险模块导入（`os`/`subprocess`/`sys`/`pickle`/`socket`/`ctypes` 等 ~70 个） | E0066 | 受限 `__import__`（BLOCKED/ALLOWED 双名单，math/json/time/re 等白名单正常） |

- 模块顶层帧缓存 builtins（CPython 架构限制），运行时代理在函数/类体内生效，**顶层逃逸依赖编译期拦截**
- `only` 块内保持更严格的 `_FlyOnly` 白名单代理
- 名单双向同步：checker/escape.go 与 fly_runtime.py 的 `_FLY_SB_*` 必须一致（E0063-E0066 运行时同样抛 `FlyRuntimeError: 沙箱: ...`）

## 8 个安全关键字

| 关键字 | 含义 | 防御目标 |
| :--- | :--- | :--- |
| `safe` | 强制净化污点变量 | SQL/命令/代码注入（编译期污点追踪） |
| `only` | 白名单权限块 | 恶意模块调用（编译期 + `__builtins__` 代理） |
| `lock` | 锁定常量 + 防反射读取 | 常量篡改、`globals()` 泄露（编译期符号表拦截） |
| `mask` | 遮蔽敏感数据 | 密码/token 经日志打印泄露（编译期输出上下文检测） |
| `cage` | 限制资源（内存/CPU/时间） | 无限循环、大内存分配（运行时 signal + resource 装饰器） |
| `guard` | 强制输入验证（类型/格式/范围） | 未校验外部输入（编译期生成断言） |
| `seal` | 冻结对象，禁止增删改属性 | 对象属性被篡改（编译期生成 `__setattr__` 拦截） |
| `trace` | 强制审计日志 | 关键操作无记录（编译期插入 logging） |

安全模型与威胁分析：**[SECURITY.md](SECURITY.md)** · **[docs/THREAT-MODEL.md](docs/THREAT-MODEL.md)**（含污点规则 R1-R5 形式化、逃逸面 E1-E12 枚举、对抗实测表、与 Bandit/Semgrep/Ruff 定位差异） · **[关键字交互矩阵](docs/关键字交互矩阵.md)**（8 关键字任意组合 + escape 全局扫描的优先级规则，22 组合测试锁定）
CVE 对照演示：**[pickle RCE——Python 被打穿 vs Fly 编译期拦截](docs/demo/cve-pickle.md)**（`bash examples/cve-pickle/run-demo.sh`，5 幕含 fly-sandbox 运行时兜底）
性能基准：**[docs/bench/bench.md](docs/bench/bench.md)**（CPython / PyPy / Fly 转译三列对比，`bash bench/run.sh`。摘要：纯算术 ≈0 税、内建代理 2.6-3.7×、启动税 ~75ms、顶层代码零税——详见护栏成本构成节）
语法兼容矩阵：**[docs/compat.md](docs/compat.md)**（Python 3.10+ 语法实测，当前 74%）

## CVE 演示：pickle 反序列化 RCE（实机截图）

同一段反序列化恶意载荷的代码：原生 Python 上线即被打穿，PyFly 在编译期拦截，即使绕过编译期也有 fly-sandbox seccomp 兜底：

![pickle RCE 演示：Python 被打穿 vs PyFly 编译期拦截](docs/demo/img/cve-pickle-demo.png)

- **原生 Python**：`pickle.loads(payload)` → `os.system("touch /tmp/pwned_by_pickle")` 任意命令执行成功
- **PyFly**：`safe` 污点追踪在编译期报错 `未净化的外部输入 data 流入 pickle.loads（危险汇点）`，含 RCE 的版本无法部署
- **纵深防御**：即使攻击者绕过编译期直接部署原生代码，`fly-sandbox`（seccomp 白名单）也会拦截 `openat` 写操作（SIGSYS），攻击命令失败

完整 5 幕演示与"为什么运行时检测拦不住"分析见 [docs/demo/cve-pickle.md](docs/demo/cve-pickle.md)。

## 编译期完备检查（可信产物）

Fly 的检查全部发生在**编译期**（lexer → parser → checker），通过检查的代码**不会产生原生 Python 运行时崩溃**：

- `fly check` / `fly build` 通过 = 语义合法，生成的 `.py` 可直接在生产环境运行
- **静态拦截**：未定义名称（模块级顺序敏感 + 函数体宽松）、本地函数参数个数、常量除零、字面量类型操作、重复定义
- **运行时兜底**：无法静态确定的动态错误（动态除零、下标越界/KeyError、属性缺失、`int()` 转换失败、不可迭代等）由 gen 注入 `_fly_*` 护栏，统一转 `FlyRuntimeError`（带源码行列号 `src:行:列`），不再裸抛 Python 异常
- 不需要"先编译再用 python 解释器跑一遍来验证"——编译通过即可信，像 Rust 一样把问题拦在编译前
- 8 个安全关键字的检查在编译期拦截或注入运行时护栏（见下表），产物本身即安全边界
- 编辑器内 LSP 诊断与 `fly check` 是**同一管线**：写代码时看到的错误 = 编译时必然报的错误

## 报错格式

Rust 风格：每个错误带错误码、源码行高亮与修复建议（`help`）和威胁模型/文档关联（`note`）：

```
error[E0031]: 污点数据流入危险汇点
  --> app.fly:10:23
   |
  10 |     obj = pickle.loads(data)
   |                       ^^^^^^
   |
   = help: 未净化的外部输入 data 流入 pickle.loads（危险汇点）。对该值先做净化（白名单/类型校验）再流入汇点，或改用 only 白名单约束
   = note: SECURITY.md 污点分析一节
```

全部错误码与消息清单见 [docs/报错清单.md](docs/报错清单.md)，终端可用 `fly error E0031` 查询任意错误码的示例报错与修复方法。

## LSP（编辑器编译期诊断）

`fly lsp` 内置 LSP 服务器（JSON-RPC over stdio，零第三方依赖）：

- 实时编译期诊断（打开/编辑/保存即检查，120ms 防抖）
- hover 显示 8 个安全关键字语义与所在行源码
- 诊断 = 编译管线本身，与 `fly check` 完全一致（见上节"可信产物"）
- 供 VSCode 插件（vscode-languageclient）与任意 LSP 客户端接入

## 测试

```bash
go test ./...
go vet ./...
```

- `testdata/golden/`：正例，转译输出与同名 `.py` golden 对比（全部经 `python3` 实跑行为验证，含 p4_cage 超时/超内存双场景）
- `testdata/errors/`：反例，必须编译报错（含 escape_* 系列沙箱逃逸反例：危险内建/反射链/`__builtins__`/危险模块导入）
- 生成的 `.py` 需 Python 3.10+ 可运行
- 安全测试体系（5 层：反例快照/名单全覆盖 188 断言/运行时兜底/健全性/golden 实跑）见 **[docs/安全测试报告.md](docs/安全测试报告.md)**

## 发布流水线

`.github/workflows/release.yml`：打 `v*` tag 触发，构建并发布多平台安装包到 GitHub Releases：

| 产物 | 平台 |
| :--- | :--- |
| `.deb`（amd64/arm64，`dpkg -i` 安装） | Linux |
| `.tar.gz`（amd64/arm64） | Linux |
| `.pkg` + `.dmg`（universal） | macOS |
| `.zip`（amd64/arm64） | Windows |
| `-installer.exe`（7-Zip SFX 自解压） | Windows |

## VSCode 插件

`editor/vscode-pyfly/`（已发布 v0.2.0）：

- **LSP 编译期诊断**：内置 `fly lsp` 服务器，打开/编辑/保存实时检查，错误进 Problems 面板（与 `fly check` 同一管线）
- `.fly` 语法高亮（TextMate），8 个安全关键字分色
- hover：悬停关键字显示语义说明
- 命令：Build to Python / Run / Check for Updates（支持 socks5 代理）/ Check Extension Update
- 配置：`fly.path`（指向含 `lsp` 子命令的 fly 二进制，默认 `fly`）、`fly.proxy`

### 插件如何更新

插件未上架 VSCode 市场，随 GitHub Release 发布 `.vsix`，更新方式二选一：

1. **命令面板**（推荐）：`Fly: Check Extension Update` —— 自动查询 GitHub 最新 Release，有新版时提示并打开下载页
2. **手动安装**：到 [GitHub Releases](https://github.com/29anan29/PyFly-lang/releases) 下载 `pyfly-lang-<版本>.vsix`，执行：

```bash
code --install-extension pyfly-lang-<版本>.vsix --force   # 覆盖旧版
```

`pyfly-lang-<版本>.vsix` 由 CI 在打 tag 时自动构建；插件版本号与 Release tag 版本保持一致。编译器本身升级用 `fly update`。

详见 [editor/vscode-pyfly/README.md](editor/vscode-pyfly/README.md)。

## 目录结构

```
src/               Rust CLI（lexer/ast/parser/gen/checkd/fmt/analyze/lsp/update/http）
cmd/fly-checkd/    编译检查守护进程（Go，stdio 二进制帧，Rust CLI 桥接）
cmd/fly-sandboxd/  沙箱守护进程（Go，Landlock+seccomp+ns，Rust CLI 桥接）
internal/lexer/    词法分析（Go 版，checkd 管线）
internal/ast/      AST 节点（带位置信息）
internal/parser/   递归下降解析器
internal/checker/  编译期语义检查（含 escape.go 沙箱逃逸拦截 E0063-E0066）
internal/gen/      代码生成 + 运行时注入（恒注入 sandbox）
internal/compile/  编译管线入口（check/build，checkd 使用）
internal/runtime/  fly_runtime.py 运行时支持库（runtime + sandbox 两节）
tools/icon/        图标生成器
assets/            logo.svg + icon.png
editor/vscode-pyfly/ VSCode 插件
docs/              长期规划（L0-L5 生态路线）、Rust 迁移方案
example/            示例：Web 版（http.server）与 PyQt6 桌面版 FlyToDos 复刻
testdata/          正反例测试文件
```
