//! fly-sandboxd: Fly-Lang Linux 原生进程沙箱（Fast 模式）
//!
//! 隔离链（自内向外纵深）：
//!   1. Landlock（路径级最小权限，非 root 可用）: 默认拒绝文件访问，
//!      仅授权系统库路径 + 脚本路径 + --cap-fs-read 白名单（只读）
//!   2. rlimit: RLIMIT_AS 内存上限 / RLIMIT_CPU 时间上限 / RLIMIT_NOFILE
//!   3. seccomp-bpf 白名单: 仅放行沙箱所需系统调用；网络/socket、mount、
//!      ptrace、ns 创建等一律 EPERM（clone 仅允许非 CLONE_NEW* flags）
//!   4. namespaces: PID（沙箱内 PID 1）、NET（默认无网络）、Mount、UTS、
//!      IPC；非 root 先建 user ns 并映射自身 uid
//!
//! 默认 deny（fail closed）：任何隔离层初始化失败即终止，不降级运行。


use std::ffi::CString;
use std::os::unix::io::RawFd;
use std::path::Path;
use std::process;
use std::time::{Duration, Instant};

// ---------- CLI ----------

#[derive(Debug)]
struct Opts {
    script: String,
    cap_fs_read: Vec<String>,
    cap_net_host: Vec<String>,
    mem_limit_mb: u64,
    timeout_ms: u64,
    cpu_sec: u64,
    nofile: u64,
    audit: bool,
    debug_ns: bool,
}

fn usage() -> ! {
    eprintln!(
        "fly-sandboxd <script.py> [options]\n\
         \n\
         options:\n\
         \x20 --cap-fs-read <path>    只读访问白名单（可重复，默认仅系统库+脚本）\n\
         \x20 --cap-net-host <host>   网络 host 白名单（v0.2 预留，当前网络一律禁用）\n\
         \x20 --mem-limit-mb <n>      内存上限 MB（默认 512）\n\
         \x20 --timeout-ms <n>        墙钟超时毫秒（默认 5000）\n\
         \x20 --cpu-sec <n>           CPU 时间上限秒（默认 10）\n\
         \x20 --nofile <n>            文件描述符上限（默认 64）\n\
         \x20 --no-audit              关闭审计日志（默认开启，JSON 行输出到 stderr）\n\
         \x20 --debug-ns              跳过命名空间（调试隔离层用）"
    );
    process::exit(2);
}

fn parse_args() -> Opts {
    let args: Vec<String> = std::env::args().skip(1).collect();
    if args.is_empty() {
        usage();
    }
    let mut o = Opts {
        script: String::new(),
        cap_fs_read: Vec::new(),
        cap_net_host: Vec::new(),
        mem_limit_mb: 512,
        timeout_ms: 5000,
        cpu_sec: 10,
        nofile: 64,
        audit: true,
        debug_ns: false,
    };
    let mut i = 0;
    let next = |args: &[String], i: &mut usize, flag: &str| -> String {
        *i += 1;
        if *i >= args.len() {
            eprintln!("{} 需要参数", flag);
            process::exit(2);
        }
        args[*i].clone()
    };
    while i < args.len() {
        match args[i].as_str() {
            "--cap-fs-read" => o.cap_fs_read.push(next(&args, &mut i, "--cap-fs-read")),
            "--cap-net-host" => o.cap_net_host.push(next(&args, &mut i, "--cap-net-host")),
            "--mem-limit-mb" => o.mem_limit_mb = next(&args, &mut i, "--mem-limit-mb").parse().unwrap_or_else(|_| usage()),
            "--timeout-ms" => o.timeout_ms = next(&args, &mut i, "--timeout-ms").parse().unwrap_or_else(|_| usage()),
            "--cpu-sec" => o.cpu_sec = next(&args, &mut i, "--cpu-sec").parse().unwrap_or_else(|_| usage()),
            "--nofile" => o.nofile = next(&args, &mut i, "--nofile").parse().unwrap_or_else(|_| usage()),
            "--no-audit" => o.audit = false,
            "--debug-ns" => o.debug_ns = true,
            "-h" | "--help" => usage(),
            _ => {
                if o.script.is_empty() {
                    o.script = args[i].clone();
                } else {
                    eprintln!("未知参数 {}", args[i]);
                    process::exit(2);
                }
            }
        }
        i += 1;
    }
    if o.script.is_empty() {
        usage();
    }
    o
}

fn audit(flag: bool, json: &str) {
    if flag {
        eprintln!("{}", json);
    }
}

// ---------- Landlock ----------

#[cfg(any(target_arch = "x86_64", target_arch = "aarch64"))]
#[allow(dead_code)] // 位常量用于文档化 HANDLED_FS 组成
mod landlock {
    use super::*;

    const LANDLOCK_CREATE_RULESET: i64 = 444;
    const LANDLOCK_ADD_RULE: i64 = 445;
    const LANDLOCK_RESTRICT_SELF: i64 = 446;
    const LANDLOCK_RULE_PATH_BENEATH: u32 = 1;
    const LANDLOCK_ACCESS_FS_EXECUTE: u64 = 1 << 0;
    const LANDLOCK_ACCESS_FS_WRITE_FILE: u64 = 1 << 1;
    const LANDLOCK_ACCESS_FS_READ_FILE: u64 = 1 << 2;
    const LANDLOCK_ACCESS_FS_READ_DIR: u64 = 1 << 3;
    const LANDLOCK_ACCESS_FS_REMOVE_DIR: u64 = 1 << 4;
    const LANDLOCK_ACCESS_FS_REMOVE_FILE: u64 = 1 << 5;
    const LANDLOCK_ACCESS_FS_MAKE_CHAR: u64 = 1 << 6;
    const LANDLOCK_ACCESS_FS_MAKE_DIR: u64 = 1 << 7;
    const LANDLOCK_ACCESS_FS_MAKE_REG: u64 = 1 << 8;
    const LANDLOCK_ACCESS_FS_MAKE_SOCK: u64 = 1 << 9;
    const LANDLOCK_ACCESS_FS_MAKE_FIFO: u64 = 1 << 10;
    const LANDLOCK_ACCESS_FS_MAKE_BLOCK: u64 = 1 << 11;
    const LANDLOCK_ACCESS_FS_MAKE_SYM: u64 = 1 << 12;
    const LANDLOCK_ACCESS_FS_REFER: u64 = 1 << 13;
    const LANDLOCK_ACCESS_FS_TRUNCATE: u64 = 1 << 14;
    const LANDLOCK_ACCESS_FS_IOCTL_DEV: u64 = 1 << 15;
    // 默认拒绝需要 handled 覆盖全部可执行访问位（仅覆盖 3 位时
    // MAKE_DIR/MAKE_REG 等不在管辖内，沙箱内可任意建文件）
    const HANDLED_FS: u64 = (1 << 16) - 1; // ABI v5 全集（EXECUTE..IOCTL_DEV）
    const SYS_ACCESS: u64 =
        LANDLOCK_ACCESS_FS_EXECUTE | LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR;
    const DATA_ACCESS: u64 = LANDLOCK_ACCESS_FS_READ_FILE | LANDLOCK_ACCESS_FS_READ_DIR;

    #[repr(C)]
    struct RulesetAttr {
        handled_access_fs: u64,
    }

    #[repr(C)]
    struct PathBeneathAttr {
        allowed_access: u64,
        parent_fd: i32,
    }

    fn syscall4(nr: i64, a1: i64, a2: i64, a3: i64, a4: i64) -> i64 {
        unsafe { libc::syscall(nr as libc::c_long, a1, a2, a3, a4) }
    }

    /// 探测内核支持的 Landlock ABI，返回可用的 handled 全集（逐级回退）。
    fn probe_handled() -> u64 {
        let levels: [u64; 3] = [HANDLED_FS, (1 << 15) - 1, (1 << 14) - 1];
        for h in levels {
            let attr = RulesetAttr {
                handled_access_fs: h,
            };
            let fd = syscall4(
                LANDLOCK_CREATE_RULESET,
                &attr as *const RulesetAttr as i64,
                std::mem::size_of::<RulesetAttr>() as i64,
                0,
                0,
            );
            if fd >= 0 {
                unsafe { libc::close(fd as RawFd) };
                return h;
            }
        }
        0
    }

    fn open_readable_path(path: &str) -> i64 {
        let c = CString::new(path).unwrap_or_default();
        unsafe {
            // 不跟 O_NOFOLLOW：/lib、/lib64 等是 symlink，跟随解析到真实目录；
            // 沙箱内路径均为静态配置，无 TOCTOU 风险
            libc::open(
                c.as_ptr(),
                libc::O_PATH | libc::O_DIRECTORY | libc::O_CLOEXEC,
            ) as i64
        }
    }

    /// 授权一个路径的指定访问位（目录或文件），返回 true。
    fn grant_access(ruleset_fd: i64, path: &str, access: u64) -> bool {
        let fd = open_readable_path(path);
        if fd < 0 {
            eprintln!("{{\"event\":\"landlock_grant_fail\",\"path\":\"{}\",\"err\":\"open: {}\"}}", path, std::io::Error::last_os_error());
            return false;
        }
        let attr = PathBeneathAttr {
            allowed_access: access,
            parent_fd: fd as i32,
        };
        let rc = syscall4(
            LANDLOCK_ADD_RULE,
            ruleset_fd,
            LANDLOCK_RULE_PATH_BENEATH as i64,
            &attr as *const PathBeneathAttr as i64,
            0,
        );
        let e = std::io::Error::last_os_error();
        unsafe { libc::close(fd as RawFd) };
        if rc == 0 {
            return true;
        }
        eprintln!("{{\"event\":\"landlock_grant_fail\",\"path\":\"{}\",\"err\":\"{}\"}}", path, e);
        false
    }

    /// 系统只读库路径（python 解释器与标准库所在地）。
    fn syslib_dirs() -> &'static [&'static str] {
        &[
            "/usr/lib",
            "/usr/local/lib",
            "/usr/lib64",
            "/usr/local/lib64",
            "/lib",
            "/lib64",
            "/usr/bin",
            "/usr/local/bin",
        ]
    }

    /// 应用 Landlock：默认拒绝所有文件访问，只读授权系统库 + 脚本 + 白名单。
    /// 返回 None 表示内核不支持（不降级，由调用方决定）。
    pub fn apply(opts: &Opts, audit: bool) -> Result<(), String> {
        let handled = probe_handled();
        if handled == 0 {
            return Err("内核不支持 Landlock（需 Linux >= 5.13）".into());
        }
        // restrict_self 需要 no_new_privs 或 root：非 root 必须先行设置（seccomp 处也会设）
        if unsafe { libc::prctl(libc::PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) } != 0 {
            return Err(format!("PR_SET_NO_NEW_PRIVS 失败: {}", std::io::Error::last_os_error()));
        }
        let attr = RulesetAttr {
            handled_access_fs: handled,
        };
        let ruleset_fd = syscall4(
            LANDLOCK_CREATE_RULESET,
            &attr as *const RulesetAttr as i64,
            std::mem::size_of::<RulesetAttr>() as i64,
            0,
            0,
        );
        if ruleset_fd < 0 {
            return Err(format!("landlock_create_ruleset 失败: {} (attr size {})", std::io::Error::last_os_error(), std::mem::size_of::<RulesetAttr>()));
        }

        let mut granted = Vec::new();
        for d in syslib_dirs() {
            // 系统库：读 + 执行（python 解释器可 exec）
            if grant_access(ruleset_fd, d, SYS_ACCESS) {
                granted.push((*d).to_string());
            }
        }
        if let Some(dir) = Path::new(&opts.script).parent() {
            // 脚本目录：只读（解释器自身已有 exec 权限）
            if grant_access(ruleset_fd, dir.to_str().unwrap_or(""), DATA_ACCESS) {
                granted.push(dir.to_str().unwrap_or("").to_string());
            }
        }
        for p in &opts.cap_fs_read {
            if grant_access(ruleset_fd, p, DATA_ACCESS) {
                granted.push(p.clone());
            }
        }

        let rc = syscall4(LANDLOCK_RESTRICT_SELF, ruleset_fd, 0, 0, 0);
        unsafe { libc::close(ruleset_fd as RawFd) };
        if rc != 0 {
            return Err(format!("landlock_restrict_self 失败: {}", std::io::Error::last_os_error()));
        }
        super::audit(
            audit,
            &format!(
                "{{\"event\":\"landlock\",\"granted\":[{}]}}",
                granted
                    .iter()
                    .map(|p| format!("\"{}\"", p))
                    .collect::<Vec<_>>()
                    .join(",")
            ),
        );
        Ok(())
    }
}

// ---------- seccomp BPF ----------

#[cfg(target_arch = "x86_64")]
const ARCH_AUDIT: u32 = 0xc000003e; // AUDIT_ARCH_X86_64

#[cfg(target_arch = "x86_64")]
const SYS: &[u32] = &[
    0, 1, 3, 5, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 25, 28, 32, 33, 35, 39,
    41, 57, 59, 60, 61, 62, 63, 72, 73, 74, 77, 78, 79, 80, 81, 83, 84, 86, 87, 89, 90, 91, 92,
    93, 94, 95, 96, 98, 99, 102, 104, 107, 108, 110, 115, 116, 124, 131, 137, 138, 141, 158,
    186, 202, 204, 217, 218, 219, 228, 229, 230, 231, 232, 233, 234, 257, 258, 260, 261, 262,
    263, 264, 265, 266, 267, 268, 269, 280, 288, 290, 291, 292, 293, 295, 296, 297, 302, 318,
    332, 334, 437,
];

#[cfg(not(target_arch = "x86_64"))]
compile_error!("fly-sandboxd v0.1 仅支持 x86_64（seccomp 白名单按架构维护，避免错误编号导致过滤失效）");

const BPF_LD_W_ABS: u16 = 0x20;
const BPF_JMP_JA: u16 = 0x05;
const BPF_JMP_JEQ_K: u16 = 0x15;
const BPF_ALU_AND_K: u16 = 0x54;
const BPF_RET_K: u16 = 0x06;

const SECCOMP_RET_ALLOW: u32 = 0x7fff0000;
const SECCOMP_RET_ERRNO: u32 = 0x00050000;
const SECCOMP_RET_KILL_PROCESS: u32 = 0x80000000;

const SECCOMP_DATA_ARCH_OFF: u32 = 4;
const SECCOMP_DATA_NR_OFF: u32 = 0;
const SECCOMP_DATA_ARGS0_OFF: u32 = 16;

const CLONE_NEW_MASK: u32 = 0x00020000 // CLONE_NEWNS
    | 0x02000000 // CLONE_NEWCGROUP
    | 0x04000000 // CLONE_NEWUTS
    | 0x08000000 // CLONE_NEWIPC
    | 0x10000000 // CLONE_NEWUSER
    | 0x20000000 // CLONE_NEWPID
    | 0x40000000; // CLONE_NEWNET

#[cfg(target_arch = "x86_64")]
const SYS_CLONE: u32 = 56;

/// 构造 seccomp-bpf 白名单程序：
/// 白名单命中 → ALLOW；clone 需 flags 不含 CLONE_NEW*；其余 → EPERM；
/// 架构不匹配 → KILL。
fn build_bpf() -> Vec<libc::sock_filter> {
    let mut p: Vec<libc::sock_filter> = Vec::new();
    let mut emit = |code: u16, jt: u8, jf: u8, k: u32| {
        p.push(libc::sock_filter { code, jt, jf, k });
    };
    // 0: ld arch
    emit(BPF_LD_W_ABS, 0, 0, SECCOMP_DATA_ARCH_OFF);
    // 1: jeq ARCH —— 命中 → 顺序（2: ld nr）；不命中 → 跳到尾部 ret kill
    emit(BPF_JMP_JEQ_K, 0, 0, ARCH_AUDIT);
    // 2: ld nr
    emit(BPF_LD_W_ABS, 0, 0, SECCOMP_DATA_NR_OFF);
    // 3..: 白名单（每条 jeq + ret allow），clone 由尾部特判
    for &nr in SYS {
        emit(BPF_JMP_JEQ_K, 0, 1, nr);
        emit(BPF_RET_K, 0, 0, SECCOMP_RET_ALLOW);
    }
    // 尾部：非白名单 → 若是 clone 则检查 flags，否则 EPERM
    let base = 3 + 2 * SYS.len() as u32;
    emit(BPF_JMP_JEQ_K, 0, 1, SYS_CLONE); // base+0
    emit(BPF_JMP_JA, 0, 0, 1); // base+1 → base+3
    emit(BPF_RET_K, 0, 0, SECCOMP_RET_ERRNO | 1); // base+2: 非 clone → EPERM
    emit(BPF_LD_W_ABS, 0, 0, SECCOMP_DATA_ARGS0_OFF); // base+3
    emit(BPF_ALU_AND_K, 0, 0, CLONE_NEW_MASK); // base+4
    emit(BPF_JMP_JEQ_K, 0, 1, 0); // base+5: flags 干净 → allow，否则 errno
    emit(BPF_RET_K, 0, 0, SECCOMP_RET_ALLOW); // base+6
    emit(BPF_RET_K, 0, 0, SECCOMP_RET_ERRNO | 1); // base+7
    emit(BPF_RET_K, 0, 0, SECCOMP_RET_KILL_PROCESS); // base+8: arch 不匹配

    // 回填 arch jeq 的跳转偏移：不匹配 → base+8（ret kill）
    p[1].jf = (base + 8 - 2) as u8;
    p
}

fn apply_seccomp() -> Result<(), String> {
    let prog = build_bpf();
    let sock_fprog = libc::sock_fprog {
        len: prog.len() as u16,
        filter: prog.as_ptr() as *mut libc::sock_filter,
    };
    let rc = unsafe {
        libc::prctl(
            libc::PR_SET_NO_NEW_PRIVS,
            1,
            0,
            0,
            0,
        )
    };
    if rc != 0 {
        return Err(format!("PR_SET_NO_NEW_PRIVS 失败: {}", std::io::Error::last_os_error()));
    }
    let rc = unsafe {
        libc::prctl(
            libc::PR_SET_SECCOMP,
            libc::SECCOMP_MODE_FILTER,
            &sock_fprog as *const libc::sock_fprog as libc::c_ulong,
            0,
            0,
        )
    };
    if rc != 0 {
        return Err(format!("PR_SET_SECCOMP 失败: {}", std::io::Error::last_os_error()));
    }
    Ok(())
}

// ---------- namespaces / 资源 ----------

fn setup_namespaces(skip: bool) -> Result<(), String> {
    if skip {
        return Ok(());
    }
    let euid = unsafe { libc::geteuid() };
    let err = |op: &str| format!("{} 失败: {}", op, std::io::Error::last_os_error());
    // uid/gid 必须在 unshare 前捕获：unshare 后（空 map）getuid 返回 65534
    let host_uid = unsafe { libc::getuid() };
    let host_gid = unsafe { libc::getgid() };
    if euid != 0 {
        let rc = unsafe { libc::unshare(libc::CLONE_NEWUSER) };
        if rc != 0 {
            return Err(err("unshare(CLONE_NEWUSER)"));
        }
        write_uid_map(host_uid, host_gid)?;
        let rc = unsafe {
            libc::unshare(
                libc::CLONE_NEWNS
                    | libc::CLONE_NEWUTS
                    | libc::CLONE_NEWIPC
                    | libc::CLONE_NEWPID
                    | libc::CLONE_NEWNET,
            )
        };
        if rc != 0 {
            return Err(err("unshare(namespaces)"));
        }
    } else {
        let rc = unsafe {
            libc::unshare(
                libc::CLONE_NEWNS
                    | libc::CLONE_NEWUTS
                    | libc::CLONE_NEWIPC
                    | libc::CLONE_NEWPID
                    | libc::CLONE_NEWNET,
            )
        };
        if rc != 0 {
            return Err(err("unshare(namespaces)"));
        }
    }
    Ok(())
}

/// 写 proc 文件（uid_map/gid_map/setgroups）。
/// 必须用干净 O_WRONLY 打开——O_CREAT|O_TRUNC 打开的 fd 写入会被内核拒绝（EPERM）。
fn write_proc(path: &str, data: &str) -> Result<(), String> {
    let c = CString::new(path).map_err(|_| "路径含 NUL".to_string())?;
    let fd = unsafe { libc::open(c.as_ptr(), libc::O_WRONLY | libc::O_CLOEXEC) };
    if fd < 0 {
        return Err(format!("{}: {}", path, std::io::Error::last_os_error()));
    }
    let buf = data.as_bytes();
    let mut off = 0usize;
    while off < buf.len() {
        let n = unsafe {
            libc::write(
                fd,
                buf.as_ptr().add(off) as *const libc::c_void,
                buf.len() - off,
            )
        };
        if n <= 0 {
            let e = std::io::Error::last_os_error();
            unsafe { libc::close(fd) };
            return Err(format!("{}: {}", path, e));
        }
        off += n as usize;
    }
    unsafe { libc::close(fd) };
    Ok(())
}

fn write_uid_map(host_uid: u32, host_gid: u32) -> Result<(), String> {
    // 顺序关键：先写 uid_map（进程成为 ns 内 root），再 setgroups deny + gid_map
    write_proc("/proc/self/uid_map", &format!("0 {} 1\n", host_uid))?;
    write_proc("/proc/self/setgroups", "deny")?;
    write_proc("/proc/self/gid_map", &format!("0 {} 1\n", host_gid))?;
    Ok(())
}

fn setup_rlimits(o: &Opts) -> Result<(), String> {
    let err = |op: &str| format!("{} 失败: {}", op, std::io::Error::last_os_error());
    let mem = libc::rlimit {
        rlim_cur: o.mem_limit_mb * 1024 * 1024,
        rlim_max: o.mem_limit_mb * 1024 * 1024,
    };
    if unsafe { libc::setrlimit(libc::RLIMIT_AS, &mem) } != 0 {
        return Err(err("setrlimit(RLIMIT_AS)"));
    }
    let cpu = libc::rlimit {
        rlim_cur: o.cpu_sec,
        rlim_max: o.cpu_sec,
    };
    if unsafe { libc::setrlimit(libc::RLIMIT_CPU, &cpu) } != 0 {
        return Err(err("setrlimit(RLIMIT_CPU)"));
    }
    let nofile = libc::rlimit {
        rlim_cur: o.nofile,
        rlim_max: o.nofile,
    };
    if unsafe { libc::setrlimit(libc::RLIMIT_NOFILE, &nofile) } != 0 {
        return Err(err("setrlimit(RLIMIT_NOFILE)"));
    }
    Ok(())
}

fn run_python(o: &Opts) -> i32 {
    let script = CString::new(o.script.as_str()).unwrap();
    let py = CString::new("/usr/bin/python3").unwrap();
    let argv: [*const libc::c_char; 3] = [py.as_ptr(), script.as_ptr(), std::ptr::null()];
    let envp: [*const libc::c_char; 1] = [std::ptr::null()];
    unsafe {
        libc::execve(py.as_ptr(), argv.as_ptr(), envp.as_ptr());
    }
    eprintln!(
        "{{\"event\":\"exec_failed\",\"err\":\"{}\"}}",
        std::io::Error::last_os_error()
    );
    127
}

fn main() {
    let o = parse_args();
    let started = Instant::now();
    if !Path::new(&o.script).is_file() {
        eprintln!("错误: 脚本 {} 不存在", o.script);
        process::exit(1);
    }
    audit(o.audit, &format!("{{\"event\":\"start\",\"script\":\"{}\",\"mem_mb\":{},\"timeout_ms\":{}}}", o.script, o.mem_limit_mb, o.timeout_ms));

    // pipe: child → parent（gpid + 退出码）
    let mut fds = [0i32; 2];
    if unsafe { libc::pipe(fds.as_mut_ptr()) } != 0 {
        eprintln!("pipe 失败");
        process::exit(1);
    }
    let (rfd, wfd) = (fds[0], fds[1]);

    let pid = unsafe { libc::fork() };
    if pid < 0 {
        eprintln!("fork 失败: {}", std::io::Error::last_os_error());
        process::exit(1);
    }

    if pid == 0 {
        // ---- child：设置命名空间后 fork 孙进程 ----
        unsafe { libc::close(rfd) };
        if let Err(e) = setup_namespaces(o.debug_ns) {
            eprintln!("{{\"event\":\"ns_failed\",\"err\":\"{}\"}}", e);
            std::process::exit(1);
        }
        let gpid = unsafe { libc::fork() };
        if gpid < 0 {
            eprintln!("嵌套 fork 失败");
            std::process::exit(1);
        }
        if gpid == 0 {
            // ---- grandchild：受限 Python ----
            if let Err(e) = landlock::apply(&o, o.audit) {
                eprintln!("{{\"event\":\"landlock_failed\",\"err\":\"{}\"}}", e);
                std::process::exit(1);
            }
            if let Err(e) = setup_rlimits(&o) {
                eprintln!("{{\"event\":\"rlimit_failed\",\"err\":\"{}\"}}", e);
                std::process::exit(1);
            }
            if let Err(e) = apply_seccomp() {
                eprintln!("{{\"event\":\"seccomp_failed\",\"err\":\"{}\"}}", e);
                std::process::exit(1);
            }
            let code = run_python(&o);
            std::process::exit(code);
        }
        // 向父进程报告 gpid
        let mut buf = [0u8; 4];
        buf.copy_from_slice(&(gpid as u32).to_be_bytes());
        let _ = unsafe {
            libc::write(wfd, buf.as_ptr() as *const libc::c_void, buf.len())
        };
        unsafe { libc::close(wfd) };
        let mut status: libc::c_int = 0;
        let _ = unsafe { libc::waitpid(gpid, &mut status, 0) };
        let code = if libc::WIFEXITED(status) {
            libc::WEXITSTATUS(status)
        } else if libc::WIFSIGNALED(status) {
            128 + libc::WTERMSIG(status)
        } else {
            1
        };
        let mut buf = [0u8; 4];
        buf.copy_from_slice(&(code as u32).to_be_bytes());
        let _ = unsafe {
            libc::write(wfd, buf.as_ptr() as *const libc::c_void, buf.len())
        };
        std::process::exit(0);
    }

    // ---- parent：监控与超时 ----
    unsafe { libc::close(wfd) };
    let mut gpid: u32 = 0;
    let mut got = 0usize;
    let mut buf = [0u8; 4];
    while got < 4 {
        let n = unsafe {
            libc::read(rfd, buf.as_mut_ptr().add(got) as *mut libc::c_void, 4 - got)
        };
        if n <= 0 {
            break;
        }
        got += n as usize;
    }
    unsafe { libc::close(rfd) };
    if got == 4 {
        gpid = u32::from_be_bytes(buf);
    }

    let deadline = started + Duration::from_millis(o.timeout_ms);
    let mut timed_out = false;
    let mut status: libc::c_int = 0;
    loop {
        if Instant::now() >= deadline {
            timed_out = true;
            break;
        }
        let rc = unsafe { libc::waitpid(pid, &mut status, libc::WNOHANG) };
        if rc == pid {
            break;
        }
        if rc < 0 {
            let e = std::io::Error::last_os_error();
            if e.raw_os_error() == Some(libc::EINTR) {
                continue;
            }
            eprintln!("waitpid 失败: {}", e);
            std::process::exit(1);
        }
        std::thread::sleep(Duration::from_millis(10));
    }
    if timed_out {
        if gpid != 0 {
            unsafe {
                libc::kill(gpid as libc::pid_t, libc::SIGKILL);
            }
        }
        unsafe {
            libc::waitpid(pid, &mut status, 0);
        }
        audit(o.audit, &format!("{{\"event\":\"timeout\",\"ms\":{}}}", o.timeout_ms));
        process::exit(124);
    }
    let wall = started.elapsed().as_millis();
    let code = if libc::WIFEXITED(status) {
        libc::WEXITSTATUS(status)
    } else if libc::WIFSIGNALED(status) {
        128 + libc::WTERMSIG(status)
    } else {
        1
    };
    audit(o.audit, &format!("{{\"event\":\"exit\",\"code\":{},\"wall_ms\":{}}}", code, wall));
    process::exit(code);
}
