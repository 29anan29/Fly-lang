// server_test.go：LSP 协议单测（帧解析/请求分发）。
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type msg struct {
	Method string          `json:"method"`
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	Params json.RawMessage `json:"params"`
}

func readMsg(r *bufio.Reader) (msg, error) {
	var m msg
	for {
		header, err := r.ReadString('\n')
		if err != nil {
			return m, err
		}
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		var n int
		if _, err := fmt.Sscanf(header, "Content-Length: %d", &n); err != nil {
			continue
		}
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return m, err
			}
			if strings.TrimSpace(line) == "" {
				break
			}
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			return m, err
		}
		if err := json.Unmarshal(body, &m); err != nil {
			return m, err
		}
		return m, nil
	}
}

func sendMsg(w io.Writer, m any) {
	b, _ := json.Marshal(m)
	fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(b))
	w.Write(b)
}

func startServer(t *testing.T) (*bufio.Reader, io.WriteCloser, func()) {
	t.Helper()
	serverIn, clientIn := io.Pipe()
	clientOut, serverOut := io.Pipe()
	done := make(chan error, 1)
	s := New()
	go func() {
		done <- s.Run(serverIn, serverOut)
	}()
	cleanup := func() {
		serverIn.Close()
		serverOut.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Fatal("服务器未退出")
		}
	}
	return bufio.NewReader(clientOut), clientIn, cleanup
}

func TestInitializeAndOpen(t *testing.T) {
	r, w, cleanup := startServer(t)
	defer cleanup()

	sendMsg(w, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	m, err := readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID == nil || m.Error != nil {
		t.Fatalf("initialize 响应异常: %+v", m)
	}
	var result struct {
		Capabilities map[string]any `json:"capabilities"`
	}
	json.Unmarshal(m.Result, &result)
	if result.Capabilities["hoverProvider"] != true {
		t.Fatalf("缺 hoverProvider: %+v", result.Capabilities)
	}
	sendMsg(w, map[string]any{"jsonrpc": "2.0", "method": "initialized", "params": map[string]any{}})

	const bad = "def main():\n    print(hello\n"
	sendMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": "file:///tmp/err.fly", "languageId": "fly", "version": 1, "text": bad},
		},
	})
	m, err = readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("期望 publishDiagnostics，得到 %+v", m)
	}
	var pub struct {
		URI         string `json:"uri"`
		Diagnostics []struct {
			Range struct {
				Start struct {
					Line      int `json:"line"`
					Character int `json:"character"`
				} `json:"start"`
			} `json:"range"`
			Severity int    `json:"severity"`
			Source   string `json:"source"`
			Message  string `json:"message"`
		} `json:"diagnostics"`
	}
	json.Unmarshal(m.Params, &pub)
	if pub.URI != "file:///tmp/err.fly" || len(pub.Diagnostics) != 1 {
		t.Fatalf("诊断内容异常: %+v", pub)
	}
	d := pub.Diagnostics[0]
	if d.Severity != 1 || d.Source != "fly" {
		t.Fatalf("诊断元数据异常: %+v", d)
	}
	if !strings.Contains(d.Message, "err.fly") {
		t.Fatalf("诊断消息应含文件名: %q", d.Message)
	}
}

func TestDiagnosticsClearedOnClose(t *testing.T) {
	r, w, cleanup := startServer(t)
	defer cleanup()

	sendMsg(w, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	readMsg(r)

	uri := "file:///tmp/ok.fly"
	sendMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params":  map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "fly", "version": 1, "text": "x = 1\n"}},
	})
	m, err := readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("期望诊断通知: %+v", m)
	}
	var pub struct {
		Diagnostics []any `json:"diagnostics"`
	}
	json.Unmarshal(m.Params, &pub)
	if len(pub.Diagnostics) != 0 {
		t.Fatalf("合法源码不应有诊断: %+v", pub)
	}

	sendMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didClose",
		"params":  map[string]any{"textDocument": map[string]any{"uri": uri}},
	})
	m, err = readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "textDocument/publishDiagnostics" {
		t.Fatalf("didClose 应清空诊断: %+v", m)
	}
	json.Unmarshal(m.Params, &pub)
	if len(pub.Diagnostics) != 0 {
		t.Fatalf("关闭后诊断应为空: %+v", pub)
	}
}

func TestHoverKeyword(t *testing.T) {
	r, w, cleanup := startServer(t)
	defer cleanup()

	sendMsg(w, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	readMsg(r)

	uri := "file:///tmp/h.fly"
	sendMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"method":  "textDocument/didOpen",
		"params":  map[string]any{"textDocument": map[string]any{"uri": uri, "languageId": "fly", "version": 1, "text": "lock SECRET = \"x\"\n"}},
	})
	readMsg(r)

	sendMsg(w, map[string]any{
		"jsonrpc": "2.0",
		"id":      7,
		"method":  "textDocument/hover",
		"params": map[string]any{
			"textDocument": map[string]any{"uri": uri},
			"position":     map[string]any{"line": 0, "character": 1},
		},
	})
	m, err := readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Error != nil {
		t.Fatalf("hover 报错: %+v", m.Error)
	}
	var result struct {
		Contents struct {
			Value string `json:"value"`
		} `json:"contents"`
	}
	json.Unmarshal(m.Result, &result)
	if !strings.Contains(result.Contents.Value, "**lock**") || !strings.Contains(result.Contents.Value, "```fly") {
		t.Fatalf("hover 内容异常: %q", result.Contents.Value)
	}
}

func TestShutdownAndExit(t *testing.T) {
	r, w, cleanup := startServer(t)
	defer cleanup()

	sendMsg(w, map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	readMsg(r)
	sendMsg(w, map[string]any{"jsonrpc": "2.0", "id": 2, "method": "shutdown", "params": map[string]any{}})
	m, err := readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Error != nil {
		t.Fatalf("shutdown 报错: %+v", m.Error)
	}
	sendMsg(w, map[string]any{"jsonrpc": "2.0", "method": "exit", "params": map[string]any{}})
	time.Sleep(100 * time.Millisecond)
}

func TestUnknownMethod(t *testing.T) {
	r, w, cleanup := startServer(t)
	defer cleanup()

	sendMsg(w, map[string]any{"jsonrpc": "2.0", "id": 9, "method": "textDocument/nope", "params": map[string]any{}})
	m, err := readMsg(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Error == nil || m.Error.Code != -32601 {
		t.Fatalf("期望 MethodNotFound: %+v", m.Error)
	}
}
