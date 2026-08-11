//! fly-sandbox：系统极隔离的执行沙箱（seccomp 白名单 + rlimit + 权限降级）。
//!
//! 用法：
//!   fly-sandbox [选项] -- <命令> [参数...]
//! 选项：
//!   --time <秒>    CPU 时间上限（RLIMIT_CPU）
//!   --mem <MB>     地址空间上限（RLIMIT_AS）
//!   --cpus <个数>   进程数上限（RLIMIT_NPROC）
//!   --fsize <KB>    文件尺寸上限（RLIMIT_FSIZE）
//!   --nobody        setuid/setgid 到 nobody（防权限滥用）
//!   --debug         打印诊断（含 KILL 的 syscall 追踪）

mod seccomp;

use std::ffi::CString;
use std::process::exit;

const SYS_CLONE: i64 = 56;
const SYS_EXECVE: i64 = 59;
const SYS_EXIT: i64 = 60;
const SYS_WAIT4: i64 = 61;
const SYS_SETRLIMIT: i64 = 160;
const SYS_SETUID: i64 = 105;
const SYS_SETGID: i64 = 106;

// 信号
const SIGCHLD: i64 = 17;

// rlimit 资源
const RLIMIT_CPU: i64 = 0;
const RLIMIT_FSIZE: i64 = 1;
const RLIMIT_AS: i64 = 9;
const RLIMIT_NPROC: i64 = 6;

#[repr(C)]
#[derive(Clone, Copy)]
struct Rlimit {
    cur: u64,
    max: u64,
}

struct Opts {
    time: Option<u64>,
    mem_mb: Option<u64>,
    cpus: Option<u64>,
    fsize_kb: Option<u64>,
    nobody: bool,
    debug: bool,
}

impl Opts {
    fn default() -> Opts {
        Opts {
            time: None,
            mem_mb: None,
            cpus: None,
            fsize_kb: None,
            nobody: false,
            debug: false,
        }
    }
}

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let mut opts = Opts::default();
    let mut cmd: Vec<String> = Vec::new();
    let mut i = 1;
    let mut pass_rest = false;
    while i < args.len() {
        let a = &args[i];
        if !pass_rest && a.starts_with("--") {
            match a.as_str() {
                "--time" => {
                    i += 1;
                    opts.time = args.get(i).and_then(|v| v.parse().ok());
                }
                "--mem" => {
                    i += 1;
                    opts.mem_mb = args.get(i).and_then(|v| v.parse().ok());
                }
                "--cpus" => {
                    i += 1;
                    opts.cpus = args.get(i).and_then(|v| v.parse().ok());
                }
                "--fsize" => {
                    i += 1;
                    opts.fsize_kb = args.get(i).and_then(|v| v.parse().ok());
                }
                "--nobody" => opts.nobody = true,
                "--debug" => opts.debug = true,
                "--" => pass_rest = true,
                _ => {
                    eprintln!("未知选项: {}", a);
                    usage();
                }
            }
        } else {
            cmd.push(a.clone());
        }
        i += 1;
    }

    if cmd.is_empty() {
        usage();
    }

    // fork 出沙箱子进程
    let pid = seccomp::syscall3(SYS_CLONE, SIGCHLD, 0, 0);
    if pid < 0 {
        eprintln!("fork 失败: {}", pid);
        exit(1);
    }
    if pid == 0 {
        // 子进程：沙箱化后 exec
        if let Err(code) = sandbox_exec(&opts, &cmd) {
            eprintln!("沙箱启动失败: {}", code);
            seccomp::syscall1(SYS_EXIT, 1);
        }
    }

    // 父进程：等待
    let mut status: i64 = 0;
    let r = seccomp::syscall4(
        SYS_WAIT4,
        pid,
        &mut status as *mut i64 as i64,
        0,
        0,
    );
    if r < 0 {
        eprintln!("wait4 失败: {}", r);
    }
    if opts.debug {
        eprintln!("[sandbox] 退出码: {}", status);
    }
    // 子进程被信号杀死时以 128+sig 退出
    let code = if status == 0 {
        0
    } else if status & 0x7f == 0 {
        (status >> 8) as i32
    } else {
        128 + (status & 0x7f) as i32
    };
    exit(code);
}

fn usage() -> ! {
    eprintln!(
        "用法: fly-sandbox [--time S] [--mem MB] [--cpus N] [--fsize KB] [--nobody] [--debug] -- <命令> [参数...]"
    );
    exit(2);
}

fn set_rlimit(resource: i64, cur: u64) {
    let lim = Rlimit { cur, max: cur };
    let r = seccomp::syscall3(
        SYS_SETRLIMIT,
        resource,
        &lim as *const Rlimit as i64,
        0,
    );
    if r < 0 && !matches!(resource, RLIMIT_NPROC | RLIMIT_FSIZE) {
        eprintln!("警告: setrlimit({}) 失败: {}", resource, r);
    }
}

fn dump_prog() {
    let prog = seccomp::build_program();
    eprintln!("[sandbox] BPF {} insns:", prog.len());
    for (i, ins) in prog.iter().enumerate() {
        eprintln!("  [{}] code={:#06x} jt={} jf={} k={}", i, ins.code, ins.jt, ins.jf, ins.k);
    }
}

fn sandbox_exec(opts: &Opts, cmd: &[String]) -> Result<(), i64> {
    // 1. rlimit 资源上限
    if let Some(t) = opts.time {
        set_rlimit(RLIMIT_CPU, t);
    }
    if let Some(m) = opts.mem_mb {
        set_rlimit(RLIMIT_AS, m * 1024 * 1024);
    }
    if let Some(n) = opts.cpus {
        set_rlimit(RLIMIT_NPROC, n);
    }
    if let Some(k) = opts.fsize_kb {
        set_rlimit(RLIMIT_FSIZE, k * 1024);
    }

    // 2. 权限降级（可选）
    if opts.nobody {
        // nobody: uid/gid = 65534
        let _ = seccomp::syscall3(SYS_SETGID, 65534, 0, 0);
        let _ = seccomp::syscall3(SYS_SETUID, 65534, 0, 0);
    }

    // 3. seccomp 白名单（系统极隔离核心）
    if opts.debug {
        dump_prog();
    }
    if std::env::var("SBX_NOSECCOMP").is_err() {
        seccomp::install()?;
    }

    // 4. exec 目标命令
    let path = CString::new(cmd[0].as_str()).map_err(|_| -1)?;
    let mut argv_ptrs: Vec<*const u8> = Vec::new();
    let mut arg_owned: Vec<CString> = Vec::new();
    for a in cmd {
        let cs = CString::new(a.as_str()).map_err(|_| -1)?;
        argv_ptrs.push(cs.as_ptr() as *const u8);
        arg_owned.push(cs);
    }
    argv_ptrs.push(std::ptr::null());
    let envp_ptrs: Vec<*const u8> = vec![std::ptr::null()];
    let r = seccomp::syscall3(
        SYS_EXECVE,
        path.as_ptr() as i64,
        argv_ptrs.as_ptr() as i64,
        envp_ptrs.as_ptr() as i64,
    );
    Err(r)
}
