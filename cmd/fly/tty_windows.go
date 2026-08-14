//go:build windows

// tty_windows.go：Windows 版终端检测（FileMode 判定），与 tty.go 按构建标签二选一。

package main

import "os"

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
