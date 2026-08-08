# Rust 迁移事项清单（To-Do / 决策 / 守则）

> 与 [Rust迁移方案.md](Rust迁移方案.md) 配套。本文只列「待决策 + 待执行」事项，方案细节见主文档。

## 1. 待决策事项（拍板后才能开工）

| # | 事项 | 默认建议 | 状态 |
| :- | :--- | :--- | :--- |
| D1 | HTTPS 层：ureq 单依赖 vs 手写（TLS 无法 std-only，必选其一） | 允许 `ureq`（静态链接、无 C 依赖） | 待定 |
| D2 | 仓库组织：同仓库切换 vs 新仓库 | 同仓库，Go 移入 `legacy-go/` | 待定 |
| D3 | 双轨并行期长度：Rust 全绿后删 Go | 并行至 R4 完成 | 待定 |
| D4 | checker 移植顺序是否跟随 Go 版 P1-P4 进度 | 跟随 | 待定 |
| D5 | 行列号按 Unicode 字符计（与 Go 一致） | 一致，禁改 | 待定 |
| D6 | Rust 版版本号：首版沿用 v0.1.0 语义还是跳 v1.0.0 | 完成 R0-R3 打 v0.2.0，R5 打 v1.0.0 | 待定 |
| D7 | 最低 Rust 版本（MSRV）与 rust-toolchain.toml 固定 | stable 最新 + 固定 toolchain 文件 | 待定 |

## 2. R0 启动事项（第一批）

- [ ] 建 `Cargo.toml` + `build.rs`（commit/repo 注入，对齐 ldflags 语义）
- [ ] `src/main.rs` CLI 框架：build/check/run/version/update 子命令分发 + 退出码契约
- [ ] `src/diagnostic.rs`：Position/Diagnostic/Report + `error: <file>.fly:L:C: msg` 输出
- [ ] golden 测试框架：`cargo test` 加载 `testdata/golden/*` 并对比 `.py`
- [ ] 反例测试框架：`# fly:error` 表驱动断言
- [ ] `rust-toolchain.toml`（固定 channel）+ `.cargo/config.toml`（release strip/lto）
- [ ] 仓库目录调整：`docs/` 备案、Go 源码规划 `legacy-go/` 移动时间点

## 3. 迁移期守则（写进 AGENTS.md）

1. **行为冻结**：R0-R4 期间 Go 版只修 bug，不新增语言功能；语言语义以 方案.md 为准，测试以 testdata 为准
2. **golden 先行**：任何模块移植完成 = 对应 golden/反例全绿，否则不算完成
3. **错误消息冻结**：`error: <file>.fly:L:C: msg` 逐字节不变，改消息必须先改 testdata 快照并双版本同步
4. **产物命名冻结**：`fly-<os>-<arch>.tar.gz|.zip` 与包内 `fly`/`fly.exe` 不变（`update.AssetFor` 契约）
5. **零 C 依赖**：crate 白名单（见方案 §11-13），PR 引入新 crate 需评审
6. **双版本对照**：迁移期每次合入跑 `scripts/diff_golden.sh`，输出差异即 bug，不允许"Rust 版先修"
7. **VSCode 插件契约**：插件零改动可切到 Rust 版二进制（只改 `fly.path`），插件侧不迁、不依赖 Rust 版特有行为

## 4. 验收里程碑（DoD）

- R1 完成：`cargo test` golden 全绿，`fly build/check` 输出与 Go 版 diff 为空
- R3 完成：`fly update --check` 真机（对 GitHub Release）返回码 2；socks5 单测通过
- R4 完成：反例测试全部报错且消息一致；`python3` 行为测试全过
- R5 完成：CI 发布产物与 v0.1.0 同构；`legacy-go/` 删除；文档更新；打 v1.0.0

## 5. 冻结清单（迁移期间禁止改动）

- [ ] `internal/update` 的 `AssetFor` 产物命名
- [ ] `error:` 输出格式与退出码（0/1/2）
- [ ] testdata 全部用例内容（新增用例须双版本同步合入）
- [ ] `fly version` 输出格式
- [ ] gen 输出（生成的 .py）——除非先改 golden
