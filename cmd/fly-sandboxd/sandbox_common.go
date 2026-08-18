package main

// 沙箱参数解析（三平台共用）：Linux（namespaces/Landlock/seccomp）、
// macOS（Seatbelt）、Windows（Job Object）共享同一 CLI 面。

import (
	"fmt"
	"os"
	"strconv"
)

type sandboxOpts struct {
	script     string
	capFSRead  []string
	capNetHost []string
	memLimitMB uint64
	timeoutMS  uint64
	cpuSec     uint64
	nofile     uint64
	audit      bool
	debugNS    bool
}

func sandboxUsage() {
	fmt.Fprintln(os.Stderr, `用法: fly sandbox <script.py> [选项]

选项:
  --cap-fs-read <path>    只读访问白名单（可重复，默认仅系统库+脚本）
  --cap-net-host <host>   网络 host 白名单（预留，当前网络一律禁用）
  --mem-limit-mb <n>      内存上限 MB（默认 512）
  --timeout-ms <n>        墙钟超时毫秒（默认 5000）
  --cpu-sec <n>           CPU 时间上限秒（默认 10）
  --nofile <n>            文件描述符上限（默认 64）
  --no-audit              关闭审计日志（默认开启，JSON 行输出到 stderr）
  --debug-ns              跳过命名空间（调试隔离层用）`)
}

func parseSandboxArgs(args []string) (*sandboxOpts, error) {
	o := &sandboxOpts{memLimitMB: 512, timeoutMS: 5000, cpuSec: 10, nofile: 64, audit: true}
	next := func(i *int) (string, error) {
		*i++
		if *i >= len(args) {
			return "", fmt.Errorf("%s 需要参数", args[*i-1])
		}
		return args[*i], nil
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cap-fs-read":
			v, err := next(&i)
			if err != nil {
				return nil, err
			}
			o.capFSRead = append(o.capFSRead, v)
		case "--cap-net-host":
			v, err := next(&i)
			if err != nil {
				return nil, err
			}
			o.capNetHost = append(o.capNetHost, v)
		case "--mem-limit-mb":
			v, err := next(&i)
			if err != nil {
				return nil, err
			}
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("--mem-limit-mb 需要整数")
			}
			o.memLimitMB = n
		case "--timeout-ms":
			v, err := next(&i)
			if err != nil {
				return nil, err
			}
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("--timeout-ms 需要整数")
			}
			o.timeoutMS = n
		case "--cpu-sec":
			v, err := next(&i)
			if err != nil {
				return nil, err
			}
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("--cpu-sec 需要整数")
			}
			o.cpuSec = n
		case "--nofile":
			v, err := next(&i)
			if err != nil {
				return nil, err
			}
			n, err := strconv.ParseUint(v, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("--nofile 需要整数")
			}
			o.nofile = n
		case "--no-audit":
			o.audit = false
		case "--debug-ns":
			o.debugNS = true
		case "-h", "--help":
			sandboxUsage()
			os.Exit(2)
		default:
			if o.script == "" {
				o.script = args[i]
			} else {
				return nil, fmt.Errorf("未知参数 %s", args[i])
			}
		}
	}
	if o.script == "" {
		return nil, fmt.Errorf("缺少 <script.py>")
	}
	return o, nil
}

func sandboxAudit(flag bool, format string, a ...any) {
	if flag {
		fmt.Fprintf(os.Stderr, format+"\n", a...)
	}
}
