# Fly-Lang 终极路线图

> **唯一真源（single source of truth）**：本文件是所有阶段（L/R/P/S）的唯一整合视图。
> 实施细节见各专项文档，本文件只负责总览、阶段编排、验收标准与优先级决策。
> 专项文档：`Plan.md`（语言实现）、`方案.md`（语言设计）、`docs/长期规划.md`（L0-L5 生态）、
> `docs/Rust迁移方案.md`（R0-R5 核心迁移）、`docs/CLI-Rust转型方案.md`（P0-P6 CLI 迁移）、
> `docs/THREAT-MODEL.md`（威胁模型与边界）、`docs/安全测试报告.md`、`docs/沙箱白皮书.md`。

---

## 1. 现状盘点（2026-08）

| 域 | 条目 | 状态 |
| :--- | :--- | :--- |
| 语言 | 8 关键字（safe/only/lock/mask/cage/guard/seal/trace） | ✅ P0-P7 全部落地 |
| 语言 | 污点引擎（R1-R5）、escape 全局扫描、运行时注入（guard/only/seal/trace/cage 节） | ✅ |
| 语言 | 进程级沙箱 fly-sandbox（clone ns + Landlock + seccomp） | ✅ Linux only |
| CLI | `fly build/run/sandbox/version/error/update/lsp`（Go） | ✅ |
| CLI | Rust 版 `version/help/error`（P1）+ `check`（P2，checkd 桥接） | ✅ 与 Go 零差异 |
| Rust | lexer（R0/R1）、ast、parser、checkd 客户端、错误渲染 format | ✅ |
| 工程 | VSCode 插件（LSP）、npm 包、GitHub Actions 发布链、自更新 | ✅ |
| 质量 | testdata 反例 52+、golden、checker 单测 | ✅ |
| 安全 | 威胁模型、边界清单 B1-B8、外部评估回应（THREAT-MODEL §9/§10） | ✅ 本次落地 |

**已填缺口（2026-08）**：G3 签名 ✅、G5 fuzz+红队 ✅、G2 交互矩阵 ✅、G4 审计注释 ✅、
G7 类型生态澄清 ✅、G1 wrapper 试点 ✅（S6，requests）。G6 错误信息迭代（S7）随 L0 P6 推进。

---

## 2. 阶段总览（统一时间线）

```
现状 ──L0──► v0.3.0（基线收尾）──L1──► v1.0.0（Rust 全量）
   └───── S 系列安全专项（S1-S6，随阶段插入，S2/S3 优先）──────┐
v1.0.0 ──L2──► v1.1（工具链自举）──L3──► v2.0（自举编译器闭环）
   ──L4──► v2.x（开放协作）──L5──► v3.0（生态与治理）
```

### L0 基线收尾（→ v0.3.0）
| 任务 | 说明 | 验收 |
| :--- | :--- | :--- |
| P6 打磨 | golden 全量覆盖、错误消息快照、边界用例（闭包/嵌套/f-string 内 mask） | `方案.md` 8 示例正反例全过 |
| 行为测试接入 CI | 生成 .py 由 python3 实跑 | CI 全绿 |
| S1 边界文档 | THREAT-MODEL §9/§10（本文件）+ README 安全边界声明 | 已交付 |

### S 系列安全专项（插队规则：S2/S3 优先，不阻塞 L1）
| 条目 | 任务 | 优先级 | 验收 |
| :--- | :--- | :--- | :--- |
| S2 | `fly update` 产物签名（ed25519，零第三方）+ 校验，失败拒绝安装 | 🔴 高 | ✅ 已交付：tools/sign + verify.go + release.yml 签名步骤 + 5 单测（正签/篡改/缺签/换钥/--insecure） |
| S3 | Go fuzz（parser/checker）+ 逃逸红队回归 | 🔴 高 | ✅ 已交付：3 个 fuzz 目标（lexer/parser/compile，seed=testdata 全量），修复 3 个 parser panic；redteam_test.go 19 项对抗回归，新增属性赋值污点传播，B7 确认不可达 |
| S4 | 8 关键字交互矩阵文档 + 表驱动组合测试 | 🟡 中 | ✅ 已交付：docs/关键字交互矩阵.md + interaction_test.go 22 组合 |
| S5 | 可选 `--keep-annotations`：产物保留安全标记注释（审计/复盘） | 🟡 中 | ✅ 已交付：fly build --keep-annotations，6 类注解（safe/mask/lock/guard/only/seal/trace/cage） |
| S6 | 第三方库 wrapper 试点（requests/sqlalchemy 任一） | 🟢 低 | ✅ 已交付：requests 受控包装（examples/third_party/safe_http.fly + docs/第三方库安全包装.md）；修复 guard 消息字符串转义 bug；运行时白名单与编译期 taint 源点一致化（allow_requests_s6 防漂移测试） |
| S7 | 错误信息用户测试迭代（G6） | 持续 | 🔲 随 P6 消息快照推进 |

> S2 待用户操作：`~/.fly-sign/priv.pem`（本次生成）内容配置到 GitHub Actions secret `SIGN_PRIVATE_KEY`，否则 CI 跳过签名（无 .sig 时 fly update 默认拒绝安装）。

### L1 Rust 迁移（→ v1.0.0）
| 阶段 | 内容 | 状态 |
| :--- | :--- | :--- |
| R0/R1 | lexer（token/缩进/字符串） | ✅ |
| R1.5 | ast + parser（1322 行翻译） | ✅ |
| P1/P2 | version/help/error + check（checkd 桥接，零差异） | ✅ |
| R2 | gen + 运行时注入（only/seal/trace/cage/runtime/sandbox 节） | 🔲 |
| P3 | `fly build` / `fly run`（checkd 复用，golden 逐字节一致） | 🔲 |
| R3 | checker 语义子集自举（可选，checker 留 Go 为 D6 默认） | 🔲 |
| P4 | `fly update`（签名先行，见 S2）/ `fly lsp` | 🔲 |
| R4 | compile 管线整合、错误聚合 20 条上限 | 🔲 |
| P5 | `fly sandbox`（决策 B：fly-sandboxd 二进制桥接） | 🔲 |
| R5/P6 | 切换与退役：release.yml 切 Rust 构建，产物命名不变，AGENTS.md/README 更新 | 🔲 |

验收：v1.0.0 与 Go 版 CLI diff 零差异；单静态二进制（<3MB）；VSCode/CI/update 契约零改动。

### L2 工具链自举（→ v1.1）
- `fly fmt`（AST 驱动，注释保留，**用 Fly 编写**，Rust 壳调用）
- `fly doc`（docstring → Markdown）、`fly test`（目录级测试运行器）
- Neovim/JetBrains LSP 客户端配置
- 验收：`fly fmt` 格式化自身源码后 golden 无差异（dogfooding 自证）

### L3 自举编译器（→ v2.0）
- gen 发射器用 Fly 重写（参考实现，D-1 双实现互证）
- 前端 `flyc.fly` 自编译闭环：`fly build flyc.fly → python3 flyc.py` 编译任意 .fly
- 验收：Fly 写的编译器能编译自身并产出可运行 .py

### L4 开放协作（→ v2.x）
- RFC 流程、CHANGELOG 自动生成、文档站（GitHub Pages + docsify，D-3）、playground
- 安全评审清单 + 漏洞披露流程
- 验收：外部贡献者 PR 全流程无维护者人工干预

### L5 生态与治理（→ v3.0）
- 标准库/包管理寄生 Python（pip/PyPI 复用，无自研 registry）
- 合规行业试点（金融/医疗/政务）与合规报告模板
- 成功指标：示例库 10+、外部贡献 PR ≥ 1（自 L4 持续）

---

## 3. 关键决策（已定，勿反复）

| # | 决策 | 结论 |
| :- | :--- | :--- |
| D1 | checker 是否迁移 Rust | **留 Go**（D6），走 checkd 桥接；R3 可选自举 |
| D2 | 工具语言 | 用 Fly 写 fmt/doc/test（自举优先），Rust 壳调用 |
| D3 | 文档站 | GitHub Pages + docsify（零成本） |
| D4 | 自举编译器形态 | 编译为 .py 由 python3 跑（无引导问题） |
| D5 | sandbox 迁移 | 决策 B：保留 Go 实现，`fly-sandboxd` 二进制桥接（安全关键代码不重写） |
| D6 | 包管理 | 寄生 Python（pip/PyPI），无自研 registry、无 flyx 包 |

## 4. 风险

| 风险 | 等级 | 缓解 |
| :--- | :--- | :--- |
| 安全保证被"改写产物"绕过（THREAT-MODEL §9.1） | 高 | 文档诚实声明 + 编译期零残留关键字为不可篡改层 |
| S2/S3 拖延导致信任链缺口 | 高 | 插队规则：S2/S3 优先于 L1 功能 |
| Rust 迁移与工具链并行冲突 | 中 | 串行：L1 完成前不做 L2+ |
| 自举版漂移 | 中 | golden diff + 双实现互证 |
| 改名撞车（fly 名称已被占用） | 中 | docs/规划.md 建议 FlySafe/PyFly 子品牌，v1.0 前评估 |
| 单人维护精力 | 低 | 全自动 CI/发布 + RFC 流程 |

## 5. 成功指标

- v0.3.0：8 关键字全落地 + S1-S3 完成（签名 + fuzz 就位）
- v1.0.0：Rust CLI 全量零差异，单二进制，update 签名验证上线
- v1.1：`fly fmt` 自举（dogfooding 闭环）
- v2.0：自举编译器 `flyc` 闭环
- v2.x+：示例库 10+、外部贡献 PR ≥ 1、合规试点 ≥ 1

## 6. 版本时间线速览

```
v0.3.0 ──── L0 + S1-S3
v1.0.0 ──── L1（Rust 全量 + P3/P4/P5/P6）
v1.1   ──── L2（工具链自举）
v2.0   ──── L3（自举编译器 flyc）
v2.x   ──── L4（开放协作）
v3.0   ──── L5（生态与治理）
```
