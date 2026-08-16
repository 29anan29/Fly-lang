package main

// fly-sandboxd：`fly sandbox` 的独立守护进程（Go 保留组件，方案 B）。
// Rust CLI 按命令自动发现同目录/FLY_SANDBOXD/PATH，spawn 本进程并透传参数/stdio/退出码。
// 仅 Linux 生效；非 Linux 平台为 stub（报"仅支持 Linux"）。
// stage2 re-exec 时 argv 带 "sandbox" 前缀（对齐 Go 版 main 分发），此处剥离。

import (
	"os"
)

func main() {
	args := os.Args[1:]
	if len(args) > 0 && args[0] == "sandbox" {
		args = args[1:]
	}
	os.Exit(cmdSandbox(args))
}
