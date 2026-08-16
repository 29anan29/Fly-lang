//! fly-sandbox: 为 PyFly 转译产物提供跨平台轻量沙箱
//!
//! 架构: Rust (host) + Wasmtime (Wasm runtime) + RustPython (Python-in-Wasm)
//!
//! 编译: cargo build --release
//! 运行: cargo run --release -- script.py --cap fs_read=/tmp/allowed --fuel 1000000
//!
//! 依赖 (Cargo.toml):
//! [dependencies]
//! wasmtime = "26"
//! wasmtime-wasi = "26"
//! clap = { version = "4", features = ["derive"] }
//! serde = { version = "1", features = ["derive"] }
//! serde_json = "1"
//! tokio = { version = "1", features = ["full"] }
//! tracing = "0.1"
//! tracing-subscriber = "0.3"
//! anyhow = "1"

use std::collections::HashSet;
use std::path::PathBuf;
use std::time::Duration;

/// PyFly 沙箱能力声明（capability set）
/// 对应 PyFly 关键字的运行时映射：
///   only    → fs_read/fs_write 路径白名单
///   cage    → fuel + max_memory_pages + timeout_ms
///   trace   → audit_log = true
///   mask    → 自动脱敏（无需显式开关）
///   safe    → （编译期已处理）
///   lock    → （编译期已处理）
///   seal    → （Python 层 _fly_seal 令牌）
///   guard   → （编译期生成断言 + 运行时）
#[derive(Debug, Clone)]
pub struct FlyCapabilities {
    /// 允许读取的文件路径前缀（空 = 禁止所有文件读）
    pub fs_read_allow: HashSet<PathBuf>,
    /// 允许写入的文件路径前缀（空 = 禁止所有文件写）
    pub fs_write_allow: HashSet<PathBuf>,
    /// 允许访问的 HTTP host 白名单（空 = 禁止所有网络）
    pub net_allowed_hosts: HashSet<String>,
    /// 允许读取的环境变量名（空 = 禁止所有环境变量）
    pub env_allowed: HashSet<String>,
    /// Wasm fuel 上限（指令数近似）
    pub max_fuel: u64,
    /// Wasm 线性内存最大页数（1 page = 64KB）
    pub max_memory_pages: u32,
    /// 执行超时（毫秒）
    pub timeout_ms: u64,
    /// 是否开启审计日志
    pub audit_log: bool,
}

impl Default for FlyCapabilities {
    fn default() -> Self {
        Self {
            fs_read_allow: HashSet::new(),
            fs_write_allow: HashSet::new(),
            net_allowed_hosts: HashSet::new(),
            env_allowed: HashSet::new(),
            // 默认 1000 万条 wasm 指令 ≈ 几十 ms 到几秒（取决于代码）
            max_fuel: 10_000_000,
            // 默认 64MB 线性内存
            max_memory_pages: 1024,
            // 默认 5 秒超时
            timeout_ms: 5000,
            audit_log: true,
        }
    }
}

/// 沙箱执行结果
#[derive(Debug)]
pub struct SandboxResult {
    pub exit_code: i32,
    pub stdout: String,
    pub stderr: String,
    pub fuel_consumed: u64,
    pub timed_out: bool,
    pub audit_events: Vec<AuditEvent>,
}

/// 审计事件（对接 PyFly `trace` 关键字）
#[derive(Debug, Clone, serde::Serialize)]
pub struct AuditEvent {
    pub timestamp_ms: u64,
    pub event_type: String, // "fs_read" | "fs_write" | "net_fetch" | "env_get" | "print"
    pub resource: String,
    pub allowed: bool,
    pub details: String,
}

/// 创建并配置 Wasmtime Engine（沙箱核心）
///
/// 安全配置要点：
/// 1. consume_fuel(true)      → 限制指令数（对应 cage max_time）
/// 2. epoch_interruption(true) → 超时中断（对应 cage max_time）
/// 3. wasm_multi_value        → 支持多值返回
/// 4. 不启用 wasm_threads     → 减少攻击面
/// 5. 不启用 wasm_simd        → 减少攻击面（除非性能需要）
pub fn create_engine() -> wasmtime::Result<wasmtime::Engine> {
    let mut config = wasmtime::Config::new();
    config.consume_fuel(true);
    config.epoch_interruption(true);
    config.wasm_multi_value(true);
    // 故意禁用以下特性以缩小攻击面：
    config.wasm_threads(false);
    config.wasm_simd(false);
    config.wasm_bulk_memory(true); // 需要，RustPython 用到
    config.wasm_reference_types(true); // 需要，RustPython 用到

    wasmtime::Engine::new(&config)
}

/// 将 PyFly 转译产物（.py）打包为 Wasm 模块
///
/// 实际实现有两种路径：
///   A) RustPython → wasm32-wasip1 → 在 Wasmtime 中解释执行 .py
///   B) CPython (wasi-sdk) → wasm32-wasip1 → 在 Wasmtime 中执行
///
/// 本骨架展示路径 A 的集成方式。
///
/// # 参数
/// - `python_source`: PyFly 转译后的 .py 源码字符串
/// - `engine`: Wasmtime Engine
///
/// # 返回
/// 编译好的 Wasm 模块（实际项目中需先编译 RustPython 为 .wasm）
async fn build_python_wasm_module(
    _python_source: &str,
    _engine: &wasmtime::Engine,
) -> anyhow::Result<Vec<u8>> {
    // ===== 实际项目中这里应：
    // 1. 读取预编译的 rustpython.wasm（构建时生成）
    // 2. 将 python_source 作为数据段注入或作为 WASI 文件传入
    // =====
    //
    // 伪代码：
    // let rustpython_wasm = tokio::fs::read("runtime/rustpython.wasm").await?;
    // let mut module = wasmtime::Module::new(engine, &rustpython_wasm)?;
    // ... 注入 python 源码 ...
    // Ok(compiled_bytes)

    anyhow::bail!(
        "需要实现：将 Fly .py 产物注入 RustPython Wasm 模块。\n\
         参考：https://github.com/RustPython/RustPython（编译目标 wasm32-wasip1）\n\
         或使用 Pyodide（浏览器场景）/ Edge Python（170KB 迷你运行时）"
    )
}

/// 注册 host functions（capability 注入点）
///
/// 每个 host function 对应一个 PyFly 运行时能力，
/// 调用时检查 FlyCapabilities 白名单，拒绝则记录审计事件。
fn register_host_functions(
    linker: &mut wasmtime::Linker<SandboxState>,
) -> anyhow::Result<()> {
    // ---- fly_print: 受控输出（对接 trace） ----
    linker.func_wrap(
        "fly_sandbox",
        "print",
        |mut caller: wasmtime::Caller<'_, SandboxState>,
         ptr: u32,
         len: u32|
         -> anyhow::Result<()> {
            let data = read_memory(&mut caller, ptr, len)?;
            let text = String::from_utf8_lossy(&data);
            println!("[sandbox stdout] {}", text);
            caller.data_mut().audit(AuditEvent {
                timestamp_ms: now_ms(),
                event_type: "print".to_string(),
                resource: text.to_string(),
                allowed: true,
                details: String::new(),
            });
            Ok(())
        },
    )?;

    // ---- fly_fs_read: 白名单文件读取（对接 only + cage） ----
    linker.func_wrap(
        "fly_sandbox",
        "fs_read",
        |mut caller: wasmtime::Caller<'_, SandboxState>,
         path_ptr: u32,
         path_len: u32,
         buf_ptr: u32,
         buf_cap: u32|
         -> anyhow::Result<u32> {
            let path = read_string(&mut caller, path_ptr, path_len);
            let state = caller.data_mut();

            // 检查白名单
            let allowed = state
                .caps
                .fs_read_allow
                .iter()
                .any(|p| PathBuf::from(&path).starts_with(p));

            state.audit(AuditEvent {
                timestamp_ms: now_ms(),
                event_type: "fs_read".to_string(),
                resource: path.clone(),
                allowed,
                details: String::new(),
            });

            if !allowed {
                // 返回 0 表示拒绝（不泄露文件是否存在）
                return Ok(0);
            }

            // 执行实际读取
            match std::fs::read(&path) {
                Ok(data) => {
                    let mem = get_memory(&mut caller)?;
                    let write_len = data.len().min(buf_cap as usize);
                    mem.write(&mut caller, buf_ptr as usize, &data[..write_len])?;
                    Ok(write_len as u32)
                }
                Err(_) => Ok(0),
            }
        },
    )?;

    // ---- fly_env_get: 白名单环境变量（对接 mask） ----
    linker.func_wrap(
        "fly_sandbox",
        "env_get",
        |mut caller: wasmtime::Caller<'_, SandboxState>,
         key_ptr: u32,
         key_len: u32,
         val_buf_ptr: u32,
         val_buf_cap: u32|
         -> anyhow::Result<u32> {
            let key = read_string(&mut caller, key_ptr, key_len);
            let state = caller.data_mut();

            let allowed = state.caps.env_allowed.contains(&key);
            state.audit(AuditEvent {
                timestamp_ms: now_ms(),
                event_type: "env_get".to_string(),
                resource: key.clone(),
                allowed,
                details: String::new(),
            });

            if !allowed {
                return Ok(0);
            }

            if let Ok(val) = std::env::var(&key) {
                let mem = get_memory(&mut caller)?;
                let bytes = val.as_bytes();
                let write_len = bytes.len().min(val_buf_cap as usize);
                mem.write(&mut caller, val_buf_ptr as usize, &bytes[..write_len])?;
                Ok(write_len as u32)
            } else {
                Ok(0)
            }
        },
    )?;

    // ---- fly_net_fetch: 白名单网络请求（对接 only + safe） ----
    linker.func_wrap(
        "fly_sandbox",
        "net_fetch",
        |mut caller: wasmtime::Caller<'_, SandboxState>,
         url_ptr: u32,
         url_len: u32|
         -> anyhow::Result<u32> {
            let url = read_string(&mut caller, url_ptr, url_len);
            let state = caller.data_mut();

            // 提取 host 并检查白名单
            let host = extract_host(&url);
            let allowed = state
                .caps
                .net_allowed_hosts
                .contains(&host);

            state.audit(AuditEvent {
                timestamp_ms: now_ms(),
                event_type: "net_fetch".to_string(),
                resource: url.clone(),
                allowed,
                details: format!("host={}", host),
            });

            // 注意：真实网络请求不应在 host function 内直接做
            // （会阻塞 wasm 执行线程）。应改为异步 + 共享内存。
            // 这里仅展示同步骨架。
            if !allowed {
                return Ok(0);
            }

            // 实际项目中用 reqwest/tokio 异步请求
            // 这里返回占位
            Ok(0)
        },
    )?;

    // ---- fly_log_trace: 审计日志（对接 trace 关键字） ----
    linker.func_wrap(
        "fly_sandbox",
        "log_trace",
        |mut caller: wasmtime::Caller<'_, SandboxState>,
         level_ptr: u32,
         level_len: u32,
         msg_ptr: u32,
         msg_len: u32|
         -> anyhow::Result<()> {
            let level = read_string(&mut caller, level_ptr, level_len);
            let msg = read_string(&mut caller, msg_ptr, msg_len);
            let state = caller.data_mut();

            state.audit(AuditEvent {
                timestamp_ms: now_ms(),
                event_type: format!("log_{}", level),
                resource: msg.clone(),
                allowed: true,
                details: String::new(),
            });

            // 同时输出到宿主 stderr（可重定向到文件/采集器）
            eprintln!("[sandbox {}] {}", level, msg);
            Ok(())
        },
    )?;

    Ok(())
}

/// 沙箱执行入口
pub async fn run_sandboxed(
    python_source: &str,
    caps: FlyCapabilities,
) -> anyhow::Result<SandboxResult> {
    let engine = create_engine()?;
    let module_bytes = build_python_wasm_module(python_source, &engine).await?;
    let module = wasmtime::Module::new(&engine, &module_bytes)?;

    // 启动 epoch 超时监控线程
    let engine_clone = engine.clone();
    let timeout = Duration::from_millis(caps.timeout_ms);
    let epoch_thread = std::thread::spawn(move || {
        std::thread::sleep(timeout);
        // 递增 epoch → 中断 wasm 执行
        engine_clone.increment_epoch();
    });

    let mut store = wasmtime::Store::new(&engine, SandboxState::new(caps.clone()));
    store.set_fuel(caps.max_fuel)?;

    // 设置内存限制
    // （实际项目中通过 wasm module 的 memory import 限制）

    let mut linker = wasmtime::Linker::new(&engine);
    register_host_functions(&mut linker)?;

    // 实例化并运行
    let instance = linker.instantiate(&mut store, &module)?;

    // 假设 RustPython 导出 `run_python` 函数
    let run = instance.get_typed_func::<(u32, u32), i32>(&mut store, "run_python")?;

    // 将 python 源码写入 wasm 线性内存
    let mem = instance.get_memory(&mut store, "memory").unwrap();
    let source_bytes = python_source.as_bytes();
    // ... 写入内存，获取 ptr/len ...
    let _ = mem; // 实际使用需分配 wasm 内存

    let exit_code = run.call(&mut store, (0, 0))?;

    // 回收结果
    let state = store.into_data();
    let fuel_consumed = caps.max_fuel - state.fuel_remaining().unwrap_or(0);

    // 等待 epoch 线程结束
    let _ = epoch_thread.join();

    Ok(SandboxResult {
        exit_code,
        stdout: String::new(),  // 实际从 wasm 内存读取
        stderr: String::new(),
        fuel_consumed,
        timed_out: false, // 实际检查 epoch 是否触发
        audit_events: state.audit_events,
    })
}

// ─────────────────────────────────────────────
// 内部辅助类型与函数
// ─────────────────────────────────────────────

pub struct SandboxState {
    pub caps: FlyCapabilities,
    pub audit_events: Vec<AuditEvent>,
    fuel_used: u64,
}

impl SandboxState {
    fn new(caps: FlyCapabilities) -> Self {
        Self {
            caps,
            audit_events: Vec::new(),
            fuel_used: 0,
        }
    }

    fn audit(&mut self, event: AuditEvent) {
        if self.caps.audit_log {
            self.audit_events.push(event);
        }
    }

    fn fuel_remaining(&self) -> Option<u64> {
        // 实际从 store 读取
        None
    }
}

fn read_memory(
    caller: &mut wasmtime::Caller<'_, SandboxState>,
    ptr: u32,
    len: u32,
) -> anyhow::Result<Vec<u8>> {
    let mem = get_memory(caller)?;
    let mut buf = vec![0u8; len as usize];
    mem.read(caller, ptr as usize, &mut buf)?;
    Ok(buf)
}

fn read_string(
    caller: &mut wasmtime::Caller<'_, SandboxState>,
    ptr: u32,
    len: u32,
) -> String {
    match read_memory(caller, ptr, len) {
        Ok(bytes) => String::from_utf8_lossy(&bytes).to_string(),
        Err(_) => String::new(),
    }
}

fn get_memory(
    caller: &mut wasmtime::Caller<'_, SandboxState>,
) -> anyhow::Result<wasmtime::Memory> {
    caller
        .get_export("memory")
        .and_then(|e| e.into_memory())
        .ok_or_else(|| anyhow::anyhow!("wasm module has no exported memory"))
}

fn extract_host(url: &str) -> String {
    // 简易提取：去掉 scheme，取第一个 /
    url.trim_start_matches("https://")
        .trim_start_matches("http://")
        .split('/')
        .next()
        .unwrap_or("")
        .to_string()
}

fn now_ms() -> u64 {
    std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_millis() as u64)
        .unwrap_or(0)
}

// ─────────────────────────────────────────────
// CLI 入口
// ─────────────────────────────────────────────

#[derive(clap::Parser)]
#[command(author, version, about = "PyFly 跨平台轻量沙箱 (Rust + Wasmtime)")]
struct Cli {
    /// PyFly 转译产物 (.py 文件)
    script: PathBuf,

    /// 允许读取的文件路径（可多次指定）
    #[arg(long = "cap-fs-read", action = clap::ArgAction::Append)]
    fs_read: Vec<PathBuf>,

    /// 允许写入的文件路径（可多次指定）
    #[arg(long = "cap-fs-write", action = clap::ArgAction::Append)]
    fs_write: Vec<PathBuf>,

    /// 允许访问的 HTTP host（可多次指定）
    #[arg(long = "cap-net-host", action = clap::ArgAction::Append)]
    net_hosts: Vec<String>,

    /// 允许读取的环境变量名（可多次指定）
    #[arg(long = "cap-env", action = clap::ArgAction::Append)]
    env_vars: Vec<String>,

    /// Wasm fuel 上限（指令数）
    #[arg(long, default_value_t = 10_000_000)]
    fuel: u64,

    /// 超时（毫秒）
    #[arg(long, default_value_t = 5000)]
    timeout_ms: u64,

    /// 最大内存页数（1 page = 64KB）
    #[arg(long, default_value_t = 1024)]
    max_memory_pages: u32,

    /// 关闭审计日志
    #[arg(long)]
    no_audit: bool,
}

#[tokio::main]
async fn main() -> anyhow::Result<()> {
    // 初始化日志
    tracing_subscriber::fmt()
        .with_target(false)
        .with_level(true)
        .init();

    let cli = Cli::parse();

    // 读取 PyFly 转译产物
    let python_source = tokio::fs::read_to_string(&cli.script).await?;
    tracing::info!("加载脚本: {:?} ({} 字节)", cli.script, python_source.len());

    // 构建能力集
    let caps = FlyCapabilities {
        fs_read_allow: cli.fs_read.into_iter().collect(),
        fs_write_allow: cli.fs_write.into_iter().collect(),
        net_allowed_hosts: cli.net_hosts.into_iter().collect(),
        env_allowed: cli.env_vars.into_iter().collect(),
        max_fuel: cli.fuel,
        max_memory_pages: cli.max_memory_pages,
        timeout_ms: cli.timeout_ms,
        audit_log: !cli.no_audit,
    };

    tracing::info!("能力集: {:?}", caps);

    // 执行沙箱
    match run_sandboxed(&python_source, caps).await {
        Ok(result) => {
            println!("=== 沙箱执行完成 ===");
            println!("退出码: {}", result.exit_code);
            println!("燃料消耗: {}", result.fuel_consumed);
            println!("超时: {}", result.timed_out);
            println!("审计事件数: {}", result.audit_events.len());
            for event in &result.audit_events {
                println!(
                    "  [{}] {} {} allowed={}",
                    event.timestamp_ms, event.event_type, event.resource, event.allowed
                );
            }
        }
        Err(e) => {
            eprintln!("沙箱执行失败: {:?}", e);
            std::process::exit(1);
        }
    }

    Ok(())
}
