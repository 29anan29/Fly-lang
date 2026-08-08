# AGENTS.md

Fly-Lang 是用 Go 实现的 Python 安全超集转译器。核心文件是 Plan.md（实现方案）和 方案.md（语言设计）。

## 构建与测试命令

- 构建 CLI：`go build -o fly ./cmd/fly`
- 测试：`go test ./...`
- 静态检查：`go vet ./...`
- 手动验证：`./fly build testdata/xxx.fly`（可加 `-o out.py`，用 `python3 out.py` 实跑行为测试）
- VSCode 插件编译：`cd editor/vscode-fly && npm run compile`（改 src/ 后必须重编译）

## 架构概览

```
cmd/fly/main.go      CLI 入口（build/check/run/version/update）
internal/lexer/      词法分析
internal/ast/        AST 节点（必须带 position，报错需要行列号）
internal/parser/     递归下降解析器
internal/checker/    编译期语义检查（报错在此阶段产生）
internal/gen/        代码生成 + 运行时注入
internal/runtime/    go:embed 的 fly_runtime.py
internal/version/    版本注入（Version/Commit/Repo，ldflags -X flylang/internal/version.X）
internal/update/     自更新（GitHub Releases API + SOCKS5/HTTP 代理，零第三方依赖）
tools/icon/          图标生成器（assets/icon.png 产物）
editor/vscode-fly/   VSCode 插件（TextMate 语法高亮 + fly check 诊断）
testdata/            正反例测试文件
```

管线：`Lexer → Parser(AST) → checker（编译期报错）→ gen（输出 Python）`。

## 版本与发布

- 版本注入：`go build -ldflags "-X flylang/internal/version.Version=v1.2.3 -X flylang/internal/version.Commit=<sha> -X flylang/internal/version.Repo=29anan29/Fly-lang"`，CI 用 `VERSION_LDFLAGS` 环境变量传入（见 .github/workflows/release.yml）
- 打 `v*` tag 触发 .github/workflows/release.yml：Linux deb/tar.gz、macOS pkg/dmg、Windows zip/7z SFX installer + GitHub Release 自动发布
- `fly update` 依赖产物命名 `fly-<os>-<arch>.tar.gz|.zip`（internal/update.AssetFor），改 CI 产物名必须同步改这里
- 代理：`--proxy` 支持 `http://`/`https://`/`socks5://[user:pass@]host:port`（socks5.go 手写实现，带认证）

## 编码约定

- 只用 Go 标准库，零第三方依赖
- 不加注释说明代码（除非必要文档注释），代码风格遵循 gofmt
- 错误消息格式：`error: <file>.fly:12:5: <消息>`，行列来自 AST 节点 position
- checker 与 gen 分离：checker 只收集信息/报错，不修改 AST 生成逻辑；gen 负责所有注入展开

## 8 个关键字职责划分

- 纯编译期（checker 拦截，零运行时残留）：`safe`、`mask`、`lock`、`guard`
- 编译期检查 + gen 注入：`only`（`__builtins__` 白名单代理）、`seal`（`__setattr__`）、`trace`（logging）、`cage`（signal/resource 装饰器）

## 测试约定

- testdata 中 `.fly` 文件可用 `# fly:error` 注释标记该行期望编译报错（表驱动反例测试用）
- 修改关键字行为后必须跑 `go test ./...`，确保反例测试仍全部报错
- 行为验证：生成的 `.py` 需 Python 3.10+ 可运行

## VSCode 插件

- `editor/vscode-fly/`：TypeScript 写的轻量插件，无 LSP
- 高亮走 `syntaxes/fly.tmLanguage.json`（TextMate），诊断走 `src/diagnostics.ts` 调 `fly check` 并解析 `error: <file>.fly:12:5: <消息>` 格式
- 构建：`npm run compile`；调试：F5（Extension Development Host）；打包：`vsce package`
- `package.json` 中 `contributes.configuration` 含 `fly.path` 与 `fly.checkOnSave`
