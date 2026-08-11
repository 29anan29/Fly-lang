package ast

import "fmt"

type Position struct {
	Line int
	Col  int
}

func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Col)
}

type Diagnostic struct {
	Pos  Position
	Code string
	Msg  string
}

func (d Diagnostic) Error() string {
	if d.Code == "" {
		return fmt.Sprintf("%d:%d: %s", d.Pos.Line, d.Pos.Col, d.Msg)
	}
	return fmt.Sprintf("%d:%d: error[%s]: %s", d.Pos.Line, d.Pos.Col, d.Code, d.Msg)
}

type Node interface {
	Pos() Position
}

type Stmt interface {
	Node
	stmtNode()
}

type Expr interface {
	Node
	exprNode()
}
