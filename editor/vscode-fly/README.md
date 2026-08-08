# Fly Lang for VSCode

[Fly](https://github.com/29anan29/Fly-lang) 语言支持插件：Python 3.10+ 安全增强超集的语法高亮与编译期诊断。

## 功能

- `.fly` 文件语法高亮（TextMate，基于 Python 语法，8 个安全关键字单独着色）
- 保存 / 打开时自动 `fly check`，错误显示在 Problems 面板（红色波浪线，点击跳转行列）
- 命令面板（`Ctrl+Shift+P`）：
  - `Fly: Check` 手动检查当前文件
  - `Fly: Build to Python` 转译当前文件为同名 `.py`
  - `Fly: Run` 在集成终端运行当前文件
  - `Fly: Check for Updates` 检查 / 安装 fly 编译器新版本（支持 `fly.proxy` 配置走 socks5 代理）

## 依赖

需要已安装 fly 编译器（可从 [Releases](https://github.com/29anan29/Fly-lang/releases) 下载，或用 `fly update` 安装）。插件通过设置 `fly.path` 查找编译器，默认 `fly`（走 PATH）。

## 设置

| 键 | 默认 | 说明 |
|---|---|---|
| `fly.path` | `fly` | fly 可执行文件路径 |
| `fly.checkOnSave` | `true` | 保存时自动检查 |
| `fly.proxy` | `""` | 更新检查代理：`http://`、`https://`、`socks5://user:pass@host:port` |

## 开发

```bash
npm install
npm run compile      # 编译 TypeScript
F5                   # Extension Development Host 调试
vsce package         # 打包 .vsix（需安装 @vscode/vsce）
```
