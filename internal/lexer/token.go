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

var tokenNames = map[TokenType]string{}

func init() {
	for name, tt := range keywords {
		tokenNames[tt] = name
	}
}

func (t TokenType) String() string {
	if s, ok := tokenNames[t]; ok {
		return s
	}
	switch t {
	case EOF:
		return "EOF"
	case IDENT:
		return "标识符"
	case INT:
		return "整数"
	case FLOAT:
		return "浮点数"
	case STRING:
		return "字符串"
	case NEWLINE:
		return "换行"
	case INDENT:
		return "缩进"
	case DEDENT:
		return "取消缩进"
	case ILLEGAL:
		return "非法token"
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case STAR:
		return "*"
	case SLASH:
		return "/"
	case PERCENT:
		return "%"
	case DOUBLESTAR:
		return "**"
	case FLOORDIV:
		return "//"
	case SHL:
		return "<<"
	case SHR:
		return ">>"
	case AMP:
		return "&"
	case PIPE:
		return "|"
	case CARET:
		return "^"
	case TILDE:
		return "~"
	case LT:
		return "<"
	case GT:
		return ">"
	case LE:
		return "<="
	case GE:
		return ">="
	case EQEQ:
		return "=="
	case NE:
		return "!="
	case ASSIGN:
		return "="
	case PLUS_ASSIGN:
		return "+="
	case MINUS_ASSIGN:
		return "-="
	case STAR_ASSIGN:
		return "*="
	case SLASH_ASSIGN:
		return "/="
	case PERCENT_ASSIGN:
		return "%="
	case DOUBLESTAR_ASSIGN:
		return "**="
	case FLOORDIV_ASSIGN:
		return "//="
	case SHL_ASSIGN:
		return "<<="
	case SHR_ASSIGN:
		return ">>="
	case AMP_ASSIGN:
		return "&="
	case PIPE_ASSIGN:
		return "|="
	case CARET_ASSIGN:
		return "^="
	case COLON:
		return ":"
	case COMMA:
		return ","
	case DOT:
		return "."
	case SEMICOLON:
		return ";"
	case ARROW:
		return "->"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case AT:
		return "@"
	case ELLIPSIS:
		return "..."
	}
	return "未知"
}

type Token struct {
	Type TokenType
	Lit  string
	Pos  ast.Position
}
