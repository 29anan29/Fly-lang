// Package lexer 词法分析：源文件字符串 → Token 流（关键字/字面量/运算符）。
// 与 Rust 版 src/lexer 行为一致（CLI 对照验收），fuzz 目标见 fuzz_test.go。
package lexer

import "flylang/internal/ast"

type TokenType int

const (
	ILLEGAL TokenType = iota
	EOF
	IDENT
	INT
	FLOAT
	STRING
	NEWLINE
	INDENT
	DEDENT

	DEF
	CLASS
	IF
	ELIF
	ELSE
	FOR
	WHILE
	RETURN
	RAISE
	TRY
	EXCEPT
	FINALLY
	PASS
	BREAK
	CONTINUE
	IMPORT
	FROM
	AS
	IN
	NOT
	AND
	OR
	IS
	NONE
	TRUE
	FALSE
	DEL
	ASSERT
	WITH
	YIELD
	GLOBAL
	NONLOCAL
	LAMBDA
	ASYNC
	AWAIT

	SAFE
	ONLY
	LOCK
	MASK
	CAGE
	GUARD
	SEAL
	TRACE

	PLUS
	MINUS
	STAR
	SLASH
	PERCENT
	DOUBLESTAR
	FLOORDIV
	SHL
	SHR
	AMP
	PIPE
	CARET
	TILDE
	LT
	GT
	LE
	GE
	EQEQ
	NE
	ASSIGN
	PLUS_ASSIGN
	MINUS_ASSIGN
	STAR_ASSIGN
	SLASH_ASSIGN
	PERCENT_ASSIGN
	DOUBLESTAR_ASSIGN
	FLOORDIV_ASSIGN
	SHL_ASSIGN
	SHR_ASSIGN
	AMP_ASSIGN
	PIPE_ASSIGN
	CARET_ASSIGN
	COLON
	COMMA
	DOT
	SEMICOLON
	ARROW
	LPAREN
	RPAREN
	LBRACKET
	RBRACKET
	LBRACE
	RBRACE
	AT
	ELLIPSIS
)

// tokenText 非关键字 token 的展示名（关键字部分由 keywords 反查得到）。
var tokenText = map[TokenType]string{
	EOF:               "EOF",
	IDENT:             "标识符",
	INT:               "整数",
	FLOAT:             "浮点数",
	STRING:            "字符串",
	NEWLINE:           "换行",
	INDENT:            "缩进",
	DEDENT:            "取消缩进",
	ILLEGAL:           "非法token",
	PLUS:              "+",
	MINUS:             "-",
	STAR:              "*",
	SLASH:             "/",
	PERCENT:           "%",
	DOUBLESTAR:        "**",
	FLOORDIV:          "//",
	SHL:               "<<",
	SHR:               ">>",
	AMP:               "&",
	PIPE:              "|",
	CARET:             "^",
	TILDE:             "~",
	LT:                "<",
	GT:                ">",
	LE:                "<=",
	GE:                ">=",
	EQEQ:              "==",
	NE:                "!=",
	ASSIGN:            "=",
	PLUS_ASSIGN:       "+=",
	MINUS_ASSIGN:      "-=",
	STAR_ASSIGN:       "*=",
	SLASH_ASSIGN:      "/=",
	PERCENT_ASSIGN:    "%=",
	DOUBLESTAR_ASSIGN: "**=",
	FLOORDIV_ASSIGN:   "//=",
	SHL_ASSIGN:        "<<=",
	SHR_ASSIGN:        ">>=",
	AMP_ASSIGN:        "&=",
	PIPE_ASSIGN:       "|=",
	CARET_ASSIGN:      "^=",
	COLON:             ":",
	COMMA:             ",",
	DOT:               ".",
	SEMICOLON:         ";",
	ARROW:             "->",
	LPAREN:            "(",
	RPAREN:            ")",
	LBRACKET:          "[",
	RBRACKET:          "]",
	LBRACE:            "{",
	RBRACE:            "}",
	AT:                "@",
	ELLIPSIS:          "...",
}

// keywords 关键字文本 → TokenType（含 8 个 Fly 安全关键字）。
var keywords = map[string]TokenType{
	"def": DEF, "class": CLASS, "if": IF, "elif": ELIF, "else": ELSE,
	"for": FOR, "while": WHILE, "return": RETURN, "raise": RAISE,
	"try": TRY, "except": EXCEPT, "finally": FINALLY, "pass": PASS,
	"break": BREAK, "continue": CONTINUE, "import": IMPORT, "from": FROM,
	"as": AS, "in": IN, "not": NOT, "and": AND, "or": OR, "is": IS,
	"None": NONE, "True": TRUE, "False": FALSE, "del": DEL,
	"assert": ASSERT, "with": WITH, "yield": YIELD, "global": GLOBAL,
	"nonlocal": NONLOCAL, "lambda": LAMBDA, "async": ASYNC, "await": AWAIT,

	"safe": SAFE, "only": ONLY, "lock": LOCK, "mask": MASK,
	"cage": CAGE, "guard": GUARD, "seal": SEAL, "trace": TRACE,
}

// tokenNames 由 keywords 反查：关键字 TokenType → 文本。
var tokenNames = map[TokenType]string{}

func init() {
	for name, tt := range keywords {
		tokenNames[tt] = name
	}
}

// String 返回 token 展示名：关键字表 → 非关键字静态表 → 未知。
func (t TokenType) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	if s, ok := tokenText[t]; ok {
		return s
	}
	return "未知"
}

type Token struct {
	Type TokenType
	Lit  string
	Pos  ast.Position
}
