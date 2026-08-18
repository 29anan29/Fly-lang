//go:build !linux && !darwin && !windows

package main

import (
	"fmt"
	"os"
)

// cmdSandbox: 其他平台不支持（Linux=namespaces/Landlock/seccomp、
// macOS=Seatbelt、Windows=Job Object 之外无既有机制）。
// 与各平台实现同签名，保证 main.go 跨平台可编译。
func cmdSandbox(args []string) int {
	fmt.Fprintln(os.Stderr, "fly sandbox 仅支持 Linux/macOS/Windows")
	return 2
}
