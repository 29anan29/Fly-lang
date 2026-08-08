# 贡献指南（Contributing Guide）

感谢你愿意为 Fly-Lang 贡献代码！请阅读本指南，确保提交能顺利合并。

## 项目定位

Fly-Lang 是用 Go 实现的 Python 安全增强超集转译器（Fly → Python，类似 TypeScript → JavaScript）。设计文档：

- [方案.md](方案.md)：语言设计（8 个安全关键字的语义）
- [Plan.md](Plan.md)：实现方案与开发阶段（P0 → P6）

## 编码约定

- **只用 Go 标准库，零第三方依赖**（VSCode 插件除外）
- 代码遵循 gofmt，不加解释性注释（除非必要文档注释）
- 错误消息格式：`error: <file>.fly:12:5: <消息>`，行列来自 AST 节点 position
- checker 与 gen 分离：checker 只收集信息/报错，不修改 AST 生成逻辑；gen 负责所有注入展开

## 提交流程

1. Fork 本仓库，新建功能分支（如 `feat/xxx`、`fix/xxx`）
2. 编码前先看 Plan.md 对应阶段与 testdata 已有用例
3. 新功能必须配套正反例测试：
   - 正例放 `testdata/golden/`（转译输出与同名 `.py` 对比）
   - 反例放 `testdata/errors/`（用 `# fly:error` 注释标记期望报错行）
4. 本地验证必须全绿：

```bash
gofmt -l .        # 无输出
go vet ./...      # 无告警
go test ./...     # 全部通过
```

5. 改 VSCode 插件（`editor/vscode-fly/src/`）后必须重编译：

```bash
cd editor/vscode-fly && npm run compile
```

6. 提交信息用简洁的一句话描述改动，如 `P1: 符号表 + lock/guard 拦截`；提交前自查 `git status`，勿把生成的 `.py`、`.vsix`、node_modules 等产物提交进仓库
7. 发起 PR，说明改动动机、影响范围与验证方式

## 修改关键字行为须知

8 个关键字（safe/only/lock/mask/cage/guard/seal/trace）的职责划分见 AGENTS.md。改动后必须跑 `go test ./...`，确保 testdata 反例仍全部报错——这是防回归的最后防线。

## 发布相关

- 打 `v*` tag 触发 CI 构建多平台安装包并发布 GitHub Release
- `fly update` 依赖产物命名 `fly-<os>-<arch>.tar.gz|.zip`（`internal/update.AssetFor`），改 CI 产物名必须同步修改
- 版本信息用 ldflags 注入（`flylang/internal/version`），见 .github/workflows/release.yml

## 问题报告

Bug 或特性请求请开 Issue，附上：Fly 版本（`fly version`）、操作系统、最小复现代码与期望行为。
