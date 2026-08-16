# Fly-Lang 核心转 Rust 方案

> 状态：提案 / 待评审
> 目标读者：项目维护者
> 配套文档：[../Plan.md](../Plan.md)（Go 版实现方案）、[../方案.md](../方案.md)（语言设计）

## 1. 背景与动机

当前 Fly-Lang 核心为 Go 实现（约 4000 行，零第三方依赖），已发布 v0.1.0。将核心转译为 Rust 的动机：

| 动机 | 说明 |
| :--- | :--- |
| 分发形态 | Rust 静态二进制更小、无运行时依赖，适合嵌入式/VSCode 插件场景 |
| 内存安全 | 编译器处理不可信源码，Rust 所有权模型天然防缓冲区类问题 |
| 生态 | 未来 LSP（tower-lsp）、格式化器等工具链生态 Rust 更成熟 |
| 性能 | 大文件转译/检查更快（增量不明显，但冷启动优于 Go 动态运行时） |

## 2. 目标与非目标

**目标**
- 核心编译管线（lexer / ast / parser / gen / compile 编排）用 Rust 重写；**checker 语义检查留 Go**（D6：并发多文件检查语义保留）
- CLI 行为、错误格式、退出码、产物命名与 Go 版**完全兼容**（VSCode 插件、CI、`fly update` 契约不变）
- testdata 正反例全部复用，golden 输出与 Go 版逐字节一致
- 保持「标准库优先，第三方 crate 最小化」的项目精神

**非目标**
- 不重写 VSCode 插件（TypeScript 不动，只改 `fly.path` 指向即可）
- 不改变语言设计（方案.md 中 8 个关键字的语义不变）
- 不做并行编译/增量缓存（单线程足够）

## 3. 决策点（需维护者拍板）

| # | 决策 | 建议（默认） | 备选 |
| :- | :--- | :--- | :--- |
| D1 | crate 策略 | **std-only 优先**；update 的 HTTPS 请求若 std-only 成本过高，允许 `ureq`（约 1 个依赖，静态链接） | reqwest（重）；完全手写 HTTP+TLS（不可行，TLS 无法 std-only） |
| D2 | 仓库组织 | **同仓库 `src/` 根目录 + `fly` 二进制同名**，Go 源码移入 `legacy-go/`（或按里程碑整体切换） | 新仓库（丢失 issue/CI 历史） |
| D3 | 双轨期 | Go 与 Rust 并行，以 testdata golden 为行为基线；Rust 全绿后才删 Go | 一步到位直接替换（风险高，不推荐） |
| D4 | checker 移植顺序 | **checker 不迁移**（见 D6），Rust 侧只做管线编排与桥接 | 跟随 Go 版 P1→P4 逐块移植（已废弃） |
| D5 | 错误输出 | 保持 Rust 风格错误块逐字节一致（`error[E<CODE>]:` + 源码行 + 下划线 + help/note）（行列号按 **Unicode 字符**计，与 Go 的 rune 语义对齐） | 按字节计（与 Go 不一致，禁止） |
| D6 | **checker 留 Go（并发语义保留）** | `checker` 整体不迁移：Go 侧保留为独立服务进程 `fly-checkd`（stdio JSON-RPC，与 LSP 同管线，`compile.CheckSource` 驱动，goroutine 并发处理多文件）。Rust 主进程 `std::thread` 并发向 checkd 发请求。R5 切换时 checkd 二进制随 Release 一并分发（产物命名 `fly-checkd-<os>-<arch>`），Rust `fly check` 自动发现同目录 checkd | checker 迁移 Rust（丢失 goroutine 并发模型，需重写调度，不选） |

## 4. 目录结构（目标态）

```
fly-lang/
├── Cargo.toml
├── build.rs                  # 注入 commit sha / repo（FLY_VERSION 环境变量注入版本）
├── src/
│   ├── main.rs               # CLI：build/check/run/version/update（checker 走 checkd 桥接）
│   ├── cli.rs                # 参数解析（手写，行为对齐 Go flag 版）
│   ├── diagnostic.rs         # Position + Diagnostic + 聚合（上限 N 条）
│   ├── version.rs            # FLY_VERSION/FLY_COMMIT（build.rs 注入，与 Go ldflags 语义对齐）
│   ├── lexer/                # token.rs（token 定义）、lexer.rs（缩进栈状态机）
│   ├── ast/                  # node.rs（节点枚举）、pos.rs
│   ├── parser/               # 递归下降
│   ├── gen/                  # python.rs（发射）、inject.rs（关键字展开）
│   ├── checkd.rs             # 桥接客户端：spawn 并管理 Go 版 fly-checkd（stdio JSON-RPC）
│   └── update/               # github.rs（API/下载）、socks5.rs（手写）、install.rs（原子替换）
├── fly_runtime.py
├── testdata/                 # 原样复用（golden + 反例）
└── legacy-go/                # Go 版源码归档（checker/checkd 常驻保留，其余迁移完成后删除）
```

## 5. 架构映射（Go → Rust）

| Go | Rust 对应 | 注意点 |
| :--- | :--- | :--- |
| `internal/lexer`（token.go/lexer.go，~800 行） | `src/lexer/` | 缩进栈状态机照搬；token 枚举化；f-string 嵌套引号处理逐字翻译 |
| `internal/ast`（ast.go/pos.go） | `src/ast/` | 见 §6 所有权设计 |
| `internal/parser`（~1000 行） | `src/parser/` | 递归下降直接翻译；`only:`/`cage():` 等新语法为特权路径 |
| `internal/checker` | **不迁移**（D6）：Go 侧保留为 `fly-checkd` 常驻服务，stdio JSON-RPC 暴露 `checkSource`（与 LSP 同一 `compile.CheckSource` 管线，goroutine 并发多文件） | Rust 侧 `checkd.rs` 客户端：spawn 子进程 + 帧协议 + 并发（`std::thread`）分发 |
| `internal/gen`（~500 行） | `src/gen/` | 发射器用 `String` buffer + 缩进计数，输出必须与 Go 逐字节一致 |
| `internal/compile`（管线编排） | `src/main.rs` 内 `compile()` | 错误聚合语义对齐（一次报多个、上限 N） |
| `internal/update`（socks5.go/update.go） | `src/update/` | socks5.go 手写协议直接翻译；HTTP 层见 D1；`AssetFor` 命名规则不变 |
| `internal/version`（ldflags） | build.rs + `env!()` | `fly version` 输出格式不变（`dev` / `vX.Y.Z`） |
| `cmd/fly/main.go` | `src/main.rs` + `cli.rs` | 退出码契约：check 错 1、update 有新版本 2、成功 0 |

## 6. AST 所有权设计（关键）

Go 版 AST 节点是 `struct` + 指针/切片，Rust 需要显式所有权策略：

- **首选：Box 树**
  `enum Node { Stmt(Box<Stmt>), Expr(Box<Expr>), ... }`，子节点用 `Box<Node>` / `Vec<Node>`，无 parent 指针。递归下降天然自顶向下，构建期无需反向引用。
- **禁止 parent 指针**：checker 需要向上查询时用「作用域栈 + 符号表」（Go 版即此做法），不要引入 `Rc<RefCell>` 反向边。
- **arena 备选**（仅当大量交叉引用出现时）：`Vec<Node>` + 索引 + `Arena` 结构，代价是借用检查复杂化。默认不做。
- **位置信息**：每个节点携带 `Pos { line: u32, col: u32 }`（1 基），`Copy`。

## 7. 错误处理与聚合

```rust
pub struct Diagnostic { pub file: String, pub pos: Pos, pub msg: String }
pub struct Report { pub errors: Vec<Diagnostic> }   // 聚合，上限 MAX_ERRORS（与 Go 版一致）
```

- lexer/parser 出错即停（返回 `Err(Report)`）；checker 收集多条再停
- 输出统一 Rust 风格错误块，转义规则与 Go 版 `formatError` 一致（下划线宽度按 Unicode 字符计）
- 不要用 `panic` 传递错误；`?` + 自定义 `Error`（`impl std::error::Error`）承载 `Report`

## 8. 契约兼容清单（不可破坏）

| 契约 | 现状 | Rust 版要求 |
| :--- | :--- | :--- |
| CLI 子命令 | build/check/run/version/update | 同名、同参数（`-o`、`--check`、`--force`、`--proxy`） |
| 错误格式 | Rust 风格错误块 | 逐字节一致（VSCode 插件正则依赖） |
| 退出码 | 0 / 1 / 2 | 一致 |
| 产物命名 | `fly-<os>-<arch>.tar.gz\|.zip`，内含 `fly`/`fly.exe` | 一致（`update.AssetFor` 规则） |
| version 输出 | `vX.Y.Z` + commit | 格式一致 |
| 生成的 .py | Python 3.10+ 合法、golden 逐字节 | 逐字节一致 |
| VSCode 插件 | 调 `fly check` 解析输出 | 零改动可用（只换二进制） |

## 9. 测试策略

- **golden 测试**：`cargo test` 内嵌跑 `testdata/golden/*.fly` → 与 `*.py` 对比（Rust 侧 `include_str!` 加载测试资源），从 R1 起必须全绿
- **反例测试**：`# fly:error` 注释标记行 → 断言编译报错且行号匹配（表驱动）
- **行为测试**：生成的 .py 用 `python3` 实跑（`std::process::Command`，仅 CI 执行）
- **CLI 集成测试**：`tests/cli.rs` 用 `env!("CARGO_BIN_EXE_fly")` 跑子命令，断言退出码与 stdout/stderr
- **update 单测**：socks5 握手用本地 `TcpListener` 假服务器（Go 版 `TestDialSocks5` 翻译）；GitHub API 用本地 HTTP 服务器假数据
- **对齐工具**：迁移期写 `scripts/diff_golden.sh` 对比 Go/Rust 两版输出，输出差异即 bug

## 10. 分阶段计划（R0 → R5）

### R0 骨架与基建（✅ 已交付）
- Cargo.toml、build.rs（注入 commit/repo，FLY_VERSION 注入版本，与 Go ldflags 语义对齐）、CLI 框架（子命令分发 + `fly version`）
- golden 测试框架搭建（tests/golden.rs 枚举 testdata 断言 .fly/.py、.fly/.err 文件对齐全；R1 起逐字节对比）
- 验收：`cargo build --release` 产出 `fly`；`fly version` 输出与 Go 版一致（dev 默认 / `FLY_VERSION` 注入 release）

### R1 lexer + ast + parser（对应 P0）
- ✅ **lexer 已交付**：token 枚举（src/lexer/token.rs，44 关键字含 8 安全关键字）、缩进栈状态机、f-string 前缀、数字/字符串/运算符全量翻译；13 个单测（Go lexer_test 全量对照）；**testdata+example 71 个 .fly 与 Go 版逐 token 零差异**（对照工具 examples/dump_tokens.rs）
- ⬜ ast 节点枚举 + Box 树；parser 递归下降
- 验收：golden 测试 **全绿**（输出与 Go 版逐字节一致）；反例 syntax/with/string 等报错行号一致

### R2 compile 管线 + gen（✅ 已交付）
- ✅ 管线编排、错误聚合、`fly build/check/run` 全功能
- ✅ `src/gen.rs`（~1800 行直译）：8 安全关键字展开（guard/only/trace/cage/seal）、runtime/sandbox 六节按需注入（fly_runtime.py include_str! 内嵌 + section 提取）、类型推导热路径豁免（src/typeinfer.rs，5 轮不动点 + 调用点参数聚合）
- ✅ 验收：golden 15 个 .py **逐字节一致**（tests/build_golden.rs 固化）；testdata/errors 52 反例 build 报错零差异；`fly run` 输出/退出码一致；python3 实跑行为一致；`--keep-annotations` 一致

### R3 update + version 子命令
- GitHub API（ureq 或 std-only 手写 HTTP，见 D1）、AssetFor、tar.gz/zip 解包、原子替换
- socks5 代理（含认证）翻译 + 假服务器单测
- 验收：`fly update --check` 对 v0.1.0 release 返回退出码 2；单测全绿

### R4 checkd 桥接（checker 语义留 Go，D6）
- Go 侧：`cmd/fly-checkd/` 独立二进制，stdio JSON-RPC（帧协议与 LSP 相同），`checkSource {src} → [Diagnostic]`，goroutine 并发处理多文件请求
- Rust 侧：`checkd.rs` 客户端（spawn + 发现同目录二进制 + 并发分发 + 结果聚合），`fly check` 输出与 Go 版一致（错误块/退出码）
- 验收：`fly check` 多文件并发、反例全部报错且消息一致；checkd 缺失时 Rust `fly check` 报友好错误

### R5 切换与退役
- CI：release.yml 换 `dtolnay/rust-toolchain@stable`，跨编译（linux arm64 用 `cross` 或容器），产物命名不变（含 `fly-checkd-<os>-<arch>` 一并分发）
- 删除 legacy-go/ 中除 checker/checkd 外的源码，AGENTS.md/README/CONTRIBUTING 更新
- 验收：打 v1.0.0 tag，Release 产物与 v0.1.0 同构；VSCode 插件直接可用

## 11. 注意事项与坑（checklist）

1. **行列号语义**：Go 按 rune（Unicode 字符）计列。Rust 用 `char_indices()` 或逐 `char` 扫描，禁止按 `byte` 计（中文源码会错位，golden 测试会立刻暴露）
2. **`match` 是 Rust 关键字**：token 枚举里 `Match` 等变体名、变量命名避开；Go 版 `case`/`switch` 相关命名同理审查
3. **HashMap 迭代顺序**：Go 版 map 随机序；Rust `HashMap` 也是无序。gen 阶段凡输出依赖遍历顺序的（如 `globals()` 容器、白名单收集）必须换 `BTreeMap` 或显式排序，否则 golden 不稳定
4. **缩进栈**：Go 版 lexer 用 `[]int` 栈 + token 标记（INDENT/DEDENT/NEWLINE），状态机逐字翻译，注意 EOF 时栈清空的边界
5. **f-string 嵌套**：`f"{a!r:{width}}}"` 类嵌套引号/格式串，Go 版已处理，翻译时保持同一递归逻辑
6. **字符串字面量**：Rust 字符串是 UTF-8 `String`；Python 源码字节流输入先用 `String::from_utf8_lossy` 或显式拒绝非 UTF-8（Go 版行为：rune 解码容错，需对齐）
7. **错误聚合上限**：Go 版有 MAX_ERRORS 上限，Rust 版必须保留同一上限与「先列完再退出」的语义，避免测试漂移
8. **stdout/stderr 分流**：编译报错走 stderr、结果走 stdout，与 Go 版一致（VSCode 插件读 stderr）
9. **`fly run` 的临时文件行为**：Go 版写临时 .py 再调 python3；Rust 版用 `tempfile` 逻辑复刻（std-only 可手写 `std::env::temp_dir` + 随机后缀）
10. **原子替换**：update 的 rename 覆盖在 Windows 上可能因占用失败，Go 版已有 `.new` + rename 逻辑，翻译后补 Windows CI 单测
11. **构建大小**：`--release` + `strip`（cargo release profile `strip = true`、`lto`），目标 tar.gz 与 Go 版同量级（~2-3MB）
12. **不要引入全局可变状态**：Go 的包级变量（如错误集合）→ Rust 显式传参或 struct 持有
13. **crate 白名单**：若 D1 允许 ureq，锁版本并 `cargo deny` 审查；禁用任何带 C 依赖的 crate（分发形态要求纯静态）
14. **CI 缓存**：`Swatinem/rust-cache` 替代 setup-go 缓存；`go-version-file` → rust-toolchain.toml

## 12. 风险

| 风险 | 等级 | 缓解 |
| :--- | :--- | :--- |
| 输出与 Go 版漂移 | 高 | golden 逐字节对比 + 双轨并行期 |
| 行列号 Unicode 差异 | 中 | 中文反例测试用例显式覆盖 |
| update 网络层重写 | 中 | 单测用假服务器；socks5 协议代码逐行对照翻译 |
| 工期 | 中 | R0-R3 可独立交付（checker 语义 R4 最后），每阶段有验收 |
| Rust 学习成本 | 低→中 | 代码总量 ~4000 行，块结构清晰，逐函数直译 |

## 13. 迁移完成后的收益量化

- 二进制：Go ~2.3MB（tar.gz）→ Rust release+strip 预计 1.5-3MB（同量级，静态链接无 libc 依赖项）
- 冷启动：Go 无运行时差异不大；Rust 无 GC，检查大文件峰值内存约降 30-50%
- 维护：单个二进制分发（跨平台静态链接），无需 CGO
