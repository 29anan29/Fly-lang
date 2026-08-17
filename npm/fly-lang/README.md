# fly-lang (npm)

PyFly 的 npm 分发包：把预编译的 Go 二进制打进 npm 包，`npm install -g` 后直接获得 `fly` 命令。

## 安装

```bash
npm install -g fly-lang
```

支持平台：Linux（x64/arm64）、macOS（x64/arm64）、Windows（x64/arm64）。

## 使用

```bash
fly build example.fly -o out.py
fly check app.fly
fly run app.fly
fly error E0031        # 查询错误码示例报错与修复方法
fly update             # 自更新（GitHub Releases 最新正式版，与原生安装一致）
```

## 工作原理

- `bin/` 打包全部平台的静态二进制（Go 交叉编译，CGO_ENABLED=0）
- `fly` 入口（package.json `bin`）由 `cli.js` 按 `process.platform + process.arch` 选择二进制并透传参数（`spawnSync` inherit stdio，退出码透传）
- 版本号与 Go 版本同步：CI 用 tag（`v0.3.6`）注入 package.json version

## 发布

完整发布走 GitHub Actions（release.yml 的 `npm` job：三平台交叉编译 → 组装 → `npm publish`），依赖 `NPM_TOKEN` secret，未配置时跳过发布只留 artifact。
