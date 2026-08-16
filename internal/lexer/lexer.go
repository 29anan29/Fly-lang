// lexer.go：词法分析主实现——缩进栈、字符串前缀、数字/运算符扫描，产出 Token 流。
package lexer

import (
	"fmt"
	"strings"

	"pyfly/internal/ast"
)

type Lexer struct {
	src         []byte
	off         int
	line        int
	col         int
	depth       int
	indents     []int
	atLineStart bool
	lineTok     bool
	pending     []Token
	err         *ast.Diagnostic
}

func New(src string) *Lexer {
	return &Lexer{
		src:         []byte(src),
		line:        1,
		col:         1,
		indents:     []int{0},
		atLineStart: true,
	}
}

func (l *Lexer) pos() ast.Position {
	return ast.Position{Line: l.line, Col: l.col}
}

func (l *Lexer) Err() *ast.Diagnostic {
	return l.err
}

func (l *Lexer) errorf(p ast.Position, format string, args ...interface{}) {
	if l.err == nil {
		l.err = &ast.Diagnostic{Pos: p, Code: ast.CodeForFormat(format), Msg: fmt.Sprintf(format, args...)}
	}
}

func (l *Lexer) advance() byte {
	c := l.src[l.off]
	l.off++
	if c == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	return c
}

func (l *Lexer) peek(off int) byte {
	if l.off+off < len(l.src) {
		return l.src[l.off+off]
	}
	return 0
}

func (l *Lexer) skipLine() {
	for l.off < len(l.src) && l.src[l.off] != '\n' {
		l.advance()
	}
}

func (l *Lexer) Next() Token {
	if l.err != nil {
		return Token{Type: EOF, Pos: l.pos()}
	}
	if len(l.pending) > 0 {
		t := l.pending[0]
		l.pending = l.pending[1:]
		return t
	}
	for {
		if l.atLineStart {
			if t, ok := l.lineStart(); ok {
				return t
			}
		}
		if l.off >= len(l.src) {
			if l.lineTok {
				l.lineTok = false
				return Token{Type: NEWLINE, Pos: l.pos()}
			}
			if l.depth > 0 {
				l.errorf(l.pos(), "未闭合的括号")
			}
			for len(l.indents) > 1 {
				l.indents = l.indents[:len(l.indents)-1]
				l.pending = append(l.pending, Token{Type: DEDENT, Pos: l.pos()})
			}
			if len(l.pending) > 0 {
				t := l.pending[0]
				l.pending = l.pending[1:]
				return t
			}
			return Token{Type: EOF, Pos: l.pos()}
		}
		c := l.src[l.off]
		switch {
		case c == ' ' || c == '\r' || c == '\t':
			l.advance()
		case c == '\n':
			if l.depth > 0 {
				l.advance()
			} else {
				pos := l.pos()
				l.advance()
				l.atLineStart = true
				if l.lineTok {
					l.lineTok = false
					return Token{Type: NEWLINE, Pos: pos}
				}
			}
		case c == '#':
			l.skipLine()
		case c == '\\' && l.peek(1) == '\n':
			l.advance()
			l.advance()
		default:
			t := l.lexToken()
			if t.Type != ILLEGAL {
				l.lineTok = true
			}
			return t
		}
	}
}

func (l *Lexer) lineStart() (Token, bool) {
	for {
		n := 0
		for l.off < len(l.src) {
			c := l.src[l.off]
			if c == ' ' {
				n++
				l.advance()
				continue
			}
			if c == '\t' {
				l.errorf(l.pos(), "行首不支持制表符缩进")
				l.atLineStart = false
				return Token{Type: ILLEGAL, Pos: l.pos()}, true
			}
			break
		}
		if l.off >= len(l.src) {
			l.atLineStart = false
			for len(l.indents) > 1 {
				l.indents = l.indents[:len(l.indents)-1]
				l.pending = append(l.pending, Token{Type: DEDENT, Pos: l.pos()})
			}
			if len(l.pending) > 0 {
				t := l.pending[0]
				l.pending = l.pending[1:]
				return t, true
			}
			return Token{Type: EOF, Pos: l.pos()}, true
		}
		c := l.src[l.off]
		if c == '\n' {
			l.advance()
			continue
		}
		if c == '#' {
			l.skipLine()
			continue
		}
		if c == '\\' && l.peek(1) == '\n' {
			l.advance()
			l.advance()
			continue
		}
		top := l.indents[len(l.indents)-1]
		switch {
		case n > top:
			l.indents = append(l.indents, n)
			l.pending = append(l.pending, Token{Type: INDENT, Pos: l.pos()})
		case n < top:
			for len(l.indents) > 0 && l.indents[len(l.indents)-1] > n {
				l.indents = l.indents[:len(l.indents)-1]
				l.pending = append(l.pending, Token{Type: DEDENT, Pos: l.pos()})
			}
			if l.indents[len(l.indents)-1] != n {
				l.errorf(l.pos(), "缩进级别与上层不一致")
				l.atLineStart = false
				return Token{Type: ILLEGAL, Pos: l.pos()}, true
			}
		}
		l.atLineStart = false
		if len(l.pending) > 0 {
			t := l.pending[0]
			l.pending = l.pending[1:]
			return t, true
		}
		return Token{}, false
	}
}

func isIdentStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isIdentPart(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func (l *Lexer) lexToken() Token {
	c := l.src[l.off]
	switch {
	case (c == 'f' || c == 'F' || c == 'r' || c == 'R' || c == 'b' || c == 'B' || c == 'u' || c == 'U') && l.isStringPrefix():
		return l.scanStringPrefix()

	case isIdentStart(c):
		start := l.pos()
		off := l.off
		for l.off < len(l.src) && isIdentPart(l.src[l.off]) {
			l.advance()
		}
		name := string(l.src[off:l.off])
		if tt, ok := keywords[name]; ok {
			return Token{Type: tt, Lit: name, Pos: start}
		}
		return Token{Type: IDENT, Lit: name, Pos: start}

	case c >= '0' && c <= '9':
		return l.scanNumber()

	case c == '\'' || c == '"':
		return l.scanString("")

	case c == '.' && l.peek(1) == '.' && l.peek(2) == '.':
		l.advance()
		l.advance()
		l.advance()
		return Token{Type: ELLIPSIS, Lit: "...", Pos: l.pos()}

	case c == '.' && l.peek(1) >= '0' && l.peek(1) <= '9':
		return l.scanNumber()

	default:
		return l.scanOp()
	}
}

func (l *Lexer) isStringPrefix() bool {
	i := l.off
	var seen [4]bool
	idx := map[byte]int{'f': 0, 'r': 1, 'b': 2, 'u': 3}
	for i < len(l.src) {
		ch := l.src[i]
		if ch == '\'' || ch == '"' {
			return true
		}
		if i-l.off >= 2 {
			return false
		}
		low := ch
		if ch >= 'A' && ch <= 'Z' {
			low = ch + 32
		}
		j, ok := idx[low]
		if !ok {
			return false
		}
		if seen[j] {
			return false
		}
		seen[j] = true
		i++
	}
	return false
}

func (l *Lexer) scanStringPrefix() Token {
	prefix := ""
	for l.off < len(l.src) && isIdentPart(l.src[l.off]) {
		prefix += string(l.advance())
	}
	return l.scanString(strings.ToLower(prefix))
}

func (l *Lexer) scanString(prefix string) Token {
	start := l.pos()
	q := l.advance()
	triple := l.peek(0) == q && l.peek(1) == q
	if triple {
		l.advance()
		l.advance()
	}
	begin := l.off
	for l.off < len(l.src) {
		c := l.src[l.off]
		if c == '\\' {
			l.advance()
			if l.off < len(l.src) {
				l.advance()
			}
			continue
		}
		if c == q {
			if triple {
				if l.peek(1) == q && l.peek(2) == q {
					lit := string(l.src[begin:l.off])
					l.advance()
					l.advance()
					l.advance()
					return Token{Type: STRING, Lit: prefix + string(q) + string(q) + string(q) + lit + string(q) + string(q) + string(q), Pos: start}
				}
				l.advance()
				continue
			}
			lit := string(l.src[begin:l.off])
			l.advance()
			return Token{Type: STRING, Lit: prefix + string(q) + lit + string(q), Pos: start}
		}
		if c == '\n' && !triple {
			l.errorf(start, "字符串未闭合")
			return Token{Type: ILLEGAL, Lit: "", Pos: start}
		}
		l.advance()
	}
	l.errorf(start, "字符串未闭合")
	return Token{Type: ILLEGAL, Lit: "", Pos: start}
}

func (l *Lexer) scanNumber() Token {
	start := l.pos()
	off := l.off
	isFloat := false
	if l.peek(0) == '0' && (l.peek(1) == 'x' || l.peek(1) == 'X' || l.peek(1) == 'o' || l.peek(1) == 'O' || l.peek(1) == 'b' || l.peek(1) == 'B') {
		l.advance()
		l.advance()
		for l.off < len(l.src) {
			c := l.src[l.off]
			if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '_' {
				l.advance()
				continue
			}
			break
		}
	} else {
		for l.off < len(l.src) {
			c := l.src[l.off]
			if (c >= '0' && c <= '9') || c == '_' {
				l.advance()
				continue
			}
			break
		}
		if l.peek(0) == '.' {
			isFloat = true
			l.advance()
			for l.off < len(l.src) {
				c := l.src[l.off]
				if (c >= '0' && c <= '9') || c == '_' {
					l.advance()
					continue
				}
				break
			}
		}
		if l.peek(0) == 'e' || l.peek(0) == 'E' {
			seen := l.advance()
			isFloat = true
			if l.peek(0) == '+' || l.peek(0) == '-' {
				l.advance()
			}
			digits := 0
			for l.off < len(l.src) {
				c := l.src[l.off]
				if (c >= '0' && c <= '9') || c == '_' {
					l.advance()
					digits++
					continue
				}
				break
			}
			if digits == 0 {
				l.errorf(start, "指数部分缺少数字: %s", string(seen))
				return Token{Type: ILLEGAL, Lit: "", Pos: start}
			}
		}
	}
	lit := string(l.src[off:l.off])
	if l.off < len(l.src) && isIdentPart(l.src[l.off]) {
		l.errorf(start, "非法数字字面量 %s", lit)
		return Token{Type: ILLEGAL, Lit: "", Pos: start}
	}
	if isFloat {
		return Token{Type: FLOAT, Lit: lit, Pos: start}
	}
	return Token{Type: INT, Lit: lit, Pos: start}
}

func (l *Lexer) scanOp() Token {
	start := l.pos()
	advance := func(n int) string {
		s := string(l.src[l.off : l.off+n])
		for i := 0; i < n; i++ {
			l.advance()
		}
		return s
	}
	two := func(t TokenType, lit string) Token {
		return Token{Type: t, Lit: lit, Pos: start}
	}
	switch l.src[l.off] {
	case '+':
		if l.peek(1) == '=' {
			return two(PLUS_ASSIGN, advance(2))
		}
		return two(PLUS, advance(1))
	case '-':
		if l.peek(1) == '=' {
			return two(MINUS_ASSIGN, advance(2))
		}
		if l.peek(1) == '>' {
			return two(ARROW, advance(2))
		}
		return two(MINUS, advance(1))
	case '*':
		if l.peek(1) == '*' && l.peek(2) == '=' {
			return two(DOUBLESTAR_ASSIGN, advance(3))
		}
		if l.peek(1) == '*' {
			return two(DOUBLESTAR, advance(2))
		}
		if l.peek(1) == '=' {
			return two(STAR_ASSIGN, advance(2))
		}
		return two(STAR, advance(1))
	case '/':
		if l.peek(1) == '/' && l.peek(2) == '=' {
			return two(FLOORDIV_ASSIGN, advance(3))
		}
		if l.peek(1) == '/' {
			return two(FLOORDIV, advance(2))
		}
		if l.peek(1) == '=' {
			return two(SLASH_ASSIGN, advance(2))
		}
		return two(SLASH, advance(1))
	case '%':
		if l.peek(1) == '=' {
			return two(PERCENT_ASSIGN, advance(2))
		}
		return two(PERCENT, advance(1))
	case '<':
		if l.peek(1) == '<' && l.peek(2) == '=' {
			return two(SHL_ASSIGN, advance(3))
		}
		if l.peek(1) == '<' {
			return two(SHL, advance(2))
		}
		if l.peek(1) == '=' {
			return two(LE, advance(2))
		}
		return two(LT, advance(1))
	case '>':
		if l.peek(1) == '>' && l.peek(2) == '=' {
			return two(SHR_ASSIGN, advance(3))
		}
		if l.peek(1) == '>' {
			return two(SHR, advance(2))
		}
		if l.peek(1) == '=' {
			return two(GE, advance(2))
		}
		return two(GT, advance(1))
	case '!':
		if l.peek(1) == '=' {
			return two(NE, advance(2))
		}
		l.errorf(start, "意外的字符 %q", '!')
		return Token{Type: ILLEGAL, Lit: "", Pos: start}
	case '&':
		if l.peek(1) == '=' {
			return two(AMP_ASSIGN, advance(2))
		}
		return two(AMP, advance(1))
	case '|':
		if l.peek(1) == '=' {
			return two(PIPE_ASSIGN, advance(2))
		}
		return two(PIPE, advance(1))
	case '^':
		if l.peek(1) == '=' {
			return two(CARET_ASSIGN, advance(2))
		}
		return two(CARET, advance(1))
	case '~':
		return two(TILDE, advance(1))
	case '=':
		if l.peek(1) == '=' {
			return two(EQEQ, advance(2))
		}
		return two(ASSIGN, advance(1))
	case ':':
		return two(COLON, advance(1))
	case ',':
		return two(COMMA, advance(1))
	case '.':
		return two(DOT, advance(1))
	case ';':
		return two(SEMICOLON, advance(1))
	case '@':
		return two(AT, advance(1))
	case '(':
		l.depth++
		return two(LPAREN, advance(1))
	case ')':
		l.depth--
		if l.depth < 0 {
			l.errorf(start, "多余的右括号 ')'")
		}
		return two(RPAREN, advance(1))
	case '[':
		l.depth++
		return two(LBRACKET, advance(1))
	case ']':
		l.depth--
		if l.depth < 0 {
			l.errorf(start, "多余的右括号 ']'")
		}
		return two(RBRACKET, advance(1))
	case '{':
		l.depth++
		return two(LBRACE, advance(1))
	case '}':
		l.depth--
		if l.depth < 0 {
			l.errorf(start, "多余的右括号 '}'")
		}
		return two(RBRACE, advance(1))
	}
	l.errorf(start, "意外的字符 %q", l.src[l.off])
	return Token{Type: ILLEGAL, Lit: "", Pos: start}
}
