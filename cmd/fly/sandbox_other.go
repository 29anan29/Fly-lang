//go:build !linux

package main

import (
	"fmt"
	"os"
)

// cmdSandbox: 非 Linux 平台不支持（namespaces/Landlock/seccomp 均为 Linux 特性）。
// 与 cmd/fly/sandbox.go（//go:build linux）同签名，保证 main.go 双平台可编译。
func cmdSandbox(args []string) int {
	fmt.Fprintln(os.Stderr, "fly sandbox 仅支持 Linux（x86_64）")
	return 2
}
