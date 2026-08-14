// lexer_test.go：词法单测（token 序列断言）。
package lexer

import (
	"testing"

	"flylang/internal/ast"
)

func tokenize(t *testing.T, src string) []Token {
	t.Helper()
	l := New(src)
	var toks []Token
	for {
		tok := l.Next()
		toks = append(toks, tok)
		if tok.Type == EOF {
			break
		}
	}
	return toks
}

func types(toks []Token) []TokenType {
	var ts []TokenType
	for _, t := range toks {
		ts = append(ts, t.Type)
	}
	return ts
}

func assertTypes(t *testing.T, src string, want ...TokenType) {
	t.Helper()
	got := types(tokenize(t, src))
	if len(got) != len(want) {
		t.Fatalf("token 数量不匹配: %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d 类型不匹配: %v, want %v", i, got, want)
		}
	}
}

func TestKeywords(t *testing.T) {
	assertTypes(t, "safe only lock mask cage guard seal trace",
		SAFE, ONLY, LOCK, MASK, CAGE, GUARD, SEAL, TRACE, NEWLINE, EOF)
}

func TestIndentDedent(t *testing.T) {
	src := "if x:\n    y = 1\n    z = 2\nw = 3\n"
	assertTypes(t, src,
		IF, IDENT, COLON, NEWLINE,
		INDENT, IDENT, ASSIGN, INT, NEWLINE,
		IDENT, ASSIGN, INT, NEWLINE,
		DEDENT, IDENT, ASSIGN, INT, NEWLINE, EOF)
}

func TestBlankAndCommentLines(t *testing.T) {
	src := "a = 1\n\n# comment\n\nb = 2\n"
	toks := tokenize(t, src)
	got := types(toks)
	want := []TokenType{IDENT, ASSIGN, INT, NEWLINE, IDENT, ASSIGN, INT, NEWLINE, EOF}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %v want %v (全部: %v)", i, got[i], want[i], got)
		}
	}
}

func TestStrings(t *testing.T) {
	src := "a = f\"x {y}\" 's' r\"r\" b\"b\" \"q\" 'q'\n"
	toks := tokenize(t, src)
	got := types(toks)
	want := []TokenType{IDENT, ASSIGN, STRING, STRING, STRING, STRING, STRING, STRING, NEWLINE, EOF}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %v want %v", i, got[i], want[i])
		}
	}
	if toks[2].Lit != `f"x {y}"` {
		t.Errorf("f-string 字面量保留失败: %q", toks[2].Lit)
	}
}

func TestTripleQuoted(t *testing.T) {
	src := "s = \"\"\"a\nb\"\"\"\n"
	toks := tokenize(t, src)
	if len(toks) < 4 || toks[2].Type != STRING || toks[2].Lit != "\"\"\"a\nb\"\"\"" {
		t.Fatalf("三引号失败: %+v", toks)
	}
}

func TestNumbers(t *testing.T) {
	assertTypes(t, "a = 0xFF + 0b10 - 0o17 * 1_000 // 2 % 1.5e3 ** .5",
		IDENT, ASSIGN, INT, PLUS, INT, MINUS, INT, STAR, INT, FLOORDIV, INT, PERCENT, FLOAT, DOUBLESTAR, FLOAT, NEWLINE, EOF)
}

func TestOperators(t *testing.T) {
	assertTypes(t, "a <= b >= c != d == e < f > g and x or y",
		IDENT, LE, IDENT, GE, IDENT, NE, IDENT, EQEQ, IDENT, LT, IDENT, GT, IDENT, AND, IDENT, OR, IDENT, NEWLINE, EOF)
}

func TestAugAssign(t *testing.T) {
	assertTypes(t, "a += 1; b -= 2; c *= 3; d **= 4",
		IDENT, PLUS_ASSIGN, INT, SEMICOLON,
		IDENT, MINUS_ASSIGN, INT, SEMICOLON,
		IDENT, STAR_ASSIGN, INT, SEMICOLON,
		IDENT, DOUBLESTAR_ASSIGN, INT, NEWLINE, EOF)
}

func TestParenthesizedContinuation(t *testing.T) {
	src := "x = (1 +\n     2)\ny = 3\n"
	toks := tokenize(t, src)
	got := types(toks)
	want := []TokenType{IDENT, ASSIGN, LPAREN, INT, PLUS, INT, RPAREN, NEWLINE, IDENT, ASSIGN, INT, NEWLINE, EOF}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %v want %v (全部: %v)", i, got[i], want[i], got)
		}
	}
}

func TestBackslashContinuation(t *testing.T) {
	src := "x = 1 + \\\n    2\ny = 3\n"
	toks := tokenize(t, src)
	got := types(toks)
	want := []TokenType{IDENT, ASSIGN, INT, PLUS, INT, NEWLINE, IDENT, ASSIGN, INT, NEWLINE, EOF}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("token %d: got %v want %v (全部: %v)", i, got[i], want[i], got)
		}
	}
}

func TestPositions(t *testing.T) {
	toks := tokenize(t, "x = 1\nabc = 2\n")
	if got := toks[0].Pos; got != (ast.Position{Line: 1, Col: 1}) {
		t.Errorf("x 位置错误: %v", got)
	}
	if got := toks[4].Pos; got != (ast.Position{Line: 2, Col: 1}) {
		t.Errorf("abc 位置错误: %v", got)
	}
}

func TestErrors(t *testing.T) {
	cases := []struct {
		name string
		src  string
	}{
		{"unterminated string", `s = "abc`},
		{"bad char", "x = 1 $ 2\n"},
		{"bad indent", "def f():\n\tpass\n"},
		{"unclosed paren", "x = (1 + 2\n"},
		{"extra paren", "x = 1)\n"},
		{"invalid number", "x = 1abc\n"},
		{"bad exponent", "x = 1e\n"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			l := New(c.src)
			for {
				tok := l.Next()
				if tok.Type == EOF {
					break
				}
			}
			if l.Err() == nil {
				t.Errorf("期望报错，实际通过")
			}
		})
	}
}
