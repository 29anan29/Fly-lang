// format.go：fly fmt 的代码格式化器——token 流级空白重排。
// 不改变 token 序列（语义安全），保留注释与三引号字符串原文；
// 调用方须先确保 check 通过（语法错误文件拒绝格式化）。
package format

import (
	"strings"

	"flylang/internal/lexer"
)

type line struct {
	num      int
	text     string
	tokens   []lexer.Token
	raw      bool // 跨行字符串/非法行：整体原样输出
	comment  string
	blank    bool
	firstTok int // 行首 token 在 tokens 全集的索引（用于缩进判断）
}

// Format 返回格式化后的源码。token 序列不变，仅调整空白与空行。
func Format(src string) string {
	toks := lexAll(src)
	if len(toks) == 0 {
		return strings.TrimRight(src, "\n")
	}
	lines := buildLines(src, toks)
	var out strings.Builder
	prevBlank := false
	for i, ln := range lines {
		if ln.blank {
			if i == 0 || prevBlank || i == len(lines)-1 {
				continue
			}
			out.WriteString("\n")
			prevBlank = true
			continue
		}
		prevBlank = false
		if i > 0 {
			out.WriteString("\n")
		}
		if ln.raw {
			out.WriteString(strings.TrimRight(ln.text, " \t"))
		} else {
			out.WriteString(renderLine(&ln))
		}
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

func lexAll(src string) []lexer.Token {
	lx := lexer.New(src)
	var toks []lexer.Token
	for {
		t := lx.Next()
		toks = append(toks, t)
		if t.Type == lexer.EOF {
			break
		}
	}
	return toks
}

// buildLines 按行分组 token，并识别注释/跨行字符串/空行。
func buildLines(src string, toks []lexer.Token) []line {
	srcLines := strings.Split(src, "\n")
	lines := make([]line, len(srcLines))
	for i := range srcLines {
		lines[i] = line{num: i + 1, text: srcLines[i]}
	}
	// 按 token 归属行分组；ILLEGAL 行标记 raw；跨行字符串行标记 raw。
	var cur *line
	for _, t := range toks {
		switch t.Type {
		case lexer.NEWLINE, lexer.INDENT, lexer.DEDENT, lexer.EOF:
			continue // 行结构由源码行重建，缩进 token 不参与空白重排
		}
		idx := t.Pos.Line - 1
		if idx < 0 || idx >= len(lines) {
			continue
		}
		cur = &lines[idx]
		if t.Type == lexer.ILLEGAL {
			cur.raw = true
			continue
		}
		if t.Type == lexer.STRING {
			end := t.Pos.Line - 1 + strings.Count(t.Lit, "\n")
			if end > t.Pos.Line-1 {
				for j := t.Pos.Line - 1; j <= end && j < len(lines); j++ {
					lines[j].raw = true
				}
			}
		}
		cur.tokens = append(cur.tokens, t)
	}
	// 注释与空行识别：行内 token 序列结束列之后的 # 内容为注释。
	for i := range lines {
		ln := &lines[i]
		if ln.raw {
			continue
		}
		if len(ln.tokens) == 0 {
			if strings.TrimSpace(ln.text) == "" {
				ln.blank = true
			} else {
				ln.raw = true // 无 token 但非空（应为注释行，走注释提取）
			}
		}
		// 注释：行内最后一个 token 结束列之后的 '#' 起（列越界则跳过）。
		if len(ln.tokens) > 0 {
			last := ln.tokens[len(ln.tokens)-1]
			endCol := last.Pos.Col + len(last.Lit)
			if endCol > 1 && endCol-1 <= len(ln.text) {
				if ci := strings.Index(ln.text[endCol-1:], "#"); ci >= 0 {
					ln.comment = ln.text[endCol-1+ci:]
				}
			}
		} else if strings.TrimSpace(ln.text) != "" {
			if strings.HasPrefix(strings.TrimSpace(ln.text), "#") {
				ln.comment = strings.TrimSpace(ln.text)
			}
		}
	}
	return lines
}

// renderLine 重排行内 token 空白并附加注释。
func renderLine(ln *line) string {
	var b strings.Builder
	// 缩进：保留源码行首空白（tab → 4 空格）。
	lead := ""
	trimmed := strings.TrimLeft(ln.text, " \t")
	leadLen := len(ln.text) - len(trimmed)
	if leadLen > 0 {
		lead = strings.ReplaceAll(ln.text[:leadLen], "\t", "    ")
	}
	b.WriteString(lead)
	depth := 0   // 括号总深度
	bracket := 0 // 方括号深度（切片冒号判断）
	var prev lexer.Token
	prev.Type = lexer.ILLEGAL
	prevUnary := false // prev 是否为一元实例（-x、f(*x)、**kw）
	for i, t := range ln.tokens {
		space := decideSpace(prev, prevUnary, t, depth, bracket, i == 0)
		if space && b.Len() > len(lead) {
			b.WriteString(" ")
		}
		b.WriteString(t.Lit)
		// 一元实例判定：前 token 不是表达式结尾（名字/字面量/闭括号/点）；
		// 非一元候选 token 必须重置，避免标志残留（~a & b 中 & 后需空格）。
		switch t.Type {
		case lexer.MINUS, lexer.PLUS, lexer.TILDE, lexer.STAR, lexer.DOUBLESTAR:
			prevUnary = !(isName(prev.Type) || isLit(prev.Type) || isClose(prev.Type) || prev.Type == lexer.DOT)
		default:
			prevUnary = false
		}
		prev = t
		switch t.Type {
		case lexer.LPAREN, lexer.LBRACKET, lexer.LBRACE:
			depth++
			if t.Type == lexer.LBRACKET {
				bracket++
			}
		case lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE:
			depth--
			if t.Type == lexer.RBRACKET && bracket > 0 {
				bracket--
			}
		}
	}
	if ln.comment != "" {
		if b.Len() > 0 {
			b.WriteString(" ")
		}
		b.WriteString(strings.TrimRight(ln.comment, " \t"))
	}
	return strings.TrimRight(b.String(), " \t")
}

// decideSpace 决定 cur 前是否需要一个空格。
func decideSpace(prev lexer.Token, prevUnary bool, cur lexer.Token, depth, bracket int, atLineStart bool) bool {
	if atLineStart {
		return false
	}
	pt, ct := prev.Type, cur.Type
	// 闭括号前无空格。
	if isClose(ct) {
		return false
	}
	// 开括号前：紧跟名字无空格（函数调用）；fly 块关键字（cage/trace/only）后无空格；
	// 语言关键字后保留空格（if (、while (）。
	if isOpen(ct) {
		switch pt {
		case lexer.IDENT, lexer.CAGE, lexer.TRACE, lexer.ONLY:
			return false
		case lexer.RPAREN, lexer.RBRACKET, lexer.RBRACE, lexer.LPAREN, lexer.LBRACKET, lexer.LBRACE:
			return false
		case lexer.INT, lexer.FLOAT, lexer.STRING, lexer.NONE, lexer.TRUE, lexer.FALSE:
			return false
		}
		return true
	}
	// 名字/字面量前：开括号/点 → 无；冒号 → 切片内无其余有；
	// kwarg 等号（f(x=1)）→ 无；一元运算符实例（-x）→ 无；其余有。
	if isName(ct) || isLit(ct) {
		if isOpen(pt) || pt == lexer.DOT {
			return false
		}
		if pt == lexer.COLON {
			return bracket == 0
		}
		if pt == lexer.ASSIGN && depth > 0 {
			return false
		}
		if prevUnary {
			return false
		}
		return true
	}
	switch ct {
	case lexer.DOT:
		return false
	case lexer.COMMA:
		return false
	case lexer.COLON:
		// 切片冒号两侧无空格；其余冒号后空格由下一个 token 决定。
		return false
	case lexer.ASSIGN:
		// call 参数 f(x=1)：括号内无空格；其余两侧空格。
		return depth == 0
	case lexer.PLUS_ASSIGN, lexer.MINUS_ASSIGN, lexer.STAR_ASSIGN, lexer.SLASH_ASSIGN,
		lexer.PERCENT_ASSIGN, lexer.DOUBLESTAR_ASSIGN, lexer.FLOORDIV_ASSIGN,
		lexer.SHL_ASSIGN, lexer.SHR_ASSIGN, lexer.AMP_ASSIGN, lexer.PIPE_ASSIGN,
		lexer.CARET_ASSIGN:
		return true
	case lexer.MINUS, lexer.PLUS, lexer.TILDE:
		// 负号/正号：紧贴开括号/逗号/冒号后（f(-1)、a[1:-1]、f(1, -2)）；
		// 其余情况有空格（x = -5、a - -b）。
		return !(isOpen(pt) || pt == lexer.COMMA || pt == lexer.COLON)
	case lexer.STAR, lexer.DOUBLESTAR:
		// 解包 star 紧贴开括号/逗号（f(*x)）；二元乘法两侧空格（a * b）。
		return !(isOpen(pt) || pt == lexer.COMMA || pt == lexer.COLON)
	case lexer.SLASH, lexer.PERCENT, lexer.FLOORDIV, lexer.SHL, lexer.SHR,
		lexer.AMP, lexer.PIPE, lexer.CARET, lexer.LT, lexer.GT, lexer.LE,
		lexer.GE, lexer.EQEQ, lexer.NE, lexer.AND, lexer.OR:
		return true
	case lexer.IS, lexer.IN:
		return pt != lexer.NOT // a not in b：NOT 与 IN 之间无空格
	case lexer.NOT:
		return true
	}
	return true
}

func isOpen(t lexer.TokenType) bool {
	return t == lexer.LPAREN || t == lexer.LBRACKET || t == lexer.LBRACE
}

func isClose(t lexer.TokenType) bool {
	return t == lexer.RPAREN || t == lexer.RBRACKET || t == lexer.RBRACE
}

func isName(t lexer.TokenType) bool {
	return t == lexer.IDENT
}

func isLit(t lexer.TokenType) bool {
	switch t {
	case lexer.INT, lexer.FLOAT, lexer.STRING, lexer.NONE, lexer.TRUE, lexer.FALSE:
		return true
	}
	return false
}

// PositionOf 供 analyze 复用：返回源码行注释行号集合（供注释比例统计）。
func CommentLines(src string) []int {
	toks := lexAll(src)
	lines := buildLines(src, toks)
	var out []int
	for _, ln := range lines {
		if ln.comment != "" {
			out = append(out, ln.num)
		}
	}
	return out
}
