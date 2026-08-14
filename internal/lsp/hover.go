// hover.go：LSP hover 支持——8 个安全关键字的中文文档。
package lsp

import (
	"encoding/json"
	"strings"
)

var keywordDocs = map[string]string{
	"safe":  "**safe** · 强制净化污点变量\n\n声明此变量为污点源，必须经净化后才能流入危险操作（eval/exec/os.system/SQL 等）。编译期污点追踪，零运行时残留。",
	"only":  "**only** · 白名单权限块\n\n块内只允许白名单内的模块/函数调用（如 `only (sys):`）。编译期校验 + `__builtins__` 白名单代理。",
	"lock":  "**lock** · 锁定常量\n\n锁定变量为常量：禁止再赋值、AugAssign、setattr 与 `globals()['X']` 反射读取。编译期符号表拦截。",
	"mask":  "**mask** · 遮蔽敏感数据\n\n标记敏感变量（密码/token），禁止流入 print/logging/f-string 等输出上下文。编译期检测。",
	"cage":  "**cage** · 资源约束\n\n`cage(max_time=, max_memory=):` 限制代码块执行时间与内存，超限抛 `TimeoutError`/`ResourceExhaustedError`。运行时 signal/resource。",
	"guard": "**guard** · 强制输入验证\n\n`guard x: type, 条件` 展开为断言，不满足抛 `GuardError`。编译期验证 + 生成断言。",
	"seal":  "**seal** · 冻结对象\n\n`seal class` 冻结类与实例，禁止增删改属性。类体注入 `__setattr__` 拦截。",
	"trace": "**trace** · 审计日志\n\n`trace(level=, args=, ret=)` 在函数入口/出口插入 logging 调用，记录参数与返回值。",
}

func (s *Server) hover(params json.RawMessage) any {
	var p struct {
		TextDocument struct {
			URI string `json:"uri"`
		} `json:"textDocument"`
		Position struct {
			Line      int `json:"line"`
			Character int `json:"character"`
		} `json:"position"`
	}
	if json.Unmarshal(params, &p) != nil {
		return nil
	}
	s.mu.Lock()
	d, ok := s.docs[p.TextDocument.URI]
	s.mu.Unlock()
	if !ok {
		return nil
	}
	lines := strings.Split(d.Text, "\n")
	if p.Position.Line < 0 || p.Position.Line >= len(lines) {
		return nil
	}
	line := lines[p.Position.Line]
	token := tokenAt(line, p.Position.Character)
	if token == "" {
		return nil
	}
	parts := []string{"```fly\n" + line + "\n```"}
	if doc, ok := keywordDocs[token]; ok {
		parts = append(parts, doc)
	}
	return map[string]any{
		"contents": map[string]any{"kind": "markdown", "value": strings.Join(parts, "\n\n---\n\n")},
	}
}

func tokenAt(line string, char int) string {
	if char < 0 || char > len(line) {
		return ""
	}
	start := char
	for start > 0 && isIdentChar(line[start-1]) {
		start--
	}
	end := char
	for end < len(line) && isIdentChar(line[end]) {
		end++
	}
	return line[start:end]
}

func isIdentChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_'
}
