# AGENTS.md

每次更新必须发布CI和releases

Fly-Lang 是用 Rust 实现的 Python 安全超集转译器（CLI 全 Rust，checker/沙箱为 Go 守护进程，P0-P6 已全部交付）。**总览唯一真源是 ROADMAP.md**（L/R/P/S 各阶段整合视图）；核心文件是 Plan.md（实现方案）、方案.md（语言设计）、docs/长期规划.md（L0-L5 生态细节）、docs/THREAT-MODEL.md（威胁模型与安全边界，含外部评估回应）、docs/Rust迁移方案.md（R0-R5 迁移）、docs/CLI-Rust转型方案.md（CLI 子命令 P0-P6 迁移，checker 留 Go 走 checkd 桥接）。

## 构建与测试命令

- 构建 Rust CLI：`cargo build --release`（产物 `target/release/fly`，并需同目录 `fly-checkd`：`go build -o target/release/fly-checkd ./cmd/fly-checkd`、`fly-sandboxd`：`go build -o target/release/fly-sandboxd ./cmd/fly-sandboxd`）
- Rust 测试：`cargo test`；Go 测试：`go test ./...`
- 静态检查：`go vet ./...`
- 版本注入：build.rs 读 `FLY_VERSION`/`FLY_COMMIT` 环境变量（CI 设 `VERSION=${github.ref_name}`、`SHA=${github.sha}`）
- 手动验证：`./target/release/fly build testdata/xxx.fly`（默认输出到根目录 `build/` 并保留相对路径；可加 `-o out.py` 指定，用 `python3` 实跑行为测试）
- VSCode 插件编译：`cd editor/vscode-fly && npm run compile`（改 src/ 后必须重编译）

## 架构概览

```
src/                 Rust CLI 源码（lib.rs 各模块：lexer/ast/parser/checkd/fmt/analyze/format/errorcode/errorinfo/lsp/json/http/update/version）
cmd/fly-checkd/      编译检查守护进程（Go，stdio 二进制帧协议，编译管线与 fly check 相同）
cmd/fly-sandboxd/    沙箱守护进程（Go：clone ns 建 user/mount/pid/net/uts/ipc → Landlock → seccomp 白名单 → exec python3，rlimit 由注入 python wrapper 设置）
internal/lexer/      词法分析
internal/ast/        AST 节点（必须带 position，报错需要行列号）
internal/parser/     递归下降解析器
internal/checker/    编译期语义检查（报错在此阶段产生）
internal/gen/        代码生成 + 运行时注入
internal/compile/    编译管线（CheckSource/FormatErrorColor，checkd 使用）
internal/runtime/    go:embed 的 fly_runtime.py
tools/icon/          图标生成器（assets/icon.png 产物）
editor/vscode-fly/   VSCode 插件（TextMate 高亮 + vscode-languageclient 连 fly lsp）
npm/fly-lang/       npm 包装器（预编译二进制打进 npm 包，npm install -g fly-lang）
testdata/            正反例测试文件
```

管线：`Lexer → Parser(AST) → checker（编译期报错）→ gen（输出 Python）`。

## LSP 约定

- `fly lsp`：stdio JSON-RPC 2.0，`Content-Length` 帧；诊断由 checkd（Go 编译管线 CheckSource）驱动，与 `fly check` 同一管线——改 checker 行为自动同步编辑器诊断
- 支持：initialize/initialized/shutdown/exit、didOpen/didChange(full)/didSave/didClose、publishDiagnostics、hover（8 关键字文档）、自定义通知 `fly/forceCheck`
- 行号转换：诊断 `Line/Col`（1 基）→ LSP 0 基；severity 恒为 1（Error）
- 客户端在 `editor/vscode-fly/src/extension.ts`（vscode-languageclient v10，`start()` 返回 `Promise<void>`，不 push disposable）
- VSCode 插件 v0.2.0+ 要求 fly 含 `lsp` 子命令，旧二进制连接会失败

## 版本与发布

- 版本注入：build.rs 读 `FLY_VERSION`/`FLY_COMMIT` 环境变量（CI 设 `VERSION=${github.ref_name}`、`SHA=${github.sha}`）；Go 侧 `go build -ldflags "-X flylang/internal/version.Version=..."` 仅用于 checkd/sandboxd 场景
- 版本细分：`version.IsDev()` 判定（Version 空/`dev`/含 `-dev` → dev 版）；`fly version` 输出 `vX.Y.Z (release)` 或 `0.X.Y-dev (commit)`；`fly update` 只检查 GitHub 最新正式版（无渠道参数）
- 打 `v*` tag 触发 .github/workflows/release.yml：Linux deb/tar.gz、macOS pkg/dmg、Windows zip/7z SFX installer + GitHub Release 自动发布
- npm 发布（`npm` job）：三平台 job 交叉编译二进制上传 artifact（`npm-*`）→ `npm` job 组装到 `npm/fly-lang/bin/` → `npm pack` 出 tgz → `npm publish`（账号 anan29_china）；版本号由 tag 注入 package.json；发布依赖 `NPM_TOKEN` secret，未配置时跳过发布只留 artifact
- VSCode Marketplace 发布（`vsix` job 内）：`vsce package` 出 vsix（attach 到 GitHub Release）后 `vsce publish --skip-duplicate`（publisher 29anan29，扩展 `29anan29.fly-lang`）；版本号由 tag 注入 editor/vscode-fly/package.json（`sed "s/\"version\": \"[0-9][^\"]*\"/..."`）；依赖 `VSCE_PAT` secret（marketplace 管理页获取），未配置时跳过发布只留 vsix artifact
- `fly error <E码>`：查询错误码示例报错/修复方法，支持 `E0031`/`31` 格式（自动补零到 EXXXX）；错误码 E0001 起连续编号，注册表源头 `internal/ast/errors.go`（每码含 Title/Help/Note/Example），Rust 版 `src/errorinfo.rs` 由 `tools/gen_errorinfo` 生成（改 errors.go 后需 `go run ./tools/gen_errorinfo` 重新生成，errorcode.rs 有全码抽查单测）
- 新增 errorf 消息必须登记错误码（Go：internal/ast/errors.go codeForFormat；Rust：src/diagnostic.rs error_code 双份同步）
- `fly update` 依赖产物命名 `fly-<os>-<arch>.tar.gz|.zip`（src/update.rs asset_for），改 CI 产物名必须同步改这里；安装包内必须含 fly + fly-checkd + fly-sandboxd 三二进制（src/update.rs extract_binaries 按名解包）
- 产物签名（S2）：发布时 release.yml 用 `go run ./tools/sign` 对每个产物签 `.sig`（ed25519，私钥 `SIGN_PRIVATE_KEY` secret）；客户端 `fly update` 默认强制验签（公钥内嵌 src/update.rs `SIGN_PUB_KEY`，密钥对用 `go run ./tools/sign genkey` 生成，私钥存 `~/.fly-sign/priv.pem`）；缺 .sig 拒绝安装，`--insecure` 跳过（仅测试源）
- fuzz（S3）：`go test -fuzz FuzzParse -fuzztime 60s ./internal/parser/`（另有 FuzzLexer/FuzzCheckSource）；改 parser/checker 后先跑 fuzz 再提交
- 关键字组合（S4）：改关键字行为后必须过 `internal/checker/interaction_test.go`（22 组合）与 `redteam_test.go`（19 对抗）
- fmt/analyze：`fly fmt` 是 token 流级空白重排（注释保留、前置 check 必须通过，语法错误文件跳过）；`fly analyze` 输出 100 制评分（McCabe 循环复杂度/认知复杂度/嵌套/重复/注释比例/命名）；实现是 Rust `src/fmt.rs` + `src/analyze.rs`（Go 版已删除）；改 fmt/analyze 后跑 `cargo test` 与 `./target/release/fly fmt --check testdata/golden/`（全仓 .fly 必须 fmt 干净）；注意产物内嵌源码行列号，fmt 改变行号后必须重新生成 golden .py（`fly build -o testdata/golden/x.py testdata/golden/x.fly`）
- 交互式 update：先 `check_writable` 预检，不可写时终端（TTY）自动 `sudo <exe> update <原参数>` 提权重试，非 TTY 回退"建议 sudo 重试"提示；确认用 `confirm`，ANSI 颜色由 stderr is_terminal + FORCE_COLOR/NO_COLOR 控制
- 代理：`--proxy` 支持 `http://`/`https://`/`socks5://[user:pass@]host:port`（src/http.rs 手写实现，带认证）

## 编码约定

- 只用 Go 标准库，零第三方依赖
- 不加注释说明代码（除非必要文档注释），代码风格遵循 gofmt
- 错误消息格式：Rust 风格 `error[E<CODE>]: <标题>` + `--> file:line:col` + 源码行下划线高亮 + `= help:` 修复建议 + `= note:` 文档链接；错误码注册表在 `internal/ast/errors.go`（format→code 自动匹配，新增 errorf 消息必须登记错误码），渲染在 `internal/compile/compile.go` 的 `formatError`，行列来自 AST 节点 position
- checker 与 gen 分离：checker 只收集信息/报错，不修改 AST 生成逻辑；gen 负责所有注入展开

## 8 个关键字职责划分

- 纯编译期（checker 拦截，零运行时残留）：`safe`、`mask`、`lock`、`guard`
- 编译期检查 + gen 注入：`only`（`__builtins__` 白名单代理）、`seal`（`__setattr__`）、`trace`（logging）、`cage`（signal/resource 装饰器）

## 沙箱（默认注入，所有编译产物在沙箱内运行）

- 生成 .py 恒注入 `runtime` + `sandbox` 两节（顺序：guard/only/trace/cage/runtime/sandbox，sandbox 依赖 runtime 的 FlyRuntimeError）
- 运行时：`_FlySandbox` 内建代理（DANGEROUS 名单按名拦截 eval/exec/open/getattr/globals/vars 等）+ `_fly_attr/_fly_get/_fly_set` 反射黑名单（`__class__/__bases__/__subclasses__/__dict__` 等 + `__builtins__/__traceback__/gi_frame/f_globals` 帧与模块逃逸）+ 受限 `__import__`（BLOCKED 模块 os/subprocess/sys/pickle 等，ALLOWED 白名单 math/json/time 等）+ 模块属性拦截（`_fly_sb_check_modattr`：ModuleType 上的 BLOCKED 子模块与 `attrgetter/itemgetter`）+ 拦截审计（`_fly_sb_audit` 输出 `[fly-sandbox] audit:` 到 stderr，gen 注入 import 时带行列号）
- 编译期：checker/escape.go 同名单拦截（E0063 危险内建、E0064 反射链/模块属性/异常帧/f-string 内联表达式、E0065 `__builtins__`、E0066 危险模块导入）——名单必须与 fly_runtime.py 的 `_FLY_SB_*` 保持一致
- f-string 词法层是整体 STRING token，编译期对花括号内表达式二次解析复用 escape 遍历（解析失败走文本级名单匹配兜底）；错误位置归一到 f-string 字面量起始
- 模块绑定跟踪：escapeCheck 记录 import 绑定名（`modBinds`），白名单模块对象上的危险子模块属性（`random.os`）与 `attrgetter/itemgetter` 编译期拦截
- CPython 模块顶层帧缓存 builtins（f_builtins），运行时代理只在函数/类体内生效；**顶层逃逸靠编译期拦截**（危险内建名出现在任何读取位置都报 E0063）
- 沙箱内 prelude 与注入代码禁止裸用危险内建：一律 `_fly_sb_builtins.getattr(...)` / `_fly_sb_module_globals[...]`（`globals()` 不是 builtins 模块属性）
- `_FlyOnly`/`_FlySandbox` 必须实现 `__getitem__`（CPython 对非 dict 的 `__builtins__` 走下标访问）

## 并发 check

- `fly check <file.fly>...` 支持多文件与目录递归（.fly），线程并发（信号量 `available_parallelism*2`），结果按输入顺序输出，任一失败退出码 1
- LSP 诊断仍为单线程 CheckSource（协议串行）

## 测试约定

- testdata 中 `.fly` 文件可用 `# fly:error` 注释标记该行期望编译报错（表驱动反例测试用）
- 修改关键字行为后必须跑 `go test ./...`，确保反例测试仍全部报错
- 行为验证：生成的 `.py` 需 Python 3.10+ 可运行

## VSCode 插件

- `editor/vscode-fly/`：TypeScript，诊断走内置 LSP（`fly lsp` 子命令 + vscode-languageclient v10），TextMate 只管高亮
- 高亮走 `syntaxes/fly.tmLanguage.json`；诊断/hover 由 LSP 提供（与 `fly check` 同一编译管线，见"LSP 约定"）
- 构建：`npm run compile`；调试：F5（Extension Development Host）；打包：`npx @vscode/vsce package`
- `package.json` 中 `contributes.configuration` 含 `fly.path` 与 `fly.proxy`（v0.2.0 起，`fly.checkOnSave` 已移除——LSP 常驻无需该开关）
