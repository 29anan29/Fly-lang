# Fly Lang for VSCode

[Fly](https://github.com/29anan29/Fly-lang) 语言支持插件（v0.3.0+）：Python 3.10+ 安全增强超集的 LSP 编译期诊断 + 语法高亮。

## 功能

- **LSP 编译期诊断**：通过内置 LSP 服务器（`fly lsp`）实时检查，错误进 Problems 面板
  - 打开 / 编辑（120ms 防抖）/ 保存自动检查
  - 诊断 = 编译管线本身（与 `fly check` 完全一致）：编译通过即产物可信，无需运行时兜底校验
- `.fly` 语法高亮（TextMate，8 个安全关键字分组着色）
- hover：悬停在 `safe/only/lock/mask/cage/guard/seal/trace` 上显示语义说明
- 命令（`Ctrl+Shift+P`）：
  - `Fly: Check` 强制重检当前文件
  - `Fly: Build to Python` 转译当前文件（终端输出）
  - `Fly: Run` 集成终端运行
  - `Fly: Check for Updates` 检查 / 更新编译器（支持 socks5 代理）

## 依赖

需要已安装 **v0.3.0+** 的 fly 编译器（含 `fly lsp` 子命令）。安装：

```bash
# 本地构建
go build -o ~/go/bin/fly ./cmd/fly
# 或从 GitHub Releases 下载后放入 PATH
```

## 设置

| 键 | 默认 | 说明 |
|---|---|---|
| `fly.path` | `fly` | fly 可执行文件路径（须为含 `lsp` 子命令的新版本） |
| `fly.proxy` | `""` | 更新检查代理：`http://`、`https://`、`socks5://user:pass@host:port` |

## 开发

```bash
npm install
npm run compile      # 编译 TypeScript
F5                   # Extension Development Host 调试
npx @vscode/vsce package   # 打包 .vsix
code --install-extension fly-lang-*.vsix   # 安装
```
