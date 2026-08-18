//go:build darwin

package main

// fly sandbox (macOS)：Seatbelt 沙箱（sandbox-exec profile）。
// 能力：文件系统写入仅限临时目录/用户目录/当前目录；网络一律禁用；
// 内存（RLIMIT_AS）与 CPU 时间（RLIMIT_CPU）上限；墙钟超时强制杀进程树。
// 强度声明：macOS 无 Linux 的 Landlock/seccomp 内核级隔离，本实现是
// 用户态 Seatbelt 策略（官方标记 deprecated，Apple 长期保留）——阻止
// 网络与越权文件写，但不防御内核漏洞。

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func cmdSandbox(args []string) int {
	o, err := parseSandboxArgs(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fly sandbox:", err)
		sandboxUsage()
		return 2
	}
	if _, err := exec.LookPath("sandbox-exec"); err != nil {
		fmt.Fprintln(os.Stderr, "fly sandbox: 需要 macOS sandbox-exec（系统自带，Apple 保留但标记废弃）")
		return 2
	}
	scriptAbs, err := filepath.Abs(o.script)
	if err != nil {
		fmt.Fprintln(os.Stderr, "fly sandbox:", err)
		return 2
	}
	inner := "ulimit -v " + strconv.FormatUint(o.memLimitMB*1024, 10) +
		"; ulimit -t " + strconv.FormatUint(o.cpuSec, 10) +
		"; exec " + shellQuote(scriptAbs)
	cmd := exec.Command("sandbox-exec", "-p", seatbeltProfile(o), "/bin/sh", "-c", inner)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintln(os.Stderr, "fly sandbox:", err)
		return 2
	}
	timeout := time.Duration(o.timeoutMS) * time.Millisecond
	timer := time.AfterFunc(timeout, func() {
		if o.audit {
			fmt.Fprintf(os.Stderr, "[fly-sandbox] audit: 超时 %dms 强制终止进程组\n", o.timeoutMS)
		}
		syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})
	err = cmd.Wait()
	timer.Stop()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintln(os.Stderr, "fly sandbox:", err)
		return 2
	}
	return 0
}

// seatbeltProfile: SBPL 策略——默认拒绝，系统库读放行，写入仅限临时/用户/当前目录，网络全禁。
func seatbeltProfile(o *sandboxOpts) string {
	p := "(version 1)\n(deny default)\n(import \"system.sb\")\n" +
		"(allow process-exec)\n" +
		"(allow file-read*)\n" +
		"(deny file-write*)\n" +
		"(allow file-write* (subpath \"/tmp\"))\n" +
		"(allow file-write* (subpath \"/private/tmp\"))\n" +
		"(allow file-write* (subpath (param \"HOME\")))\n" +
		"(allow file-write* (subpath (param \"CWD\")))\n" +
		"(deny network*)\n"
	for _, d := range o.capFSRead {
		if abs, err := filepath.Abs(d); err == nil {
			p += "(allow file-read* (subpath " + shellQuote(abs) + "))\n"
		}
	}
	return p
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
