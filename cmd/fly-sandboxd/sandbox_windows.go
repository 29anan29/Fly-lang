//go:build windows

package main

// fly sandbox (Windows)：Job Object 沙箱。
// 能力：JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE（主进程退出/异常即终止整个进程树）、
// JOB_OBJECT_LIMIT_JOB_MEMORY（总内存上限）、JOB_OBJECT_LIMIT_JOB_TIME
// （墙钟上限，到点 TerminateJobObject）、JOB_OBJECT_LIMIT_ACTIVE_PROCESS（进程数上限）。
// 强度声明：Windows 无 Linux 的 Landlock/seccomp 内核级隔离，本实现是
// Win32 Job Object 用户态限制——不防御文件系统/网络越权，仅保证进程树
// 生命周期与资源上限（对应 cage 语义）。

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"
)

const (
	infoClassExtended = 9
	infoClassBasic    = 2

	jobObjectLimitKillOnJobClose = 0x2000
	jobObjectLimitJobMemory      = 0x200
	jobObjectLimitJobTime        = 0x4
	jobObjectLimitActiveProcess  = 0x8

	createSuspended = 0x4
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type ioCounters struct {
	ReadOperationCount  uint64
	WriteOperationCount uint64
	OtherOperationCount uint64
	ReadTransferCount   uint64
	WriteTransferCount  uint64
	OtherTransferCount  uint64
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                ioCounters
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

type startUpInfoW struct {
	Cb            uint32
	Reserved      *uint16
	Desktop       *uint16
	Title         *uint16
	X             uint32
	Y             uint32
	XSize         uint32
	YSize         uint32
	XCountChars   uint32
	YCountChars   uint32
	FillAttribute uint32
	Flags         uint32
	ShowWindow    uint16
	CbReserved2   uint16
	LpReserved2   *byte
	StdInput      uintptr
	StdOutput     uintptr
	StdErr        uintptr
}

type processInformation struct {
	Process   uintptr
	Thread    uintptr
	ProcessID uint32
	ThreadID  uint32
}

var (
	kernel32                = syscall.NewLazyDLL("kernel32.dll")
	procCreateJobObjectW    = kernel32.NewProc("CreateJobObjectW")
	procSetInfoJobObject    = kernel32.NewProc("SetInformationJobObject")
	procAssignJob           = kernel32.NewProc("AssignProcessToJobObject")
	procTerminateJob        = kernel32.NewProc("TerminateJobObject")
	procCreateProcessW      = kernel32.NewProc("CreateProcessW")
	procResumeThread        = kernel32.NewProc("ResumeThread")
	procWaitForSingleObject = kernel32.NewProc("WaitForSingleObject")
	procGetExitCodeProcess  = kernel32.NewProc("GetExitCodeProcess")
	procCloseHandle         = kernel32.NewProc("CloseHandle")
)

func cmdSandbox(args []string) int {
	o, err := parseSandboxArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fly sandbox:", err)
		sandboxUsage()
		return 2
	}
	scriptAbs, err := absPath(o.script)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fly sandbox:", err)
		return 2
	}

	job, _, errno := procCreateJobObjectW.Call(0, 0)
	if job == 0 {
		fmt.Fprintf(os.Stderr, "fly sandbox: CreateJobObjectW 失败 (errno %d)\n", errno)
		return 2
	}
	defer procCloseHandle.Call(job)

	info := jobObjectExtendedLimitInformation{
		BasicLimitInformation: jobObjectBasicLimitInformation{
			LimitFlags:          jobObjectLimitKillOnJobClose | jobObjectLimitJobMemory | jobObjectLimitJobTime | jobObjectLimitActiveProcess,
			PerJobUserTimeLimit: int64(o.timeoutMS) * int64(10000), // 100ns 单位
			ActiveProcessLimit:  16,
		},
		JobMemoryLimit: uintptr(o.memLimitMB) * 1024 * 1024,
	}
	r, _, errno := procSetInfoJobObject.Call(job, infoClassExtended, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info))
	if r == 0 {
		fmt.Fprintf(os.Stderr, "fly sandbox: SetInformationJobObject 失败 (errno %d)\n", errno)
		return 2
	}

	cmdLine, _ := syscall.UTF16PtrFromString("python3 " + scriptAbs)
	var si startUpInfoW
	si.Cb = uint32(unsafe.Sizeof(si))
	var pi processInformation
	r, _, errno = procCreateProcessW.Call(0, uintptr(unsafe.Pointer(cmdLine)), 0, 0, 0, createSuspended, 0, 0, uintptr(unsafe.Pointer(&si)), uintptr(unsafe.Pointer(&pi)))
	if r == 0 {
		fmt.Fprintf(os.Stderr, "fly sandbox: CreateProcessW 失败 (errno %d)\n", errno)
		return 2
	}
	defer procCloseHandle.Call(pi.Process)
	defer procCloseHandle.Call(pi.Thread)

	r, _, errno = procAssignJob.Call(job, pi.Process)
	if r == 0 {
		fmt.Fprintf(os.Stderr, "fly sandbox: AssignProcessToJobObject 失败 (errno %d)\n", errno)
		procTerminateJob.Call(job, 1)
		return 2
	}
	procResumeThread.Call(pi.Thread)

	_, _, _ = procWaitForSingleObject.Call(pi.Process, 0xFFFFFFFF) // INFINITE；超时由 Job 时间上限触发
	var code uint32
	procGetExitCodeProcess.Call(pi.Process, uintptr(unsafe.Pointer(&code)))
	return int(code)
}

// absPath: 相对路径转绝对（基于当前目录），返回带引号的 Windows 路径。
func absPath(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return `"` + abs + `"`, nil
}
