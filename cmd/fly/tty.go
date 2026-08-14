//go:build !windows

// tty.go：终端检测（ioctl TIOCGWINSZ），控制交互式 update 的 sudo 提权与 ANSI 颜色。

package main

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	Row, Col, Xpixel, Ypixel uint16
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	if fi.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&ws)))
	return errno == 0
}
