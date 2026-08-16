# Fly-Lang CLI Rust 转型方案

> 状态：定稿
> 配套文档：[Rust迁移方案.md](./Rust迁移方案.md)（核心管线迁移，含 D6：checker 留 Go）
> 目标读者：项目维护者

## 1. 背景

核心管线（lexer/ast/parser/gen/compile）逐步迁 Rust（见 Rust迁移方案.md，R1 lexer 已交付）。本方案定义 **CLI 层（`fly` 二进制）** 的转型路径：9 个子命令逐个从 Go 切到 Rust，行为逐字节对齐，双轨共存直到全量切换。

Go 版 CLI 现状（`cmd/fly/`，共 1075 行）：

| 文件 | 行数 | 职责 |
| :--- | :--- | :--- |
| `main.go` | 436 | 子命令分发 + build/check/run/version/error/update/lsp |
| `sandbox.go` | 585 | `fly sandbox` 进程级沙箱（clone ns + Landlock + seccomp，Linux only） |
| `sandbox_other.go` | 15 | 非 Linux stub |
| `tty.go` / `tty_windows.go` | 39 | `isTTY`（ioctl TIOCGWINSZ / Windows CharDevice） |

## 2. 目标形态

- **`fly` 单一二进制已由 `cargo build --release` 产出**（`target/release/fly`），Go 版 CLI（`cmd/fly/`）已退役删除
- 9 个子命令行为、退出码、stdout/stderr 分流、错误格式**逐字节一致**
- VSCode 插件（`fly.path` 指向）、`fly update` 契约、CI 产物命名**零改动**（安装包新增 fly-checkd/fly-sandboxd 两守护进程，Rust update 全量解包替换）
- **checker 不迁移**（D6）：Go 版 checker 以 `fly-checkd` 独立进程提供（stdio 二进制帧，长驻复用），Rust CLI 通过 `checkd.rs` 桥接调用
- **`fly sandbox` 保留 Go 实现**（§4 决策 B）：`fly-sandboxd` 独立进程，Rust CLI spawn 桥接
- **`fly fmt` / `fly analyze` 已 Rust 化**：`src/fmt.rs`（token 流级空白重排，注释保留）+ `src/analyze.rs`（McCabe 循环复杂度/认知复杂度/嵌套/重复/注释比例/命名，fuck-u-code 报告口径），与 Go 版 diff 零差异后 Go 实现删除

## 3. 子命令迁移分阶段（P0 → P6）

每个阶段交付即验收（cargo test + 与 Go 版 diff 零差异），不阻塞后续阶段。

### P0 骨架（✅ 已交付）
- `Cargo.toml`/`build.rs`/`src/lib.rs`/`src/main.rs` 分发框架
- `fly version`：与 Go 版输出一致（`dev (commit7)` 默认 / `FLY_VERSION` 注入 `vX.Y.Z (release)`）
- `tests/golden.rs`：testdata 文件对齐全检查

### P1 零依赖命令（version/help/error）✅ 已交付
| 命令 | Go 依赖 | Rust 实现要点 | 验收 |
| :--- | :--- | :--- | :--- |
| `version` | internal/version | ✅ 已完成（src/version.rs）；build.rs 对齐 Go ldflags 语义（本地构建裸 `dev`，CI 注入 `FLY_VERSION`/`FLY_COMMIT`） | 输出逐字节一致 |
| `help` | usage 常量 | main.rs USAGE 常量（与 Go usage 逐字节一致） | 逐字节一致 |
| `error <E码>` | internal/ast（errorInfo 注册表，66 个错误码） | 注册表由一次性生成器 `tools/gen_errorinfo`（Go，读 `ast.AllErrorInfos`）生成 `src/errorinfo.rs`（475 行，入库）；`src/errorcode.rs` 查询 + 单测（全码连续编号、E0031/31 补零格式） | 66 码全码抽查 + `E9999` 未知码，输出/退出码零差异 |

### P2 check ✅ 已交付（R1 parser 之后按序完成）
- Rust：lexer（R1 交付）→ ast（`src/ast.rs`，461 行翻译，enum + Box 递归）→ parser（`src/parser.rs`，1322 行翻译，28 个单测覆盖语法）→ 语法错误本地报；**语义检查走 checkd 桥接**（Go 侧 `compile.CheckSource`，与 LSP 同管线）
- **checkd 协议定为自定义二进制帧**（非 JSON-RPC）：stdin 帧 `[4B BE len][1B color][path][src]`，stdout 帧 `[4B BE len][1B status][count][code/line(LE)/col(LE)/msg...]`——避免 Rust 端手写 JSON 解析；Go 侧 `cmd/fly-checkd/`（bufio 逐帧、EOF 退出 0），Rust 侧 `src/checkd.rs` 客户端（FLY_CHECKD 环境变量 → 同目录 → PATH 查找）
- 错误渲染在 Rust 端复刻：`src/format.rs` 逐行对照 Go `formatError`（bred/cyan/red ANSI、`%4d` 行号、underlineLen 字节语义、无码诊断无尾部 \n 由 Fprintln 统一补）
- 并发：`std::thread::scope` + 自实现信号量（Mutex+Condvar，std 无 Semaphore），对齐 Go `NumCPU()*2`
- 验收：testdata/errors 52 个反例 diff 零差异；多文件混合/目录递归/空目录/不存在文件/无参数/golden 全场景零差异

### P3 build / run ✅ 已交付（R2 gen 交付）
- Rust：lexer → parser → **gen 注入展开**（src/gen.rs，纯 AST 变换，不依赖 checker）→ 输出 .py；语义检查同样走 checkd（与 check 同管线）
- `-o <out.py>` 与默认 `build/` 目录保留相对路径逻辑（default_out_path 复刻 Go defaultOutPath）
- `fly run`：生成临时 .py → `python3` 执行，临时文件清理
- 验收：testdata/golden 全部 .py 与 Go 版逐字节一致（tests/build_golden.rs 固化）；`python3` 实跑行为一致；errors 52 反例 build/run 报错 stdout/stderr/退出码零差异

### P4 update / lsp ✅ 已交付
- `update`（src/update.rs + src/http.rs）：GitHub API 手写 HTTP/1.1 + rustls(ring) TLS、`asset_for` 产物命名、tar.gz 手写解包 + zip 解包、ed25519 验签（手写 base64）、原子替换、sudo 提权重试、SOCKS5/HTTP 代理（手写协议）、交互确认与 ANSI；安装包解出 **fly + fly-checkd + fly-sandboxd 三二进制**全量替换
- `lsp`（src/lsp.rs + src/json.rs）：JSON-RPC over stdio 重写（诊断/hover/forceCheck，debounce 120ms），checker 经 checkd 长驻复用（CheckdSession）
- 验收：`fly update` 真实全链路（下载真产物→验签→解包→替换→version 输出 release）；LSP full_session 端到端测试

### P5 sandbox ✅ 已交付（决策见 §4）
- `cmd/fly-sandboxd/`（Go 保留组件）：`fly sandbox` 拆分独立进程，stage2 re-exec 剥离 "sandbox" 前缀
- Rust `cmd_sandbox`：FLY_SANDBOXD → 同目录 → PATH 自动发现，参数/stdio/退出码透传
- 验收：正常 exit 0、超时 exit 124、逃逸拦截（os.system 被拒 32512）

### P6 切换与退役 ✅ 已交付
- release.yml 三平台构建切 Rust（dtolnay/rust-toolchain + FLY_VERSION/FLY_COMMIT 注入 + 交叉架构），Go 只构建 fly-checkd/fly-sandboxd
- 安装包（tar.gz/zip/deb/pkg/dmg/7z/npm）内 **fly + fly-checkd + fly-sandboxd 三二进制**
- `cmd/fly/` 退役删除；`internal/format`/`internal/analyze`/`internal/lsp`/`internal/update`/`internal/version` 随 Go CLI 一并删除（Rust 版为唯一实现）
- `fly fmt` / `fly analyze` 迁 Rust（src/fmt.rs + src/analyze.rs），与 Go 版 golden/反例 diff 零差异

## 4. sandbox 子命令的迁移决策

`fly sandbox` 585 行，直接依赖 Go 运行时（os/exec 的 clone ns 实现、syscall 包）做 Landlock/seccomp 白名单。

| 方案 | 说明 | 取舍 |
| :--- | :--- | :--- |
| A. Rust 手写 syscall | clone(CLONE_NEWNS/USER/PID/NET/UTS/IPC) + Landlock(landlock_create_ruleset) + seccomp(BPF) 全部 `libc::`/手写裸 syscall | std-only 精神，但工作量大（~600 行 syscall 代码）；沙箱是安全关键路径，测试成本高 |
| B. 沙箱保留 Go | `fly sandbox` 由独立的 `fly-sandboxd`（Go）提供，Rust CLI spawn 它；主 CLI 全 Rust | 分发作多一个二进制（与 checkd 同机制）；沙箱代码不动，风险最低 |
| C. 删除 sandbox 子命令 | L0-L5 生态规划中 sandbox 是核心卖点，不删 | —— |

**默认 B**（与 checkd 相同的"Go 保留组件"机制，安全关键代码不重写）。Rust CLI 按命令自动发现同目录 `fly-sandboxd`；缺失时报友好错误。若后续 Rust 生态成熟（如引入经审计的 landlock crate）再评估 A。

## 5. 双轨共存与对照

- **双轨期已结束**：Rust `fly`（`cargo build --release`）为唯一 CLI，Go 版 `cmd/fly` 已删除
- 对照方法论（开发期使用，供回归）：对同一组文件跑 Go/Rust 两版 `check/build/fmt/analyze`，diff stdout/stderr/退出码（/tmp/opencode/p6 保留对照脚本）
- 合并原则：**Rust 全绿才切**；CI 中 Rust 版为主，Go 只构建 checkd/sandboxd
- golden/反例快照（testdata）为两版共用行为基线，禁止单方改动

## 6. 契约清单（不可破坏，逐项测试断言）

| 契约 | 说明 | 测试 |
| :--- | :--- | :--- |
| 退出码 | check/build 错 1、update 有新版本 2、成功 0、未知命令 2 | tests/cli.rs |
| stdout/stderr | 编译结果走 stdout、错误块走 stderr（VSCode 插件读 stderr） | tests/cli.rs |
| 错误块格式 | `error[E<CODE>]: 标题` + `--> file:line:col` + 源码下划线 + help/note；Unicode 列号 | testdata/errors/*.err 快照 |
| ANSI | tty 彩色 / 管道无色（isTTY + NO_COLOR）；`FormatErrorColor` 语义 | script 模拟 tty |
| version | `dev` / `dev (commit7)` / `vX.Y.Z (release)` | 单测 |
| error 查询 | `E0031` / `31` 两种格式，补零到 EXXXX | 单测 |
| update 产物 | `fly-<os>-<arch>.tar.gz\|.zip` 内含 fly + fly-checkd + fly-sandboxd | AssetFor + extract_binaries 单测 |
| lsp | stdio JSON-RPC 帧协议、publishDiagnostics | 集成测试 |
| 沙箱 | Landlock+seccomp 兜底行为不变（Go 实现，fly-sandboxd） | 既有安全测试 |

## 7. 风险

| 风险 | 等级 | 缓解 |
| :--- | :--- | :--- |
| 错误块格式漂移 | 高 | .err 快照 + diff_cli.sh 双保险 |
| checkd 桥接延迟 | 中 | checkd 长驻（LSP 模式已验证进程模型）；失败回退提示 |
| update 网络层重写 | 中 | 假服务器单测；socks5 逐行对照翻译 |
| sandbox 双二进制分发 | 低 | 与 checkd 同机制，安装脚本一并处理 |
| 双轨维护负担 | 中 | 明确接棒点（P3 后 CI 默认 Rust check/build）；testdata 为唯一基线 |
