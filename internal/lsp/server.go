// server.go：LSP 服务器——JSON-RPC 2.0 over stdio，诊断与 fly check 同一编译管线（CheckSource）。
package lsp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"flylang/internal/compile"
)

type Document struct {
	URI  string
	Text string
}

type Server struct {
	mu     sync.Mutex
	docs   map[string]*Document
	timers map[string]*time.Timer
	shutOK bool
	out    chan []byte
}

func New() *Server {
	return &Server{
		docs:   make(map[string]*Document),
		timers: make(map[string]*time.Timer),
	}
}

type request struct {
	ID     json.RawMessage `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params"`
}

func (s *Server) Run(stdin io.Reader, stdout io.Writer) error {
	s.out = make(chan []byte, 128)
	wDone := make(chan error, 1)
	go func() {
		w := bufio.NewWriter(stdout)
		for msg := range s.out {
			if err := writeMessage(w, msg); err != nil {
				wDone <- err
				return
			}
			if err := w.Flush(); err != nil {
				wDone <- err
				return
			}
		}
		wDone <- nil
	}()

	r := bufio.NewReader(stdin)
	for {
		header, err := r.ReadString('\n')
		if err != nil {
			close(s.out)
			<-wDone
			return err
		}
		header = strings.TrimSpace(header)
		if header == "" {
			continue
		}
		if !strings.HasPrefix(header, "Content-Length:") {
			fmt.Fprintf(os.Stderr, "lsp: 未知头 %q\n", header)
			continue
		}
		var n int
		if _, err := fmt.Sscanf(header, "Content-Length: %d", &n); err != nil || n <= 0 || n > 64<<20 {
			fmt.Fprintf(os.Stderr, "lsp: 非法 Content-Length %q\n", header)
			continue
		}
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				close(s.out)
				<-wDone
				return err
			}
			if strings.TrimSpace(line) == "" {
				break
			}
		}
		body := make([]byte, n)
		if _, err := io.ReadFull(r, body); err != nil {
			close(s.out)
			<-wDone
			return err
		}
		var req request
		if err := json.Unmarshal(body, &req); err != nil {
			fmt.Fprintf(os.Stderr, "lsp: JSON 解析失败: %v\n", err)
			continue
		}
		resp := s.dispatch(&req)
		if resp != nil {
			s.out <- resp
		}
		if req.Method == "exit" {
			close(s.out)
			if err := <-wDone; err != nil {
				return err
			}
			if s.shutOK {
				return nil
			}
			return fmt.Errorf("exit 未先 shutdown")
		}
	}
}

func writeMessage(w io.Writer, msg []byte) error {
	if _, err := fmt.Fprintf(w, "Content-Length: %d\r\n\r\n", len(msg)); err != nil {
		return err
	}
	_, err := w.Write(msg)
	return err
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func sError(id json.RawMessage, code int, message string) []byte {
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"error":   rpcError{Code: code, Message: message},
	})
	return msg
}

func sResult(id json.RawMessage, result any) []byte {
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
		"result":  result,
	})
	return msg
}

func sNotify(method string, params any) []byte {
	msg, _ := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	})
	return msg
}

func (s *Server) dispatch(req *request) []byte {
	switch req.Method {
	case "initialize":
		return sResult(req.ID, map[string]any{
			"capabilities": map[string]any{
				"textDocumentSync": map[string]any{
					"openClose": true,
					"change":    1,
					"save":      true,
				},
				"hoverProvider": true,
			},
			"serverInfo": map[string]any{"name": "fly", "version": "1.0.0"},
		})
	case "initialized":
		return nil
	case "shutdown":
		s.shutOK = true
		return sResult(req.ID, nil)
	case "exit":
		return nil
	case "textDocument/didOpen":
		s.didOpen(req.Params)
		return nil
	case "textDocument/didChange":
		s.didChange(req.Params)
		return nil
	case "textDocument/didSave":
		s.didSave(req.Params)
		return nil
	case "textDocument/didClose":
		s.didClose(req.Params)
		return nil
	case "textDocument/hover":
		return sResult(req.ID, s.hover(req.Params))
	case "fly/forceCheck":
		var p struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
		}
		_ = json.Unmarshal(req.Params, &p)
		s.checkURI(p.TextDocument.URI)
		return nil
	}
	return sError(req.ID, -32601, "方法未实现: "+req.Method)
}

func uriToPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return strings.TrimPrefix(uri, "file://")
	}
	return u.Path
}

func (s *Server) didOpen(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI  string `json:"uri"`
			Text string `json:"text"`
		} `json:"textDocument"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	uri := p.TextDocument.URI
	s.mu.Lock()
	s.docs[uri] = &Document{URI: uri, Text: p.TextDocument.Text}
	s.mu.Unlock()
	s.checkURI(uri)
}

func (s *Server) didChange(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		ContentChanges []struct {
			Text string `json:"text"`
		} `json:"contentChanges"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	uri := p.TextDocument.URI
	if len(p.ContentChanges) == 0 {
		return
	}
	text := p.ContentChanges[len(p.ContentChanges)-1].Text
	s.mu.Lock()
	if d, ok := s.docs[uri]; ok {
		d.Text = text
	} else {
		s.docs[uri] = &Document{URI: uri, Text: text}
	}
	s.mu.Unlock()
	s.scheduleCheck(uri)
}

func (s *Server) didSave(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	s.checkURI(p.TextDocument.URI)
}

func (s *Server) didClose(params json.RawMessage) {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
	}
	if json.Unmarshal(params, &p) != nil {
		return
	}
	uri := p.TextDocument.URI
	s.mu.Lock()
	delete(s.docs, uri)
	if t, ok := s.timers[uri]; ok {
		t.Stop()
		delete(s.timers, uri)
	}
	s.mu.Unlock()
	s.publish(uri, nil)
}

const debounceDelay = 120 * time.Millisecond

func (s *Server) scheduleCheck(uri string) {
	s.mu.Lock()
	t := time.AfterFunc(debounceDelay, func() {
		s.checkURI(uri)
	})
	if old, ok := s.timers[uri]; ok {
		old.Stop()
	}
	s.timers[uri] = t
	s.mu.Unlock()
}

func (s *Server) checkURI(uri string) {
	s.mu.Lock()
	d, ok := s.docs[uri]
	s.mu.Unlock()
	if !ok {
		return
	}
	path := uriToPath(uri)
	base := filepath.Base(path)
	errs := compile.CheckSource(d.Text)
	diags := make([]any, 0, len(errs))
	for _, e := range errs {
		line, col := e.Pos.Line, e.Pos.Col
		if line == 0 {
			line = 1
		}
		diags = append(diags, map[string]any{
			"range": map[string]any{
				"start": map[string]any{"line": line - 1, "character": col - 1},
				"end":   map[string]any{"line": line - 1, "character": col},
			},
			"severity": 1,
			"source":   "fly",
			"code":     e.Code,
			"message":  fmt.Sprintf("error[%s]: %s: %s", e.Code, base, e.Msg),
		})
	}
	s.publish(uri, diags)
}

func (s *Server) publish(uri string, diags []any) {
	s.out <- sNotify("textDocument/publishDiagnostics", map[string]any{
		"uri":         uri,
		"diagnostics": diags,
	})
}
