# fly-sandbox

> **PyFly 跨平台轻量沙箱** — Rust + Wasmtime + RustPython
>
> 为 PyFly 转译产物（`.py`）提供 **Linux / macOS / Windows** 三平台一致的
> 轻量级安全隔离执行环境。

## 架构

```
Fly .fly → fly build → .py → RustPython(Wasm) → Wasmtime(Rust宿主)
                                              ↕ host functions (capability)
                                          Rust 宿主进程
```

| 层 | 机制 | 对应 PyFly 关键字 |
|---|---|---|
| **L1 编译期** | Fly 转译器静态检查 | `safe` `only` `lock` `mask` `guard` |
| **L2 Python 运行时** | `_FlyOnly` 代理、`@_fly_cage`、`_fly_seal` | `only` `cage` `seal` `trace` |
| **L3 Wasm 隔离** | 线性内存 + fuel + capability host functions | 全部（纵深防御） |

## 快速开始

### 1. 构建 RustPython Wasm 运行时

```bash
# 前置：rustup target add wasm32-wasip1
python3 build_wasm.py
```

输出：`runtime/rustpython.wasm`

### 2. 构建宿主程序

```bash
cargo build --release
```

输出：`target/release/fly-sandbox`

### 3. 运行 PyFly 脚本

```bash
# 无外部能力（最严格）
./target/release/fly-sandbox script.py

# 授予文件读 + 网络 + 环境变量
./target/release/fly-sandbox script.py \
  --cap-fs-read /tmp/allowed \
  --cap-net-host api.example.com \
  --cap-env DATABASE_URL \
  --fuel 5000000 \
  --timeout-ms 10000
```

## Capability 模型

| CLI 参数 | 作用 | 默认 |
|---|---|---|
| `--cap-fs-read <path>` | 允许读取的文件/目录前缀 | 无（禁止所有） |
| `--cap-fs-write <path>` | 允许写入的文件/目录前缀 | 无（禁止所有） |
| `--cap-net-host <host>` | 允许访问的 HTTP host | 无（禁止所有） |
| `--cap-env <name>` | 允许读取的环境变量 | 无（禁止所有） |
| `--fuel <n>` | Wasm 指令数上限 | 10,000,000 |
| `--timeout-ms <ms>` | 执行超时 | 5000 |
| `--max-memory-pages <n>` | Wasm 内存上限（页） | 1024 (64MB) |
| `--no-audit` | 关闭审计日志 | 开启 |

## 与 PyFly 原生 fly-sandbox 对比

| 维度 | Fly 原生 (seccomp) | 本方案 (Rust+Wasm) |
|---|---|---|
| 平台 | Linux only | ✅ Linux / macOS / Windows |
| 默认权限 | 继承 Python 进程 | ✅ 零权限（capability-based） |
| 资源限制 | SIGALRM + RLIMIT_AS | ✅ fuel + epoch + memory pages |
| 绕过产物 | 改 .py 可绕过运行时 | ✅ 改 .py 无效（Wasm 层拦截） |
| 冷启动 | ~5ms | ~10-50ms |
| 内存开销 | ~10MB | ~15-30MB |

## 安全设计原则

1. **Deny by default** — Wasm 模块无默认系统访问，每个能力需显式授予
2. **Defense in depth** — 编译期 → Python 运行时 → Wasm 隔离 三层
3. **Capability-based** — 白名单路径/host/环境变量，无通配符
4. **Auditable** — 每次 host function 调用记录审计事件
5. **Fail closed** — 任何未授权访问返回空/零，不泄露存在性

## 开发

```bash
# 运行测试
cargo test

# 运行示例
cargo run --release -- examples/safe_math.py
cargo run --release -- examples/file_read.py --cap-fs-read /tmp

# 代码检查
cargo clippy -- -D warnings
cargo fmt --check
```

## 许可证

与 PyFly 主项目保持一致。
