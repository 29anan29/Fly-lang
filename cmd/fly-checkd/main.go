package main

// fly-checkd：Rust CLI 的编译检查守护进程（stdio 二进制帧协议）。
// 请求帧: [4B BE len][payload]
//   payload: [1B color][4B BE path_len][path][4B BE src_len][src]
// 响应帧: [4B BE len][payload]
//   payload: [1B 0x00=ok] 后跟 N 条诊断
//            [1B 0x01=err] 后跟 [4B BE msg_len][msg]（checkd 内部错误）
//   每条: [4B BE code_len][code][4B LE line][4B LE col][4B BE msg_len][msg]
// 客户端写满请求后关闭 stdin，EOF 即退出 0。
// 编译管线与 fly check 相同（parser+checker），color 由客户端 TTY 判定传入。

import (
	"bufio"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"flylang/internal/ast"
	"flylang/internal/compile"
)

func main() {
	if err := serve(); err != nil {
		fmt.Fprintf(os.Stderr, "checkd: %v\n", err)
		os.Exit(1)
	}
}

func serve() error {
	in := bufio.NewReader(os.Stdin)
	for {
		payload, err := readFrame(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		resp := handle(payload)
		if err := writeFrame(os.Stdout, resp); err != nil {
			return err
		}
	}
}

func handle(payload []byte) []byte {
	color, _, src, err := decodeRequest(payload)
	if err != nil {
		return encodeError(err.Error())
	}
	diags := compile.CheckSource(src)
	return encodeDiagnostics(diags, color)
}

func encodeError(msg string) []byte {
	var b []byte
	b = append(b, 0x01)
	b = append(b, uint32Bytes(len(msg))...)
	b = append(b, msg...)
	return b
}

func encodeDiagnostics(diags []ast.Diagnostic, color byte) []byte {
	var b []byte
	b = append(b, 0x00)
	b = append(b, byte(len(diags)))
	for _, d := range diags {
		b = append(b, uint32Bytes(len(d.Code))...)
		b = append(b, d.Code...)
		var line, col [4]byte
		binary.LittleEndian.PutUint32(line[:], uint32(d.Pos.Line))
		binary.LittleEndian.PutUint32(col[:], uint32(d.Pos.Col))
		b = append(b, line[:]...)
		b = append(b, col[:]...)
		b = append(b, uint32Bytes(len(d.Msg))...)
		b = append(b, d.Msg...)
	}
	_ = color
	return b
}

func decodeRequest(payload []byte) (color byte, path, src string, err error) {
	if len(payload) < 1+4+4 {
		return 0, "", "", errors.New("请求过短")
	}
	color = payload[0]
	pathLen := int(binary.BigEndian.Uint32(payload[1:5]))
	rest := payload[5:]
	if len(rest) < pathLen+4 {
		return 0, "", "", errors.New("请求 path 长度不符")
	}
	path = string(rest[:pathLen])
	rest = rest[pathLen:]
	srcLen := int(binary.BigEndian.Uint32(rest[:4]))
	rest = rest[4:]
	if len(rest) < srcLen {
		return 0, "", "", errors.New("请求 src 长度不符")
	}
	src = string(rest[:srcLen])
	return color, path, src, nil
}

func uint32Bytes(n int) []byte {
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], uint32(n))
	return b[:]
}

func readFrame(r *bufio.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > 64<<20 {
		return nil, errors.New("帧过大")
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, err
	}
	return buf, nil
}

func writeFrame(w io.Writer, payload []byte) error {
	if _, err := w.Write(uint32Bytes(len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
