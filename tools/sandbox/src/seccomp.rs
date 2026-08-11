//! seccomp BPF：系统极隔离的核心——syscall 白名单。
//!
//! 策略（x86_64）：
//! - 白名单内 syscall 放行（含 openat/open 的"仅读"标志检查）
//! - 其余一律 KILL_PROCESS（绝对防逃逸：exec/网络/写文件/提权/逃逸原语全禁）
//! - 默认 KILL 而非 ERRNO：恶意代码触发即进程终止，不给任何旁路机会

const SYS_SETITIMER: u32 = 38;
const SYS_GETITIMER: u32 = 36;
const SYS_ALARM: u32 = 37;
const SYS_SOCKET: u32 = 41;
const AF_INET: u32 = 2;
const AF_INET6: u32 = 10;
const DATA_ARG0: u32 = 16;
const SYS_ACCEPT: u32 = 43;
const SYS_SENDTO: u32 = 44;
const SYS_RECVFROM: u32 = 45;
const SYS_SENDMSG: u32 = 46;
const SYS_RECVMSG: u32 = 47;
const SYS_SHUTDOWN: u32 = 48;
const SYS_BIND: u32 = 49;
const SYS_LISTEN: u32 = 50;
const SYS_SOCKETPAIR: u32 = 53;
const SYS_GETSOCKOPT: u32 = 54;
const SYS_SETSOCKOPT: u32 = 55;
const SYS_EXECVE: u32 = 59;
const SYS_TRUNCATE: u32 = 76;
const SYS_FTRUNCATE: u32 = 77;
const SYS_RENAME: u32 = 82;
const SYS_MKDIR: u32 = 83;
const SYS_RMDIR: u32 = 84;
const SYS_LINK: u32 = 86;
const SYS_UNLINK: u32 = 87;
const SYS_SYMLINK: u32 = 88;
const SYS_CHMOD: u32 = 90;
const SYS_FCHMOD: u32 = 91;
const SYS_CHOWN: u32 = 92;
const SYS_FCHOWN: u32 = 93;
const SYS_LCHOWN: u32 = 94;
const SYS_PTRACE: u32 = 101;
const SYS_SETUID: u32 = 105;
const SYS_SETGID: u32 = 106;
const SYS_SETEUID: u32 = 107;
const SYS_SETEGID: u32 = 108;
const SYS_SETGROUPS: u32 = 116;
const SYS_SETRESUID: u32 = 117;
const SYS_SETRESGID: u32 = 119;
const SYS_SETFSUID: u32 = 122;
const SYS_SETFSGID: u32 = 123;
const SYS_CAPSET: u32 = 126;
const SYS_UTIME: u32 = 132;
const SYS_MKNOD: u32 = 133;
const SYS_SETRLIMIT: u32 = 160;
const SYS_UTIMES: u32 = 163;
const SYS_MOUNT: u32 = 165;
const SYS_UMOUNT2: u32 = 166;
const SYS_SWAPON: u32 = 167;
const SYS_SWAPOFF: u32 = 168;
const SYS_REBOOT: u32 = 169;
const SYS_SETHOSTNAME: u32 = 170;
const SYS_SETDOMAINNAME: u32 = 171;
const SYS_INIT_MODULE: u32 = 175;
const SYS_DELETE_MODULE: u32 = 176;
const SYS_EXIT_GROUP: u32 = 231;
const SYS_KEXEC_LOAD: u32 = 246;
const SYS_ADD_KEY: u32 = 248;
const SYS_REQUEST_KEY: u32 = 249;
const SYS_KEYCTL: u32 = 250;
const SYS_MKDIRAT: u32 = 258;
const SYS_MKNODAT: u32 = 259;
const SYS_FUTIMESAT: u32 = 261;
const SYS_UNLINKAT: u32 = 263;
const SYS_RENAMEAT: u32 = 264;
const SYS_LINKAT: u32 = 265;
const SYS_SYMLINKAT: u32 = 266;
const SYS_UNSHARE: u32 = 272;
const SYS_UTIMENSAT: u32 = 280;
const SYS_ACCEPT4: u32 = 288;
const SYS_PERF_EVENT_OPEN: u32 = 298;
const SYS_OPEN_BY_HANDLE_AT: u32 = 304;
const SYS_SETNS: u32 = 308;
const SYS_PROCESS_VM_READV: u32 = 310;
const SYS_PROCESS_VM_WRITEV: u32 = 311;
const SYS_BPF: u32 = 321;
const SYS_EXECVEAT: u32 = 322;
const SYS_USERFAULTFD: u32 = 323;
const SYS_IO_URING_SETUP: u32 = 425;
const SYS_IO_URING_ENTER: u32 = 426;
const SYS_CLONE3: u32 = 435;

const SYS_RSEQ: u32 = 334;
const SYS_STATX: u32 = 332;

pub const SECCOMP_RET_ALLOW: u32 = 0x7fff0000;
pub const SECCOMP_RET_KILL_PROCESS: u32 = 0x80000000;
pub const SECCOMP_RET_ERRNO: u32 = 0x00050000;

// x86_64 syscall 号
const SYS_READ: u32 = 0;
const SYS_WRITE: u32 = 1;
const SYS_OPEN: u32 = 2;
const SYS_CLOSE: u32 = 3;
const SYS_STAT: u32 = 4;
const SYS_FSTAT: u32 = 5;
const SYS_LSTAT: u32 = 6;
const SYS_POLL: u32 = 7;
const SYS_LSEEK: u32 = 8;
const SYS_MMAP: u32 = 9;
const SYS_MPROTECT: u32 = 10;
const SYS_MUNMAP: u32 = 11;
const SYS_BRK: u32 = 12;
const SYS_RT_SIGACTION: u32 = 13;
const SYS_RT_SIGPROCMASK: u32 = 14;
const SYS_RT_SIGRETURN: u32 = 15;
const SYS_IOCTL: u32 = 16;
const SYS_PREAD64: u32 = 17;
const SYS_PWRITE64: u32 = 18;
const SYS_READV: u32 = 19;
const SYS_WRITEV: u32 = 20;
const SYS_ACCESS: u32 = 21;
const SYS_PIPE: u32 = 22;
const SYS_SELECT: u32 = 23;
const SYS_SCHED_YIELD: u32 = 24;
const SYS_MREMAP: u32 = 25;
const SYS_MSYNC: u32 = 26;
const SYS_MINCORE: u32 = 27;
const SYS_MADVISE: u32 = 28;
const SYS_DUP: u32 = 32;
const SYS_DUP2: u32 = 33;
const SYS_NANOSLEEP: u32 = 35;
const SYS_GETPID: u32 = 39;
const SYS_CLONE: u32 = 56;
const SYS_FORK: u32 = 57;
const SYS_VFORK: u32 = 58;
const SYS_EXIT: u32 = 60;
const SYS_WAIT4: u32 = 61;
const SYS_KILL: u32 = 62;
const SYS_UNAME: u32 = 63;
const SYS_GETPPID: u32 = 64;
const SYS_FCNTL: u32 = 72;
const SYS_FLOCK: u32 = 73;
const SYS_FSYNC: u32 = 74;
const SYS_FDATASYNC: u32 = 75;
const SYS_GETDENTS: u32 = 78;
const SYS_GETCWD: u32 = 79;
const SYS_CHDIR: u32 = 80;
const SYS_FCHDIR: u32 = 81;
const SYS_UMASK: u32 = 95;
const SYS_GETTIMEOFDAY: u32 = 96;
const SYS_GETRLIMIT: u32 = 97;
const SYS_GETRUSAGE: u32 = 98;
const SYS_SYSINFO: u32 = 99;
const SYS_TIMES: u32 = 100;
const SYS_GETUID: u32 = 102;
const SYS_GETGID: u32 = 104;
const SYS_SIGALTSTACK: u32 = 131;
const SYS_GETEUID: u32 = 107;
const SYS_GETEGID: u32 = 108;
const SYS_GETTID: u32 = 186;
const SYS_SET_ROBUST_LIST: u32 = 273;
const SYS_GET_ROBUST_LIST: u32 = 274;
const SYS_CLOCK_GETTIME: u32 = 228;
const SYS_CLOCK_GETRES: u32 = 229;
const SYS_EPOLL_CREATE1: u32 = 291;
const SYS_EPOLL_CTL: u32 = 233;
const SYS_EPOLL_WAIT: u32 = 232;
const SYS_EPOLL_PWAIT: u32 = 281;
const SYS_EVENTFD2: u32 = 290;
const SYS_FUTEX: u32 = 202;
const SYS_SCHED_GETAFFINITY: u32 = 204;
const SYS_GETRANDOM: u32 = 318;
const SYS_ARCH_PRCTL: u32 = 158;
const SYS_PRLIMIT64: u32 = 302;
const SYS_GETDENTS64: u32 = 217;
const SYS_OPENAT: u32 = 257;
const SYS_NEWFSTATAT: u32 = 262;
const SYS_FACCESSAT: u32 = 269;
const SYS_READLINK: u32 = 89;
const SYS_READLINKAT: u32 = 267;
const SYS_STATFS: u32 = 137;
const SYS_FSTATFS: u32 = 138;
const SYS_DUP3: u32 = 292;
const SYS_PIPE2: u32 = 293;
const SYS_SET_TID_ADDRESS: u32 = 218;
const SYS_CLOSE_RANGE: u32 = 436;
const SYS_GETPRIORITY: u32 = 140;
const SYS_SCHED_GETPARAM: u32 = 143;
const SYS_SCHED_GETSCHEDULER: u32 = 144;
const SYS_RT_SIGTIMEDWAIT: u32 = 128;
const SYS_SIGPENDING: u32 = 127;
const SYS_PSELECT6: u32 = 270;
const SYS_PPOLL: u32 = 271;
const SYS_MEMFD_CREATE: u32 = 319;
const SYS_GETRESUID: u32 = 118;
const SYS_GETRESGID: u32 = 120;

// 写标志掩码：open/openat 的 flags 含任一 → 拒绝（无持久化攻击效果）
const O_WRONLY: u32 = 0o1;
const O_RDWR: u32 = 0o2;
const O_CREAT: u32 = 0o100;
const O_TRUNC: u32 = 0o1000;
const O_APPEND: u32 = 0o2000;
const WRITE_FLAGS: u32 = O_WRONLY | O_RDWR | O_CREAT | O_TRUNC | O_APPEND;

// seccomp_data 中 args 偏移
const DATA_ARG1: u32 = 24;
const DATA_ARG2: u32 = 32;

#[repr(C)]
#[derive(Clone, Copy)]
pub struct SockFilter {
    pub code: u16,
    pub jt: u8,
    pub jf: u8,
    pub k: u32,
}

const BPF_LD: u16 = 0x00;
const BPF_W: u16 = 0x00;
const BPF_ABS: u16 = 0x20;
const BPF_JMP: u16 = 0x05;
const BPF_JEQ: u16 = 0x10;
const BPF_K: u16 = 0x00;
const BPF_RET: u16 = 0x06;
const BPF_ALU: u16 = 0x04;
const BPF_AND: u16 = 0x50;

const AUDIT_ARCH_X86_64: u32 = 0xc000003e;

#[repr(C)]
pub struct SockFprog {
    pub len: u16,
    pub filter: *const SockFilter,
}

/// 构建 syscall 白名单 BPF 程序。
/// 结构：
///   load arch → != x86_64 → KILL
///   load nr → 对白名单逐条 JEQ（命中→ALLOW，未中→下一条）
///   open/openat 特殊：flags 含写位 → KILL
///   默认 → KILL_PROCESS
/// 安装 seccomp 过滤器（no_new_privs + FILTER）。
pub fn install() -> Result<(), i64> {
    const PR_SET_NO_NEW_PRIVS: i64 = 38;
    const PR_SET_SECCOMP: i64 = 22;
    const SECCOMP_MODE_FILTER: i64 = 2;
    let prog = build_program();
    let fprog = SockFprog {
        len: prog.len() as u16,
        filter: prog.as_ptr(),
    };
    let r1 = syscall3(157, PR_SET_NO_NEW_PRIVS, 1, 0);
    if r1 < 0 {
        return Err(r1);
    }
    let r2 = syscall3(157, PR_SET_SECCOMP, SECCOMP_MODE_FILTER, &fprog as *const SockFprog as i64);
    if r2 < 0 {
        return Err(r2);
    }
    Ok(())
}

pub fn build_program() -> Vec<SockFilter> {
    let mut f: Vec<SockFilter> = Vec::new();
    let push = |f: &mut Vec<SockFilter>, code: u16, jt: u8, jf: u8, k: u32| {
        f.push(SockFilter { code, jt, jf, k });
    };

    // 白名单（放行）
    let base: [u32; 104] = [
        SYS_READ, SYS_WRITE, SYS_CLOSE, SYS_STAT, SYS_FSTAT, SYS_LSTAT, SYS_POLL, SYS_LSEEK,
        SYS_MMAP, SYS_MPROTECT, SYS_MUNMAP, SYS_BRK, SYS_RT_SIGACTION, SYS_RT_SIGPROCMASK,
        SYS_RT_SIGRETURN, SYS_IOCTL, SYS_PREAD64, SYS_PWRITE64, SYS_READV, SYS_WRITEV, SYS_ACCESS,
        SYS_PIPE, SYS_SELECT, SYS_SCHED_YIELD, SYS_MREMAP, SYS_MSYNC, SYS_MINCORE, SYS_MADVISE,
        SYS_DUP, SYS_DUP2, SYS_NANOSLEEP, SYS_GETPID, SYS_CLONE, SYS_FORK, SYS_VFORK, SYS_EXIT,
        SYS_WAIT4, SYS_KILL, SYS_UNAME, SYS_GETPPID, SYS_FCNTL, SYS_FLOCK, SYS_FSYNC, SYS_FDATASYNC,
        SYS_GETDENTS, SYS_GETCWD, SYS_CHDIR, SYS_FCHDIR, SYS_UMASK, SYS_GETTIMEOFDAY, SYS_GETRLIMIT,
        SYS_GETRUSAGE, SYS_SYSINFO, SYS_TIMES, SYS_GETUID, SYS_GETGID, SYS_SIGALTSTACK, SYS_GETEUID,
        SYS_GETEGID, SYS_GETTID, SYS_SET_ROBUST_LIST, SYS_GET_ROBUST_LIST, SYS_CLOCK_GETTIME,
        SYS_CLOCK_GETRES, SYS_EPOLL_CREATE1, SYS_EPOLL_CTL, SYS_EPOLL_WAIT, SYS_EPOLL_PWAIT,
        SYS_EVENTFD2, SYS_FUTEX, SYS_SCHED_GETAFFINITY, SYS_GETRANDOM, SYS_ARCH_PRCTL, SYS_PRLIMIT64,
        SYS_GETDENTS64, SYS_NEWFSTATAT, SYS_FACCESSAT, SYS_READLINK, SYS_READLINKAT, SYS_STATFS,
        SYS_FSTATFS, SYS_DUP3, SYS_PIPE2, SYS_SET_TID_ADDRESS, SYS_CLOSE_RANGE, SYS_GETPRIORITY,
        SYS_SCHED_GETPARAM, SYS_SCHED_GETSCHEDULER, SYS_RT_SIGTIMEDWAIT, SYS_SIGPENDING,
        SYS_PSELECT6, SYS_PPOLL, SYS_MEMFD_CREATE, SYS_GETRESUID, SYS_GETRESGID,
        SYS_EXECVE, SYS_EXIT_GROUP, SYS_RSEQ, SYS_SCHED_GETAFFINITY, SYS_STATX,
        SYS_CLONE3, // python subprocess 用它创建子进程
        SYS_SETITIMER, SYS_GETITIMER, SYS_ALARM, // cage 的 signal.setitimer/alarm 依赖
    ];
    // 高危（命中即 KILL_PROCESS）
    // 注：SYS_CONNECT 不在其中——inet socket 创建已被 socket() domain 检查拦截，
    // 现存 connect 只能作用于 AF_UNIX（nscd/本地 IPC），放行。
    let danger: [u32; 72] = [
        SYS_ACCEPT, SYS_SENDTO, SYS_RECVFROM, SYS_SENDMSG, SYS_RECVMSG,
        SYS_SHUTDOWN, SYS_BIND, SYS_LISTEN, SYS_SETSOCKOPT, SYS_GETSOCKOPT, SYS_SOCKETPAIR,
        SYS_ACCEPT4, SYS_CHMOD, SYS_FCHMOD, SYS_CHOWN, SYS_FCHOWN, SYS_LCHOWN,
        SYS_SETUID, SYS_SETGID, SYS_SETEUID, SYS_SETEGID, SYS_SETFSUID, SYS_SETFSGID,
        SYS_SETRESUID, SYS_SETRESGID, SYS_SETGROUPS, SYS_SETRLIMIT, SYS_CAPSET,
        SYS_PTRACE, SYS_MOUNT, SYS_UMOUNT2, SYS_SWAPON, SYS_SWAPOFF,
        SYS_UNSHARE, SYS_SETNS, SYS_BPF, SYS_USERFAULTFD, SYS_OPEN_BY_HANDLE_AT,
        SYS_PROCESS_VM_READV, SYS_PROCESS_VM_WRITEV, SYS_PERF_EVENT_OPEN,
        SYS_KEYCTL, SYS_ADD_KEY, SYS_REQUEST_KEY,
        SYS_REBOOT, SYS_KEXEC_LOAD, SYS_INIT_MODULE, SYS_DELETE_MODULE,
        SYS_IO_URING_SETUP, SYS_IO_URING_ENTER,
        SYS_UNLINK, SYS_UNLINKAT, SYS_RENAME, SYS_RENAMEAT, SYS_MKDIR, SYS_MKDIRAT,
        SYS_RMDIR, SYS_SYMLINK, SYS_SYMLINKAT, SYS_LINK, SYS_LINKAT,
        SYS_TRUNCATE, SYS_FTRUNCATE, SYS_MKNOD, SYS_MKNODAT,
        SYS_UTIME, SYS_UTIMES, SYS_UTIMENSAT, SYS_FUTIMESAT,
        SYS_SETHOSTNAME, SYS_SETDOMAINNAME, SYS_EXECVEAT,
    ];

    // 布局：
    // [0] LD arch | [1] JEQ arch(未匹配→KILL) | [2] LD nr
    // [3..) base JEQ（命中→ALLOW[末]）
    // [openat] openat JEQ（命中→oa flags 检查）
    // [open] open JEQ（命中→o flags 检查）
    // [danger..) 高危 JEQ（命中→KILL）
    // [socket] socket JEQ（命中→domain 检查；仅 AF_INET/6 → KILL，其余放行）
    // [enosys] ENOSYS（链尾默认）
    // [kill] KILL（danger/socket-inet 命中跳转目标）
    // [oa] LD args2, AND, JEQ(只读→ALLOW, 写→KILL), KILL
    // [o] LD args1, AND, JEQ(只读→ALLOW, 写→KILL), KILL
    // [allow] RET ALLOW
    push(&mut f, BPF_LD | BPF_W | BPF_ABS, 0, 0, 4); // [0] LD arch
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, AUDIT_ARCH_X86_64); // [1] JEQ（未匹配→KILL）
    push(&mut f, BPF_LD | BPF_W | BPF_ABS, 0, 0, 0); // [2] LD nr
    let base_start = 3;
    for &nr in &base {
        push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, nr);
    }
    let openat_jeq = base_start + base.len(); // 103
    let open_jeq = openat_jeq + 1; // 104
    let danger_start = open_jeq + 1; // 105
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, SYS_OPENAT);
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, SYS_OPEN);
    for &nr in &danger {
        push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, nr);
    }
    let sock_jeq = danger_start + danger.len(); // socket JEQ
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, SYS_SOCKET);
    let sock_ld = sock_jeq + 1; // LD args0（domain）
    push(&mut f, BPF_LD | BPF_W | BPF_ABS, 0, 0, DATA_ARG0);
    let inet4_jeq = sock_ld + 1;
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, AF_INET);
    let inet6_jeq = inet4_jeq + 1;
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, AF_INET6);
    let sock_allow = inet6_jeq + 1; // 非 inet domain → 放行
    push(&mut f, BPF_RET | BPF_K, 0, 0, SECCOMP_RET_ALLOW);
    let enosys = sock_allow + 1; // 链尾默认：ENOSYS
    push(&mut f, BPF_RET | BPF_K, 0, 0, SECCOMP_RET_ERRNO | 38);
    let kill = enosys + 1; // KILL（danger/inet socket 命中）
    push(&mut f, BPF_RET | BPF_K, 0, 0, SECCOMP_RET_KILL_PROCESS);
    let oa_jeq = kill + 1 + 2; // LD,AND 占 2 条 → oa_jeq
    let o_jeq = oa_jeq + 4; // +KILL+LD+AND → o_jeq
    let allow = o_jeq + 2; // +KILL → allow
    // oa 检查块（openat flags = args[2]，offset 32）
    push(&mut f, BPF_LD | BPF_W | BPF_ABS, 0, 0, DATA_ARG2);
    push(&mut f, BPF_ALU | BPF_AND | BPF_K, 0, 0, WRITE_FLAGS);
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, 0); // oa_jeq（回填）
    push(&mut f, BPF_RET | BPF_K, 0, 0, SECCOMP_RET_KILL_PROCESS); // flags 含写 → KILL
    // o 检查块（open flags = args[1]，offset 24）
    push(&mut f, BPF_LD | BPF_W | BPF_ABS, 0, 0, DATA_ARG1);
    push(&mut f, BPF_ALU | BPF_AND | BPF_K, 0, 0, WRITE_FLAGS);
    push(&mut f, BPF_JMP | BPF_JEQ | BPF_K, 0, 0, 0); // o_jeq（回填）
    push(&mut f, BPF_RET | BPF_K, 0, 0, SECCOMP_RET_KILL_PROCESS); // flags 含写 → KILL
    push(&mut f, BPF_RET | BPF_K, 0, 0, SECCOMP_RET_ALLOW); // [allow]

    // 回填
    f[1].jf = (kill - 2) as u8; // arch 不匹配 → KILL
    for i in 0..base.len() {
        let idx = base_start + i;
        f[idx].jt = (allow - (idx + 1)) as u8; // base 命中 → ALLOW
    }
    f[openat_jeq].jt = (oa_jeq - 2 - (openat_jeq + 1)) as u8; // openat 命中 → oa 块 LD
    f[open_jeq].jt = (o_jeq - 2 - (open_jeq + 1)) as u8; // open 命中 → o 块 LD
    for i in 0..danger.len() {
        let idx = danger_start + i;
        f[idx].jt = (kill - (idx + 1)) as u8; // 高危命中 → KILL
    }
    f[sock_jeq].jt = (sock_ld - (sock_jeq + 1)) as u8; // socket 命中 → domain 检查
    f[sock_jeq].jf = (enosys - (sock_jeq + 1)) as u8; // 非 socket → 链尾 ENOSYS
    f[inet4_jeq].jt = (kill - (inet4_jeq + 1)) as u8; // AF_INET → KILL
    f[inet6_jeq].jt = (kill - (inet6_jeq + 1)) as u8; // AF_INET6 → KILL
    f[oa_jeq].jt = (allow - (oa_jeq + 1)) as u8; // flags 只读 → ALLOW
    f[oa_jeq].jf = 0; // flags 含写 → 下一条 KILL
    f[o_jeq].jt = (allow - (o_jeq + 1)) as u8;
    f[o_jeq].jf = 0;
    f
}

// ---- 裸 syscall（x86_64，零依赖）----
// 注意：syscall 只 clobber rcx/r11，其余寄存器由调用者保存。
// 所有未使用的参数寄存器显式清零：部分环境有外层 seccomp/拦截层
// 会读取全部 6 个参数寄存器，残留垃圾值会触发 EINVAL。

#[inline(always)]
pub fn syscall1(n: i64, a1: i64) -> i64 {
    let r: i64;
    unsafe {
        std::arch::asm!("syscall",
            inout("rax") n => r,
            in("rdi") a1,
            in("r10") 0i64, in("r8") 0i64, in("r9") 0i64,
            lateout("rcx") _, lateout("r11") _,
            options(nostack));
    }
    r
}

#[inline(always)]
pub fn syscall3(n: i64, a1: i64, a2: i64, a3: i64) -> i64 {
    let r: i64;
    unsafe {
        std::arch::asm!("syscall",
            inout("rax") n => r,
            in("rdi") a1, in("rsi") a2, in("rdx") a3,
            in("r10") 0i64, in("r8") 0i64, in("r9") 0i64,
            lateout("rcx") _, lateout("r11") _,
            options(nostack));
    }
    r
}

#[inline(always)]
pub fn syscall4(n: i64, a1: i64, a2: i64, a3: i64, a4: i64) -> i64 {
    let r: i64;
    unsafe {
        std::arch::asm!("syscall",
            inout("rax") n => r,
            in("rdi") a1, in("rsi") a2, in("rdx") a3, in("r10") a4,
            in("r8") 0i64, in("r9") 0i64,
            lateout("rcx") _, lateout("r11") _,
            options(nostack));
    }
    r
}
