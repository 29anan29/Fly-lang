//go:build linux

package main

// fly sandbox: 进程级沙箱（Fast 模式）——namespaces + Landlock + seccomp + rlimit。
// 主进程 → (fork) stage1 supervisor（unshare 命名空间）→ (fork) stage2 受限进程
// （Landlock 默认拒绝文件访问 → rlimit → seccomp 白名单 → execve python3）。
// 仅支持 x86_64（seccomp 白名单按架构维护，避免错误编号导致过滤失效）。

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	sandboxStageEnv = "_FLY_SANDBOX_STAGE"
	sandboxUIDEnv   = "_FLY_SANDBOX_UID"
	sandboxGIDEnv   = "_FLY_SANDBOX_GID"
	sandboxExeEnv   = "_FLY_SANDBOX_EXE"
)

// ---------- Landlock ----------

const (
	landlockCreateRuleset   = 444
	landlockAddRule         = 445
	landlockRestrictSelf    = 446
	landlockRulePathBeneath = 1

	// Go 标准库 syscall 缺失的常量（x/sys/unix 才有，项目零第三方依赖故自定）
	sbOPath             = 0x200000 // x86_64
	sbPRSetNoNewPrivs   = 38
	sbPRSetSeccomp      = 22
	sbSeccompModeFilter = 2
)

var sbATFDCWD = int64(-100)

const (
	landlockAccessFSExecute   = uint64(1 << 0)
	landlockAccessFSWriteFile = uint64(1 << 1)
	landlockAccessFSReadFile  = uint64(1 << 2)
	landlockAccessFSReadDir   = uint64(1 << 3)
)

// 默认拒绝需要 handled 覆盖全部可执行访问位（仅覆盖 3 位时
// MAKE_DIR/MAKE_REG 等不在管辖内，沙箱内可任意建文件）。
const landlockHandledFSFull = uint64(0xFFFF) // ABI v5 全集（EXECUTE..IOCTL_DEV）

// Go 无 packed：16 字节布局，内核只读前 12 字节（实测兼容）
type landlockPathBeneathAttr struct {
	allowedAccess uint64
	parentFD      int32
}

type landlockRulesetAttr struct {
	handledAccessFS uint64
}

func probeLandlockHandled() uint64 {
	for _, h := range []uint64{landlockHandledFSFull, 0x7FFF, 0x3FFF} {
		attr := landlockRulesetAttr{handledAccessFS: h}
		fd, _, errno := syscall.Syscall(
			landlockCreateRuleset,
			uintptr(unsafe.Pointer(&attr)),
			unsafe.Sizeof(attr),
			0,
		)
		if errno == 0 {
			syscall.Close(int(fd))
			return h
		}
	}
	return 0
}

func landlockOpenDir(path string) int {
	bp, err := syscall.BytePtrFromString(path)
	if err != nil {
		return -1
	}
	fd, _, _ := syscall.Syscall(
		syscall.SYS_OPENAT,
		uintptr(sbATFDCWD),
		uintptr(unsafe.Pointer(bp)),
		sbOPath|syscall.O_DIRECTORY|syscall.O_CLOEXEC,
	)
	return int(fd)
}

func landlockGrant(rulesetFD int, path string, access uint64) bool {
	fd := landlockOpenDir(path)
	if fd < 0 {
		// 目录不存在是常态（如 /usr/local/lib64 仅在部分发行版存在），静默跳过
		return false
	}
	attr := landlockPathBeneathAttr{allowedAccess: access, parentFD: int32(fd)}
	rc, _, errno := syscall.Syscall6(
		landlockAddRule,
		uintptr(rulesetFD),
		landlockRulePathBeneath,
		uintptr(unsafe.Pointer(&attr)),
		0, 0, 0,
	)
	syscall.Close(fd)
	if rc == 0 {
		return true
	}
	fmt.Fprintf(os.Stderr, "{\"event\":\"landlock_grant_fail\",\"path\":\"%s\",\"err\":\"add_rule: %v\"}\n", path, errno)
	return false
}

var landlockSyslibDirs = []string{
	"/usr/lib",
	"/usr/local/lib",
	"/usr/lib64",
	"/usr/local/lib64",
	"/lib",
	"/lib64",
	"/usr/bin",
	"/usr/local/bin",
}

func applyLandlock(o *sandboxOpts, audit bool) error {
	handled := probeLandlockHandled()
	if handled == 0 {
		return fmt.Errorf("内核不支持 Landlock（需 Linux >= 5.13）")
	}
	// restrict_self 需要 no_new_privs 或 root：非 root 必须先行设置（seccomp 处也会设）
	if _, _, errno := syscall.Syscall(
		syscall.SYS_PRCTL, sbPRSetNoNewPrivs, 1, 0,
	); errno != 0 {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS 失败: %v", errno)
	}
	attr := landlockRulesetAttr{handledAccessFS: handled}
	rulesetFD, _, errno := syscall.Syscall(
		landlockCreateRuleset,
		uintptr(unsafe.Pointer(&attr)),
		unsafe.Sizeof(attr),
		0,
	)
	if errno != 0 {
		return fmt.Errorf("landlock_create_ruleset 失败: %v", errno)
	}
	rfd := int(rulesetFD)
	defer syscall.Close(rfd)

	var granted []string
	sysAccess := landlockAccessFSExecute | landlockAccessFSReadFile | landlockAccessFSReadDir
	dataAccess := landlockAccessFSReadFile | landlockAccessFSReadDir
	for _, d := range landlockSyslibDirs {
		if landlockGrant(rfd, d, sysAccess) {
			granted = append(granted, d)
		}
	}
	// 脚本目录（相对路径也转绝对，否则 dirOf 为空无法授权）
	if abs, err := filepath.Abs(o.script); err == nil {
		if dir := dirOf(abs); dir != "" {
			if landlockGrant(rfd, dir, dataAccess) {
				granted = append(granted, dir)
			}
		}
	}
	for _, p := range o.capFSRead {
		if landlockGrant(rfd, p, dataAccess) {
			granted = append(granted, p)
		}
	}

	rc, _, errno := syscall.Syscall(landlockRestrictSelf, uintptr(rfd), 0, 0)
	if rc != 0 {
		return fmt.Errorf("landlock_restrict_self 失败: %v", errno)
	}
	sandboxAudit(audit, "{\"event\":\"landlock\",\"granted\":[%s]}", strings.Join(quoteAll(granted), ","))
	return nil
}

func quoteAll(ss []string) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = fmt.Sprintf("%q", s)
	}
	return out
}

func dirOf(path string) string {
	i := strings.LastIndexByte(path, '/')
	if i <= 0 {
		return ""
	}
	return path[:i]
}

// ---------- seccomp BPF ----------

const sandboxArchAudit = 0xc000003e // AUDIT_ARCH_X86_64

// 白名单系统调用（x86_64）；clone 由尾部特判 flags 不含 CLONE_NEW*。
// clone3(435) 同样特判：Go runtime/python 线程创建带 CLONE_NEW* 时 EINVAL 前
// 直接拦截（沙箱内 uid 0 可 clone3 逃逸出 net/mount ns）。
// 38 = setitimer：cage max_time 的 _fly_signal.setitimer 走此调用。
var sandboxSysWhitelist = []uint32{
	0, 1, 3, 5, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 25, 28, 32, 33, 35, 38, 39,
	41, 57, 59, 60, 61, 62, 63, 72, 73, 74, 77, 78, 79, 80, 81, 83, 84, 86, 87, 89, 90, 91, 92,
	93, 94, 95, 96, 98, 99, 102, 104, 107, 108, 110, 115, 116, 124, 131, 137, 138, 141, 158,
	186, 202, 204, 217, 218, 219, 228, 229, 230, 231, 232, 233, 234, 257, 258, 260, 261, 262,
	263, 264, 265, 266, 267, 268, 269, 280, 288, 290, 291, 292, 293, 295, 296, 297, 302, 318,
	332, 334, 437,
}

const (
	sandboxSysClone  = 56
	sandboxSysClone3 = 435
)

const sandboxCloneNewMask = 0x00020000 | // CLONE_NEWNS
	0x02000000 | // CLONE_NEWCGROUP
	0x04000000 | // CLONE_NEWUTS
	0x08000000 | // CLONE_NEWIPC
	0x10000000 | // CLONE_NEWUSER
	0x20000000 | // CLONE_NEWPID
	0x40000000 // CLONE_NEWNET

const (
	bpfLDWABS  = 0x20
	bpfJMPJA   = 0x05
	bpfJMPJEQK = 0x15
	bpfALUANDK = 0x54
	bpfRETK    = 0x06

	seccompRetAllow       = 0x7fff0000
	seccompRetErrno       = 0x00050000
	seccompRetKillProcess = 0x80000000
)

func buildSandboxBpf() []syscall.SockFilter {
	var p []syscall.SockFilter
	emit := func(code uint16, jt, jf uint8, k uint32) {
		p = append(p, syscall.SockFilter{Code: code, Jt: jt, Jf: jf, K: k})
	}
	emit(bpfLDWABS, 0, 0, 4) // ld arch
	emit(bpfJMPJEQK, 0, 0, sandboxArchAudit)
	emit(bpfLDWABS, 0, 0, 0) // ld nr
	for _, nr := range sandboxSysWhitelist {
		emit(bpfJMPJEQK, 0, 1, nr)
		emit(bpfRETK, 0, 0, seccompRetAllow)
	}
	// clone/clone3 特判：flags 含 CLONE_NEW* 一律拦截（沙箱内禁止新建命名空间）。
	// 56 匹配跳 2（越过 435 判定与 EPERM）到 flags 检查；435 匹配跳 1（越过 EPERM）；
	// 两者都不匹配（白名单外非 clone 变体）→ EPERM
	emit(bpfJMPJEQK, 2, 0, sandboxSysClone)  // 56 匹配 → flags 检查
	emit(bpfJMPJEQK, 1, 0, sandboxSysClone3) // 435 匹配 → flags 检查
	emit(bpfRETK, 0, 0, seccompRetErrno|1)   // 非 clone/clone3 → EPERM
	emit(bpfLDWABS, 0, 0, 16)                // args0（clone/clone3 flags 都在 arg0）
	emit(bpfALUANDK, 0, 0, sandboxCloneNewMask)
	emit(bpfJMPJEQK, 0, 1, 0) // flags 干净 → allow
	emit(bpfRETK, 0, 0, seccompRetAllow)
	emit(bpfRETK, 0, 0, seccompRetErrno|1)     // 含 CLONE_NEW* → EPERM
	emit(bpfRETK, 0, 0, seccompRetKillProcess) // 仅 arch 不符兜底
	p[1].Jf = uint8(len(p) - 3)                // arch 不匹配 → 跳最后一条 kill
	return p
}

func applySandboxSeccomp() error {
	prog := buildSandboxBpf()
	fprog := syscall.SockFprog{Len: uint16(len(prog)), Filter: &prog[0]}
	if _, _, errno := syscall.Syscall(
		syscall.SYS_PRCTL, sbPRSetNoNewPrivs, 1, 0,
	); errno != 0 {
		return fmt.Errorf("PR_SET_NO_NEW_PRIVS 失败: %v", errno)
	}
	if _, _, errno := syscall.Syscall(
		syscall.SYS_PRCTL,
		sbPRSetSeccomp,
		uintptr(sbSeccompModeFilter),
		uintptr(unsafe.Pointer(&fprog)),
	); errno != 0 {
		return fmt.Errorf("PR_SET_SECCOMP 失败: %v", errno)
	}
	return nil
}

// ---------- 命名空间 / 资源 ----------

// rlimit 注入到 python wrapper（而非 stage2 的 Go runtime）：
// Go 1.26 runtime 启动即保留约 1.2GB 虚拟地址空间（arena），stage2 内设
// RLIMIT_AS 会在 exec 前偶发 mmap OOM（runtime/out of memory abort）。
// exec 后 python 的 VA 很小，此处设置精确且无冲突。
func sandboxLimitWrapper() string {
	return `import os, resource, runpy, sys
def _fly_set(name, res):
    v = os.environ.get(name)
    if v:
        resource.setrlimit(res, (int(v), int(v)))
_fly_set("_FLY_MEM_MB", resource.RLIMIT_AS)
_fly_set("_FLY_CPU_SEC", resource.RLIMIT_CPU)
_fly_set("_FLY_NOFILE", resource.RLIMIT_NOFILE)
for k in ("_FLY_MEM_MB", "_FLY_CPU_SEC", "_FLY_NOFILE"):
    os.environ.pop(k, None)
sys.argv = [sys.argv[1]] + sys.argv[2:]
runpy.run_path(sys.argv[0], run_name="__main__")
`
}

// stage2: 受限进程——Landlock → seccomp → execve python3（rlimit 由注入 wrapper 设置）
func sandboxStage2(args []string) int {
	o, err := parseSandboxArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if err := applyLandlock(o, o.audit); err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"landlock_failed\",\"err\":\"%v\"}\n", err)
		return 1
	}
	if err := applySandboxSeccomp(); err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"seccomp_failed\",\"err\":\"%v\"}\n", err)
		return 1
	}
	env := append(os.Environ(),
		"_FLY_MEM_MB="+strconv.FormatUint(o.memLimitMB*1024*1024, 10),
		"_FLY_CPU_SEC="+strconv.FormatUint(o.cpuSec, 10),
		"_FLY_NOFILE="+strconv.FormatUint(o.nofile, 10))
	argv := []string{"python3", "-c", sandboxLimitWrapper(), o.script}
	if err := syscall.Exec("/usr/bin/python3", argv, env); err != nil {
		fmt.Fprintf(os.Stderr, "{\"event\":\"exec_failed\",\"err\":\"%v\"}\n", err)
		return 127
	}
	return 0
}

// 命名空间 flags（clone 时直接创建；非 root 需带头 CLONE_NEWUSER 以便映射 uid）。
// 必须用 Cloneflags 而非 Unshareflags：unshare 建 PID ns 时进程留在宿主 PID ns，
// 沙箱内 Go runtime 创建线程的 clone3 会 EINVAL（go runtime abort）。
func sandboxNSFlags(o *sandboxOpts) uintptr {
	if o.debugNS {
		return 0
	}
	if syscall.Geteuid() != 0 {
		return syscall.CLONE_NEWUSER | syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS |
			syscall.CLONE_NEWIPC | syscall.CLONE_NEWPID | syscall.CLONE_NEWNET
	}
	return syscall.CLONE_NEWNS | syscall.CLONE_NEWUTS | syscall.CLONE_NEWIPC |
		syscall.CLONE_NEWPID | syscall.CLONE_NEWNET
}

// ---------- 阶段执行 ----------

// python bridge：仅做进程转发（exec stage2）。Go runtime 多线程无法首次写
// uid_map（EPERM，内核要求单线程）——userns 映射改由 SysProcAttr 的
// UidMappings/GidMappings 完成（Go 在单线程 fork 子进程里写 map）。
func sandboxBridgeScript() string {
	return `import os, sys
exe = os.environ["` + sandboxExeEnv + `"]
os.execv(exe, [exe, "sandbox"] + sys.argv[1:])
`
}

// 透传给 stage2 的参数（stage2 需要重建 opts）
func sandboxForwardFlags(o *sandboxOpts) []string {
	var f []string
	for _, p := range o.capFSRead {
		f = append(f, "--cap-fs-read", p)
	}
	for _, h := range o.capNetHost {
		f = append(f, "--cap-net-host", h)
	}
	f = append(f, "--mem-limit-mb", strconv.FormatUint(o.memLimitMB, 10))
	f = append(f, "--timeout-ms", strconv.FormatUint(o.timeoutMS, 10))
	f = append(f, "--cpu-sec", strconv.FormatUint(o.cpuSec, 10))
	f = append(f, "--nofile", strconv.FormatUint(o.nofile, 10))
	if !o.audit {
		f = append(f, "--no-audit")
	}
	return f
}

// ---------- 命令入口 ----------

func cmdSandbox(args []string) int {
	if os.Getenv(sandboxStageEnv) == "2" {
		return sandboxStage2(args)
	}

	o, err := parseSandboxArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		sandboxUsage()
		return 2
	}
	if runtime.GOARCH != "amd64" {
		fmt.Fprintln(os.Stderr, "fly sandbox v0.1 仅支持 x86_64（seccomp 白名单按架构维护）")
		return 2
	}
	if info, err := os.Stat(o.script); err != nil || !info.Mode().IsRegular() {
		fmt.Fprintf(os.Stderr, "错误: 脚本 %s 不存在\n", o.script)
		return 1
	}

	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "os.Executable:", err)
		return 1
	}
	started := time.Now()
	sandboxAudit(o.audit, "{\"event\":\"start\",\"script\":\"%s\",\"mem_mb\":%d,\"timeout_ms\":%d}", o.script, o.memLimitMB, o.timeoutMS)

	// 主进程 → python bridge（clone 建命名空间，规避 Go 多线程下 unshare EINVAL）
	// → bridge exec stage2。userns 映射由 UidMappings/GidMappings 完成
	// （Go 在单线程 fork 子进程里写 map，内核要求首次写 map 单线程）。
	// 全程同一进程组（Setpgid），超时 kill(-pgid) 一锅端。
	argv := append([]string{"/usr/bin/python3", "-c", sandboxBridgeScript(),
		o.script}, sandboxForwardFlags(o)...)
	env := append(os.Environ(), sandboxStageEnv+"=2",
		fmt.Sprintf("%s=%s", sandboxExeEnv, exe))
	sys := &syscall.SysProcAttr{Setpgid: true, Cloneflags: sandboxNSFlags(o)}
	if syscall.Geteuid() != 0 && !o.debugNS {
		sys.UidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: syscall.Getuid(), Size: 1}}
		sys.GidMappings = []syscall.SysProcIDMap{{ContainerID: 0, HostID: syscall.Getgid(), Size: 1}}
	}
	pid, err := syscall.ForkExec("/usr/bin/python3", argv, &syscall.ProcAttr{
		Env:   env,
		Files: []uintptr{0, 1, 2}, // Files 为 nil 时 Go 会关闭全部 fd（含 stdio）
		Sys:   sys,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "spawn 失败: %v\n", err)
		return 1
	}

	deadline := time.After(time.Duration(o.timeoutMS) * time.Millisecond)
	timedOut := false
	var ws syscall.WaitStatus
	done := make(chan struct{})
	go func() {
		syscall.Wait4(pid, &ws, 0, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-deadline:
		timedOut = true
		syscall.Kill(-pid, syscall.SIGKILL)
		<-done
	}
	wall := time.Since(started).Milliseconds()
	if timedOut {
		sandboxAudit(o.audit, "{\"event\":\"timeout\",\"ms\":%d}", o.timeoutMS)
		return 124
	}
	var code int
	if ws.Exited() {
		code = ws.ExitStatus()
	} else if ws.Signaled() {
		code = 128 + int(ws.Signal())
	} else {
		code = 1
	}
	sandboxAudit(o.audit, "{\"event\":\"exit\",\"code\":%d,\"wall_ms\":%d}", code, wall)
	return code
}
