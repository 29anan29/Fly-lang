# AGENTS.md

每次更新必须发布CI和releases

Fly-Lang 是用 Go 实现的 Python 安全超集转译器。核心文件是 Plan.md（实现方案）、方案.md（语言设计）、docs/长期规划.md（L0-L5 生态路线，含自举体系）、docs/Rust迁移方案.md（R0-R5 迁移）。

## 构建与测试命令

- 构建 CLI：`go build -o fly ./cmd/fly`
- 测试：`go test ./...`
- 静态检查：`go vet ./...`
- 手动验证：`./fly build testdata/xxx.fly`（默认输出到根目录 `build/` 并保留相对路径；可加 `-o out.py` 指定，用 `python3` 实跑行为测试）
- VSCode 插件编译：`cd editor/vscode-fly && npm run compile`（改 src/ 后必须重编译）

## 架构概览

```
cmd/fly/main.go      CLI 入口（build/check/run/version/update/lsp）
internal/lexer/      词法分析
internal/ast/        AST 节点（必须带 position，报错需要行列号）
internal/parser/     递归下降解析器
internal/checker/    编译期语义检查（报错在此阶段产生）
internal/gen/        代码生成 + 运行时注入
internal/lsp/        LSP 服务器（JSON-RPC over stdio，零依赖：诊断/hover/forceCheck）
internal/runtime/    go:embed 的 fly_runtime.py
internal/version/    版本注入（Version/Commit/Repo，ldflags -X flylang/internal/version.X）
internal/update/     自更新（GitHub Releases API + SOCKS5/HTTP 代理，零第三方依赖）
tools/icon/          图标生成器（assets/icon.png 产物）
editor/vscode-fly/   VSCode 插件（TextMate 高亮 + vscode-languageclient 连 fly lsp）
testdata/            正反例测试文件
```

管线：`Lexer → Parser(AST) → checker（编译期报错）→ gen（输出 Python）`。

## LSP 约定

- `fly lsp`：stdio JSON-RPC 2.0，`Content-Length` 帧；诊断由 `compile.CheckSource`（内存字符串编译）驱动，与 `fly check` 同一管线——改 checker 行为自动同步编辑器诊断
- 支持：initialize/initialized/shutdown/exit、didOpen/didChange(full)/didSave/didClose、publishDiagnostics、hover（8 关键字文档）、自定义通知 `fly/forceCheck`
- 行号转换：诊断 `Line/Col`（1 基）→ LSP 0 基；severity 恒为 1（Error）
- 客户端在 `editor/vscode-fly/src/extension.ts`（vscode-languageclient v10，`start()` 返回 `Promise<void>`，不 push disposable）
- VSCode 插件 v0.2.0+ 要求 fly 含 `lsp` 子命令，旧二进制连接会失败

## 版本与发布

- 版本注入：`go build -ldflags "-X flylang/internal/version.Version=v1.2.3 -X flylang/internal/version.Commit=<sha> -X flylang/internal/version.Repo=29anan29/Fly-lang"`，CI 用 `VERSION_LDFLAGS` 环境变量传入（见 .github/workflows/release.yml）
- 版本渠道细分：`version.IsDev()` 判定（Version 空/`dev`/含 `-dev` → dev 版）；`fly version` 输出 `vX.Y.Z (release)` 或 `0.X.Y-dev (commit)`；`fly update --channel dev|release`（默认随当前版本），dev 渠道查 GitHub prerelease（`update.LatestDev`），无预发布时回退正式版渠道；CI 在 tag 含 `-dev` 时 `gh release create --prerelease`
- 打 `v*` tag 触发 .github/workflows/release.yml：Linux deb/tar.gz、macOS pkg/dmg、Windows zip/7z SFX installer + GitHub Release 自动发布
- `fly update` 依赖产物命名 `fly-<os>-<arch>.tar.gz|.zip`（internal/update.AssetFor），改 CI 产物名必须同步改这里
- 交互式 update：先 `CheckWritable` 预检，不可写时终端（TTY）自动 `sudo <exe> update <原参数>` 提权重试，非 TTY 回退"建议 sudo 重试"提示；确认用 `update.Confirm`，ANSI 颜色由 `isTTY`（ioctl TIOCGWINSZ，见 cmd/fly/tty*.go）控制
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

- `editor/vscode-fly/`：TypeScript，诊断走内置 LSP（`fly lsp` 子命令 + vscode-languageclient v10），TextMate 只管高亮
- 高亮走 `syntaxes/fly.tmLanguage.json`；诊断/hover 由 LSP 提供（与 `fly check` 同一编译管线，见"LSP 约定"）
- 构建：`npm run compile`；调试：F5（Extension Development Host）；打包：`npx @vscode/vsce package`
- `package.json` 中 `contributes.configuration` 含 `fly.path` 与 `fly.proxy`（v0.2.0 起，`fly.checkOnSave` 已移除——LSP 常驻无需该开关）
