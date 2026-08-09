package gen

import (
	"bytes"
	"strings"

	"flylang/internal/ast"
)

type Gen struct {
	buf    bytes.Buffer
	indent int
}

func Generate(m *ast.Module) string {
	g := &Gen{}
	docEnd := 0
	if len(m.Stmts) > 0 {
		if es, ok := m.Stmts[0].(*ast.ExprStmt); ok {
			if _, ok := es.X.(*ast.StringLit); ok {
				docEnd = 1
			}
		}
	}
	guard := needsGuard(m.Stmts)
	for i, s := range m.Stmts {
		if i == docEnd && guard {
			if docEnd == 1 {
				g.w("\n")
			}
			g.guardPrelude()
		}
		g.stmt(s)
	}
	return g.buf.String()
}

func needsGuard(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		switch t := s.(type) {
		case *ast.GuardStmt:
			return true
		case *ast.FuncDef:
			if needsGuard(t.Body) {
				return true
			}
		case *ast.ClassDef:
			if needsGuard(t.Body) {
				return true
			}
		case *ast.IfStmt:
			if needsGuard(t.Then) || needsGuard(t.Else) {
				return true
			}
			for _, el := range t.Elifs {
				if needsGuard(el.Body) {
					return true
				}
			}
		case *ast.ForStmt:
			if needsGuard(t.Body) || needsGuard(t.Else) {
				return true
			}
		case *ast.WhileStmt:
			if needsGuard(t.Body) || needsGuard(t.Else) {
				return true
			}
		case *ast.TryStmt:
			if needsGuard(t.Body) || needsGuard(t.Else) || needsGuard(t.Finally) {
				return true
			}
			for _, h := range t.Handlers {
				if needsGuard(h.Body) {
					return true
				}
			}
		}
	}
	return false
}

func (g *Gen) guardPrelude() {
	g.w(`class GuardError(Exception):
    """Fly guard 断言失败"""

    pass

`)
}

func (g *Gen) indentLine() {
	g.buf.WriteString(strings.Repeat("    ", g.indent))
}

func (g *Gen) render(e ast.Expr) string {
	if e == nil {
		return ""
	}
	sub := &Gen{}
	sub.expr(e, precLowest)
	return strings.TrimSpace(sub.buf.String())
}

func (g *Gen) guardMsg(t *ast.GuardStmt) string {
	var sb strings.Builder
	sb.WriteString("guard")
	parts := 0
	if t.Name != "" {
		sb.WriteString(" " + t.Name)
		parts++
	}
	if t.Type != nil {
		sb.WriteString(": " + g.render(t.Type))
		parts++
	}
	for _, cond := range t.Conds {
		if parts > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(" " + g.render(cond))
		parts++
	}
	return sb.String()
}

func (g *Gen) w(s string) {
	g.buf.WriteString(s)
}

func (g *Gen) stmt(s ast.Stmt) {
	switch t := s.(type) {
	case *ast.ImportStmt:
		g.indentLine()
		g.w("import ")
		for i, it := range t.Items {
			if i > 0 {
				g.w(", ")
			}
			g.w(it.Name)
			if it.Alias != "" {
				g.w(" as " + it.Alias)
			}
		}
		g.w("\n")
	case *ast.FromImportStmt:
		g.indentLine()
		g.w("from " + t.Module + " import ")
		for i, it := range t.Items {
			if i > 0 {
				g.w(", ")
			}
			g.w(it.Name)
			if it.Alias != "" {
				g.w(" as " + it.Alias)
			}
		}
		g.w("\n")
	case *ast.AssignStmt:
		g.indentLine()
		for i, l := range t.Left {
			if i > 0 {
				g.w(" = ")
			}
			g.expr(l, precLowest)
		}
		g.w(" " + t.Op + " ")
		g.expr(t.Right, precLowest)
		g.w("\n")
	case *ast.LockStmt:
		if t.Value != nil {
			g.indentLine()
			g.w(t.Name)
			g.w(" = ")
			g.expr(t.Value, precLowest)
			g.w("\n")
		}
	case *ast.GuardStmt:
		g.indentLine()
		g.w("if not (")
		first := true
		if t.Name != "" && t.Type != nil {
			g.w("isinstance(" + t.Name + ", ")
			g.expr(t.Type, precLowest)
			g.w(")")
			first = false
		}
		for _, cond := range t.Conds {
			if !first {
				g.w(" and ")
			}
			g.w("(")
			g.expr(cond, precLowest)
			g.w(")")
			first = false
		}
		g.w("):\n")
		g.indent++
		g.indentLine()
		g.w(`raise GuardError("` + g.guardMsg(t) + `")` + "\n")
		g.indent--
	case *ast.ExprStmt:
		g.indentLine()
		g.expr(t.X, precLowest)
		g.w("\n")
	case *ast.FuncDef:
		for _, d := range t.Decorators {
			g.indentLine()
			g.w("@")
			g.expr(d, precLowest)
			g.w("\n")
		}
		g.indentLine()
		g.w("def " + t.Name + "(")
		g.params(t.Params)
		g.w(")")
		if t.ReturnType != nil {
			g.w(" -> ")
			g.expr(t.ReturnType, precLowest)
		}
		g.w(":")
		g.suite(t.Body)
	case *ast.ClassDef:
		for _, d := range t.Decorators {
			g.indentLine()
			g.w("@")
			g.expr(d, precLowest)
			g.w("\n")
		}
		g.indentLine()
		g.w("class " + t.Name)
		if len(t.Bases) > 0 {
			g.w("(")
			for i, b := range t.Bases {
				if i > 0 {
					g.w(", ")
				}
				g.expr(b, precLowest)
			}
			g.w(")")
		}
		g.w(":")
		g.suite(t.Body)
	case *ast.IfStmt:
		g.indentLine()
		g.w("if ")
		g.expr(t.Cond, precLowest)
		g.w(":")
		g.suite(t.Then)
		for _, el := range t.Elifs {
			g.indentLine()
			g.w("elif ")
			g.expr(el.Cond, precLowest)
			g.w(":")
			g.suite(el.Body)
		}
		if len(t.Else) > 0 {
			g.indentLine()
			g.w("else:")
			g.suite(t.Else)
		}
	case *ast.ForStmt:
		g.indentLine()
		g.w("for ")
		g.expr(t.Target, precLowest)
		g.w(" in ")
		g.expr(t.Iter, precLowest)
		g.w(":")
		g.suite(t.Body)
		if len(t.Else) > 0 {
			g.indentLine()
			g.w("else:")
			g.suite(t.Else)
		}
	case *ast.WhileStmt:
		g.indentLine()
		g.w("while ")
		g.expr(t.Cond, precLowest)
		g.w(":")
		g.suite(t.Body)
		if len(t.Else) > 0 {
			g.indentLine()
			g.w("else:")
			g.suite(t.Else)
		}
	case *ast.ReturnStmt:
		g.indentLine()
		g.w("return")
		if t.Value != nil {
			g.w(" ")
			g.expr(t.Value, precLowest)
		}
		g.w("\n")
	case *ast.RaiseStmt:
		g.indentLine()
		g.w("raise")
		if t.Exc != nil {
			g.w(" ")
			g.expr(t.Exc, precLowest)
		}
		if t.From != nil {
			g.w(" from ")
			g.expr(t.From, precLowest)
		}
		g.w("\n")
	case *ast.TryStmt:
		g.indentLine()
		g.w("try:")
		g.suite(t.Body)
		for _, h := range t.Handlers {
			g.indentLine()
			g.w("except")
			if h.Type != nil {
				g.w(" ")
				g.expr(h.Type, precLowest)
				if h.Name != "" {
					g.w(" as " + h.Name)
				}
			}
			g.w(":")
			g.suite(h.Body)
		}
		if len(t.Else) > 0 {
			g.indentLine()
			g.w("else:")
			g.suite(t.Else)
		}
		if len(t.Finally) > 0 {
			g.indentLine()
			g.w("finally:")
			g.suite(t.Finally)
		}
	case *ast.PassStmt:
		g.indentLine()
		g.w("pass\n")
	case *ast.BreakStmt:
		g.indentLine()
		g.w("break\n")
	case *ast.ContinueStmt:
		g.indentLine()
		g.w("continue\n")
	case *ast.DeleteStmt:
		g.indentLine()
		g.w("del ")
		for i, tg := range t.Targets {
			if i > 0 {
				g.w(", ")
			}
			g.expr(tg, precLowest)
		}
		g.w("\n")
	}
}

func (g *Gen) params(params []ast.Param) {
	for i, p := range params {
		if i > 0 {
			g.w(", ")
		}
		if p.Star {
			g.w("*")
		}
		if p.DblStar {
			g.w("**")
		}
		if p.Name != "" {
			g.w(p.Name)
		}
		if p.Anno != nil {
			g.w(": ")
			g.expr(p.Anno, precLowest)
		}
		if p.Default != nil {
			g.w("=")
			g.expr(p.Default, precLowest)
		}
	}
}

func (g *Gen) suite(body []ast.Stmt) {
	if len(body) == 0 {
		g.w(" pass\n")
		return
	}
	g.w("\n")
	g.indent++
	for _, s := range body {
		g.stmt(s)
	}
	g.indent--
}

const (
	precLowest  = -10
	precTuple   = -4
	precCond    = -3
	precOrBool  = -2
	precAndBool = -1
	precCompare = 0
	precOr      = 1
	precXor     = 2
	precAnd     = 3
	precShift   = 4
	precAdd     = 5
	precMul     = 6
	precUnary   = 7
	precPower   = 8
	precPost    = 9
	precAtom    = 10
)

func binPrec(op string) int {
	switch op {
	case "|":
		return precOr
	case "^":
		return precXor
	case "&":
		return precAnd
	case "<<", ">>":
		return precShift
	case "+", "-":
		return precAdd
	case "*", "/", "//", "%":
		return precMul
	case "**":
		return precPower
	}
	return precAtom
}

func precOf(e ast.Expr) int {
	switch t := e.(type) {
	case *ast.Name, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.EllipsisLit,
		*ast.ListLit, *ast.DictLit, *ast.SetLit, *ast.ListComp:
		return precAtom
	case *ast.CallExpr, *ast.AttrExpr, *ast.SubscriptExpr:
		return precPost
	case *ast.BinOpExpr:
		return binPrec(t.Op)
	case *ast.UnaryOpExpr:
		if t.Op == "not" {
			return precCompare
		}
		return precUnary
	case *ast.BoolOpExpr:
		if t.Op == "and" {
			return precAndBool
		}
		return precOrBool
	case *ast.CompareExpr:
		return precCompare
	case *ast.CondExpr:
		return precCond
	case *ast.TupleLit:
		if t.Paren {
			return precPost
		}
		return precTuple
	case *ast.SliceExpr:
		return precPost
	}
	return precLowest
}

func (g *Gen) expr(e ast.Expr, parent int) {
	if e == nil {
		return
	}
	if precOf(e) < parent {
		g.w("(")
		g.expr(e, precLowest)
		g.w(")")
		return
	}
	switch t := e.(type) {
	case *ast.Name:
		g.w(t.Name)
	case *ast.IntLit:
		g.w(t.Value)
	case *ast.FloatLit:
		g.w(t.Value)
	case *ast.StringLit:
		g.w(t.Value)
	case *ast.EllipsisLit:
		g.w("...")
	case *ast.ListLit:
		g.w("[")
		for i, el := range t.Elems {
			if i > 0 {
				g.w(", ")
			}
			g.expr(el, precCond)
		}
		g.w("]")
	case *ast.TupleLit:
		if t.Paren {
			g.w("(")
		}
		for i, el := range t.Elems {
			if i > 0 {
				g.w(", ")
			}
			g.expr(el, precCond)
		}
		if t.Paren {
			if len(t.Elems) == 1 {
				g.w(",")
			}
			g.w(")")
		} else if len(t.Elems) == 1 {
			g.w(",")
		}
	case *ast.DictLit:
		g.w("{")
		for i := range t.Keys {
			if i > 0 {
				g.w(", ")
			}
			g.expr(t.Keys[i], precCond)
			g.w(": ")
			g.expr(t.Vals[i], precCond)
		}
		g.w("}")
	case *ast.SetLit:
		g.w("{")
		for i, el := range t.Elems {
			if i > 0 {
				g.w(", ")
			}
			g.expr(el, precCond)
		}
		g.w("}")
	case *ast.CallExpr:
		g.expr(t.Func, precPost)
		g.w("(")
		first := true
		for _, a := range t.Args {
			if !first {
				g.w(", ")
			}
			first = false
			g.expr(a, precCond)
		}
		if t.Star != nil {
			if !first {
				g.w(", ")
			}
			first = false
			g.w("*")
			g.expr(t.Star, precCond)
		}
		if t.DblStar != nil {
			if !first {
				g.w(", ")
			}
			first = false
			g.w("**")
			g.expr(t.DblStar, precCond)
		}
		for _, kw := range t.Kwargs {
			if !first {
				g.w(", ")
			}
			first = false
			g.w(kw.Name + "=")
			g.expr(kw.Value, precCond)
		}
		g.w(")")
	case *ast.AttrExpr:
		g.expr(t.X, precPost)
		g.w("." + t.Name)
	case *ast.SubscriptExpr:
		g.expr(t.X, precPost)
		g.w("[")
		g.expr(t.Index, precLowest)
		g.w("]")
	case *ast.SliceExpr:
		if t.Lo != nil {
			g.expr(t.Lo, precCond)
		}
		g.w(":")
		if t.Hi != nil {
			g.expr(t.Hi, precCond)
		}
		if t.Step != nil {
			g.w(":")
			g.expr(t.Step, precCond)
		}
	case *ast.BinOpExpr:
		if t.Op == "**" {
			g.expr(t.X, precPower+1)
			g.w("**")
			g.expr(t.Y, precPower)
			return
		}
		p := binPrec(t.Op)
		g.expr(t.X, p)
		g.w(" " + t.Op + " ")
		g.expr(t.Y, p+1)
	case *ast.UnaryOpExpr:
		g.w(t.Op + " ")
		if t.Op == "not" {
			g.expr(t.X, precCompare)
		} else {
			g.expr(t.X, precUnary)
		}
	case *ast.BoolOpExpr:
		p := precOrBool
		if t.Op == "and" {
			p = precAndBool
		}
		g.expr(t.X, p)
		g.w(" " + t.Op + " ")
		g.expr(t.Y, p)
	case *ast.CompareExpr:
		g.expr(t.X, precCompare)
		for i, op := range t.Ops {
			g.w(" " + op + " ")
			g.expr(t.Ys[i], precCompare)
		}
	case *ast.CondExpr:
		g.expr(t.Then, precCond+1)
		g.w(" if ")
		g.expr(t.Cond, precCond)
		g.w(" else ")
		g.expr(t.Else, precCond)
	case *ast.ListComp:
		g.w("[")
		g.expr(t.Elem, precCond)
		for _, cl := range t.Clauses {
			g.w(" for ")
			g.expr(cl.Target, precCond)
			g.w(" in ")
			g.expr(cl.Iter, precCond)
			for _, f := range cl.Ifs {
				g.w(" if ")
				g.expr(f, precCond)
			}
		}
		g.w("]")
	}
}
