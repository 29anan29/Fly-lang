<div align="center">

  <img src="assets/logo.svg" alt="Fly Logo" width="200" />

# Fly-Lang

**Python 3.10+ 的安全增强超集转译器**（Go 实现，Fly → Python，类似 TypeScript → JavaScript）

</div>

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/29anan29/Fly-lang)](https://github.com/29anan29/Fly-lang/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/29anan29/Fly-lang/release.yml?label=Release%20CI)](https://github.com/29anan29/Fly-lang/actions)

Fly 是 Python 3.10+ 的安全增强超集：用 Go 实现的转译器，把 `.fly` 源码转译为 Python。新增 8 个安全关键字，在编译期静态检查 + 展开删除，零运行时残留语法；产物默认注入沙箱运行时（见"沙箱"一节）。

详细设计见 [方案.md](方案.md)（语言设计）与 [Plan.md](Plan.md)（实现方案）。

## 构建

```bash
go build -o fly ./cmd/fly
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
fly update --channel dev    # 走 dev 渠道（GitHub 预发布版；默认随当前版本：dev 版→dev，正式版→release）
fly update --proxy socks5://user:pass@host:1080   # 走 SOCKS5/HTTP 代理
```

安装目录不可写时（如 `/usr/bin` 下的旧版），终端里直接执行 `fly update` 会自动通过 `sudo` 提权重试，无需手动 `sudo <路径>/fly update`。版本号细分：`fly version` 显示 `vX.Y.Z (release)`（正式版）或 `0.X.Y-dev (commit)`（dev 版）；打 `vX.Y.Z-dev` 的 tag 由 CI 发布为 prerelease，`fly update --channel dev` 即可获取。

## 用法

```
./fly build [选项] <file.fly>   转译为 Python（含沙箱运行时，见"沙箱"一节）
./fly check <file.fly>...       仅编译检查，支持多文件与目录递归（goroutine 并发，出错退出码 1）
./fly run <file.fly>            转译并在沙箱内执行（python3）
./fly version                   打印版本与提交号
./fly error <E码>               查询错误码示例报错与修复方法（如 fly error E0066）
./fly update [--check|--force|--channel <dev|release>|--proxy <url>]  自更新（见上）
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

每个生成的 `.py` 恒注入 `runtime` + `sandbox` 两个运行时节，沙箱在任意 Python 3.10+ 环境生效，编译期拦截与运行时兜底双层防护：

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

安全模型与威胁分析：**[SECURITY.md](SECURITY.md)** · **[docs/THREAT-MODEL.md](docs/THREAT-MODEL.md)**（含污点规则 R1-R5 形式化、对抗实测表、与 Bandit/Semgrep/Ruff 定位差异）
CVE 对照演示：**[pickle RCE——Python 被打穿 vs Fly 编译期拦截](docs/demo/cve-pickle.md)**（`bash examples/cve-pickle/run-demo.sh`，5 幕含 fly-sandbox 运行时兜底）
性能基准：**[docs/bench/bench.md](docs/bench/bench.md)**（CPython / PyPy / Fly 转译三列对比，`bash bench/run.sh`）
语法兼容矩阵：**[docs/compat.md](docs/compat.md)**（Python 3.10+ 语法实测，当前 74%）

## CVE 演示：pickle 反序列化 RCE（实机截图）

同一段反序列化恶意载荷的代码：原生 Python 上线即被打穿，Fly-Lang 在编译期拦截，即使绕过编译期也有 fly-sandbox seccomp 兜底：

![pickle RCE 演示：Python 被打穿 vs Fly-Lang 编译期拦截](docs/demo/img/cve-pickle-demo.png)

- **原生 Python**：`pickle.loads(payload)` → `os.system("touch /tmp/pwned_by_pickle")` 任意命令执行成功
- **Fly-Lang**：`safe` 污点追踪在编译期报错 `未净化的外部输入 data 流入 pickle.loads（危险汇点）`，含 RCE 的版本无法部署
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

`editor/vscode-fly/`（已发布 v0.2.0）：

- **LSP 编译期诊断**：内置 `fly lsp` 服务器，打开/编辑/保存实时检查，错误进 Problems 面板（与 `fly check` 同一管线）
- `.fly` 语法高亮（TextMate），8 个安全关键字分色
- hover：悬停关键字显示语义说明
- 命令：Build to Python / Run / Check for Updates（支持 socks5 代理）/ Check Extension Update
- 配置：`fly.path`（指向含 `lsp` 子命令的 fly 二进制，默认 `fly`）、`fly.proxy`

### 插件如何更新

插件未上架 VSCode 市场，随 GitHub Release 发布 `.vsix`，更新方式二选一：

1. **命令面板**（推荐）：`Fly: Check Extension Update` —— 自动查询 GitHub 最新 Release，有新版时提示并打开下载页
2. **手动安装**：到 [GitHub Releases](https://github.com/29anan29/Fly-lang/releases) 下载 `fly-lang-<版本>.vsix`，执行：

```bash
code --install-extension fly-lang-<版本>.vsix --force   # 覆盖旧版
```

`fly-lang-<版本>.vsix` 由 CI 在打 tag 时自动构建；插件版本号与 Release tag 版本保持一致。编译器本身升级用 `fly update`。

详见 [editor/vscode-fly/README.md](editor/vscode-fly/README.md)。

## 目录结构

```
cmd/fly/           CLI 入口（build/check/run/version/update/lsp）
internal/lexer/    词法分析
internal/ast/      AST 节点（带位置信息）
internal/parser/   递归下降解析器
internal/checker/  编译期语义检查（含 escape.go 沙箱逃逸拦截 E0063-E0066）
internal/gen/      代码生成 + 运行时注入（恒注入 sandbox）
internal/lsp/      LSP 服务器（JSON-RPC stdio：诊断/hover）
internal/runtime/  fly_runtime.py 运行时支持库（runtime + sandbox 两节）
internal/version/  版本信息（ldflags 注入）
internal/update/   自更新 + SOCKS5 代理
tools/icon/        图标生成器
assets/            logo.svg + icon.png
editor/vscode-fly/ VSCode 插件
docs/              长期规划（L0-L5 生态路线）、Rust 迁移方案
example/            示例：Web 版（http.server）与 PyQt6 桌面版 FlyToDos 复刻
testdata/          正反例测试文件
```
