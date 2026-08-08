<div align="center">

  <img src="assets/logo.svg" alt="Fly Logo" width="200" />

# Fly-Lang

**Python 3.10+ 的安全增强超集转译器**（Go 实现，Fly → Python，类似 TypeScript → JavaScript）

</div>

[![Go](https://img.shields.io/badge/Go-1.21+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Release](https://img.shields.io/github/v/release/29anan29/Fly-lang)](https://github.com/29anan29/Fly-lang/releases)
[![Build](https://img.shields.io/github/actions/workflow/status/29anan29/Fly-lang/release.yml?label=Release%20CI)](https://github.com/29anan29/Fly-lang/actions)

Fly 是 Python 3.10+ 的安全增强超集：用 Go 实现的转译器，把 `.fly` 源码转译为纯净 Python。新增 8 个安全关键字，在编译期静态检查 + 展开删除，零运行时残留语法。

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
fly update                  # 检查并更新到最新版
fly update --check          # 仅检查，有新版本时退出码 2
fly update --force          # 强制更新（即使已是最新）
fly update --proxy socks5://user:pass@host:1080   # 走 SOCKS5/HTTP 代理
```

## 用法

```
./fly build [选项] <file.fly>   转译为 Python
./fly check <file.fly>          仅编译检查（出错退出码 1）
./fly run <file.fly>            转译并执行（python3）
./fly version                   打印版本与提交号
./fly update [--check|--force|--proxy <url>]  自更新（见上）
```

build 选项：

```
-o <out.py>   指定输出文件（默认与源文件同名 .py）
```

示例：

```bash
./fly build testdata/golden/hello.fly -o out.py
python3 out.py
./fly check app.fly
./fly run testdata/golden/basic.fly
```

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

## 报错格式

```
error: <file>.fly:12:5: <消息>
```

## 测试

```bash
go test ./...
go vet ./...
```

- `testdata/golden/`：正例，转译输出与同名 `.py` golden 对比
- `testdata/errors/`：反例，必须编译报错
- 生成的 `.py` 需 Python 3.10+ 可运行

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

`editor/vscode-fly/`：`.fly` 语法高亮（TextMate）、保存自动 `fly check` 诊断（Problems 面板）、Build/Run/Check for Updates 命令（更新支持 socks5 代理）。详见 [editor/vscode-fly/README.md](editor/vscode-fly/README.md)。

## 目录结构

```
cmd/fly/           CLI 入口（build/check/run/version/update）
internal/lexer/    词法分析
internal/ast/      AST 节点（带位置信息）
internal/parser/   递归下降解析器
internal/checker/  编译期语义检查（P1 起）
internal/gen/      代码生成 + 运行时注入
internal/runtime/  fly_runtime.py 运行时支持库（P4 起）
internal/version/  版本信息（ldflags 注入）
internal/update/   自更新 + SOCKS5 代理
tools/icon/        图标生成器
assets/            logo.svg + icon.png
editor/vscode-fly/ VSCode 插件
testdata/          正反例测试文件
```
