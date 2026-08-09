package checker

import (
	"fmt"

	"flylang/internal/ast"
)

const maxErrs = 20

type Checker struct {
	global *Scope
	cur    *Scope
	locked map[string]ast.Position
	errs   []ast.Diagnostic
}

func Check(m *ast.Module) []ast.Diagnostic {
	c := &Checker{global: NewScope(nil), locked: make(map[string]ast.Position)}
	c.cur = c.global
	c.collectModule(m)
	c.cur = c.global
	c.checkModule(m)
	return c.errs
}

func (c *Checker) errorf(pos ast.Position, format string, args ...interface{}) {
	if len(c.errs) >= maxErrs {
		return
	}
	c.errs = append(c.errs, ast.Diagnostic{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

func (c *Checker) collectModule(m *ast.Module) {
	for _, s := range m.Stmts {
		c.collectStmt(s)
	}
}

func (c *Checker) collectStmt(s ast.Stmt) {
	switch t := s.(type) {
	case *ast.ImportStmt:
		for _, it := range t.Items {
			c.cur.Define(importedName(it), &Symbol{Kind: KImport, Pos: s.Pos()})
		}
	case *ast.FromImportStmt:
		for _, it := range t.Items {
			if it.Name == "*" {
				continue
			}
			c.cur.Define(importedName(it), &Symbol{Kind: KImport, Pos: s.Pos()})
		}
	case *ast.AssignStmt:
		for _, l := range t.Left {
			c.defineTarget(l)
		}
	case *ast.LockStmt:
		if t.Value != nil {
			c.cur.Define(t.Name, &Symbol{Kind: KVar, Pos: t.Pos_})
		}
	case *ast.FuncDef:
		c.cur.Define(t.Name, &Symbol{Kind: KFunc, Pos: t.Pos_})
		fn := NewScope(c.cur)
		old := c.cur
		c.cur = fn
		for _, p := range t.Params {
			if p.Name != "" {
				fn.Define(p.Name, &Symbol{Kind: KParam, Pos: t.Pos_, Anno: p.Anno})
			}
		}
		for _, st := range t.Body {
			c.collectStmt(st)
		}
		c.cur = old
	case *ast.ClassDef:
		c.cur.Define(t.Name, &Symbol{Kind: KClass, Pos: t.Pos_})
		cl := NewScope(c.cur)
		old := c.cur
		c.cur = cl
		for _, st := range t.Body {
			c.collectStmt(st)
		}
		c.cur = old
	case *ast.ForStmt:
		c.defineTarget(t.Target)
		for _, st := range t.Body {
			c.collectStmt(st)
		}
		for _, st := range t.Else {
			c.collectStmt(st)
		}
	case *ast.WhileStmt:
		for _, st := range t.Body {
			c.collectStmt(st)
		}
		for _, st := range t.Else {
			c.collectStmt(st)
		}
	case *ast.IfStmt:
		for _, st := range t.Then {
			c.collectStmt(st)
		}
		for _, el := range t.Elifs {
			for _, st := range el.Body {
				c.collectStmt(st)
			}
		}
		for _, st := range t.Else {
			c.collectStmt(st)
		}
	case *ast.TryStmt:
		for _, st := range t.Body {
			c.collectStmt(st)
		}
		for _, h := range t.Handlers {
			if h.Name != "" {
				c.cur.Define(h.Name, &Symbol{Kind: KVar, Pos: h.Pos_})
			}
			for _, st := range h.Body {
				c.collectStmt(st)
			}
		}
		for _, st := range t.Else {
			c.collectStmt(st)
		}
		for _, st := range t.Finally {
			c.collectStmt(st)
		}
	}
}

func (c *Checker) defineTarget(e ast.Expr) {
	switch t := e.(type) {
	case *ast.Name:
		c.cur.Define(t.Name, &Symbol{Kind: KVar, Pos: t.Pos_})
	case *ast.TupleLit:
		for _, el := range t.Elems {
			c.defineTarget(el)
		}
	case *ast.ListLit:
		for _, el := range t.Elems {
			c.defineTarget(el)
		}
	}
}

func importedName(it ast.ImportItem) string {
	if it.Alias != "" {
		return it.Alias
	}
	return it.Name
}

func (c *Checker) checkModule(m *ast.Module) {
	for _, s := range m.Stmts {
		c.checkStmt(s)
	}
}

func (c *Checker) checkStmt(s ast.Stmt) {
	switch t := s.(type) {
	case *ast.LockStmt:
		c.checkLock(t)
	case *ast.GuardStmt:
		c.checkGuard(t)
	case *ast.AssignStmt:
		for _, l := range t.Left {
			c.checkTarget(l)
		}
		for _, l := range t.Left {
			c.defineTarget(l)
		}
		c.walkExpr(t.Right)
	case *ast.DeleteStmt:
		for _, tg := range t.Targets {
			if n, ok := tg.(*ast.Name); ok {
				if pos, ok := c.locked[n.Name]; ok {
					c.errorf(pos, "lock 变量 %s 不可删除", n.Name)
				}
			}
		}
	case *ast.FuncDef:
		c.checkFunc(t)
	case *ast.ClassDef:
		cl := NewScope(c.cur)
		old := c.cur
		c.cur = cl
		for _, st := range t.Body {
			c.checkStmt(st)
		}
		c.cur = old
	case *ast.IfStmt:
		c.walkExpr(t.Cond)
		for _, st := range t.Then {
			c.checkStmt(st)
		}
		for _, el := range t.Elifs {
			c.walkExpr(el.Cond)
			for _, st := range el.Body {
				c.checkStmt(st)
			}
		}
		for _, st := range t.Else {
			c.checkStmt(st)
		}
	case *ast.ForStmt:
		c.walkExpr(t.Iter)
		c.checkTarget(t.Target)
		c.defineTarget(t.Target)
		for _, st := range t.Body {
			c.checkStmt(st)
		}
		for _, st := range t.Else {
			c.checkStmt(st)
		}
	case *ast.WhileStmt:
		c.walkExpr(t.Cond)
		for _, st := range t.Body {
			c.checkStmt(st)
		}
		for _, st := range t.Else {
			c.checkStmt(st)
		}
	case *ast.TryStmt:
		for _, st := range t.Body {
			c.checkStmt(st)
		}
		for _, h := range t.Handlers {
			c.walkExpr(h.Type)
			if h.Name != "" {
				c.cur.Define(h.Name, &Symbol{Kind: KVar, Pos: h.Pos_})
			}
			for _, st := range h.Body {
				c.checkStmt(st)
			}
		}
		for _, st := range t.Else {
			c.checkStmt(st)
		}
		for _, st := range t.Finally {
			c.checkStmt(st)
		}
	case *ast.ReturnStmt:
		c.walkExpr(t.Value)
	case *ast.RaiseStmt:
		c.walkExpr(t.Exc)
		c.walkExpr(t.From)
	case *ast.ExprStmt:
		c.walkExpr(t.X)
	}
}

func (c *Checker) checkFunc(t *ast.FuncDef) {
	fn := NewScope(c.cur)
	old := c.cur
	c.cur = fn
	for _, p := range t.Params {
		if p.Name != "" {
			fn.Define(p.Name, &Symbol{Kind: KParam, Pos: t.Pos_, Anno: p.Anno})
		}
	}
	for _, st := range t.Body {
		c.checkStmt(st)
	}
	c.cur = old
}

func (c *Checker) checkTarget(e ast.Expr) {
	switch t := e.(type) {
	case *ast.Name:
		if pos, ok := c.locked[t.Name]; ok {
			c.errorf(pos, "lock 变量 %s 不可再赋值", t.Name)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			c.checkTarget(el)
		}
	case *ast.ListLit:
		for _, el := range t.Elems {
			c.checkTarget(el)
		}
	case *ast.SubscriptExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Index)
		if name := c.reflectReadName(t); name != "" {
			c.errorf(c.locked[name], "lock 变量 %s 不可通过反射修改", name)
		}
	case *ast.AttrExpr:
		c.walkExpr(t.X)
	}
}

func (c *Checker) walkExpr(e ast.Expr) {
	switch t := e.(type) {
	case *ast.Name, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.EllipsisLit:
	case *ast.ListLit:
		for _, el := range t.Elems {
			c.walkExpr(el)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			c.walkExpr(el)
		}
	case *ast.DictLit:
		for i := range t.Keys {
			c.walkExpr(t.Keys[i])
			c.walkExpr(t.Vals[i])
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			c.walkExpr(el)
		}
	case *ast.CallExpr:
		c.walkExpr(t.Func)
		for _, a := range t.Args {
			c.walkExpr(a)
		}
		c.walkExpr(t.Star)
		c.walkExpr(t.DblStar)
		for _, kw := range t.Kwargs {
			c.walkExpr(kw.Value)
		}
		c.checkReflectCall(t)
	case *ast.AttrExpr:
		c.walkExpr(t.X)
	case *ast.SubscriptExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Index)
		if name := c.reflectReadName(t); name != "" {
			c.errorf(c.locked[name], "lock 变量 %s 不可通过 %s() 反射读取", name, reflectCallName(t.X))
		}
	case *ast.SliceExpr:
		c.walkExpr(t.Lo)
		c.walkExpr(t.Hi)
		c.walkExpr(t.Step)
	case *ast.BinOpExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Y)
	case *ast.UnaryOpExpr:
		c.walkExpr(t.X)
	case *ast.BoolOpExpr:
		c.walkExpr(t.X)
		c.walkExpr(t.Y)
	case *ast.CompareExpr:
		c.walkExpr(t.X)
		for _, y := range t.Ys {
			c.walkExpr(y)
		}
	case *ast.CondExpr:
		c.walkExpr(t.Cond)
		c.walkExpr(t.Then)
		c.walkExpr(t.Else)
	case *ast.ListComp:
		c.walkExpr(t.Elem)
		for _, cl := range t.Clauses {
			c.walkExpr(cl.Target)
			c.walkExpr(cl.Iter)
			for _, f := range cl.Ifs {
				c.walkExpr(f)
			}
		}
	}
}

func (c *Checker) checkReflectCall(t *ast.CallExpr) {
	name, ok := t.Func.(*ast.Name)
	if !ok {
		return
	}
	switch name.Name {
	case "setattr":
		if len(t.Args) < 2 {
			return
		}
		if n, ok := t.Args[0].(*ast.Name); ok {
			if pos, ok := c.locked[n.Name]; ok {
				c.errorf(pos, "lock 变量 %s 不可通过 setattr 修改", n.Name)
				return
			}
		}
		if globals := reflectGlobals(t.Args[0]); globals != "" {
			if lit, ok := t.Args[1].(*ast.StringLit); ok {
				if pos, ok := c.locked[unquote(lit.Value)]; ok {
					c.errorf(pos, "lock 变量 %s 不可通过 setattr 修改", unquote(lit.Value))
				}
			}
		}
	case "globals", "vars", "locals":
		if len(t.Args) < 1 {
			return
		}
		if lit, ok := t.Args[0].(*ast.StringLit); ok {
			if pos, ok := c.locked[unquote(lit.Value)]; ok {
				c.errorf(pos, "lock 变量 %s 不可通过 %s() 反射读取", unquote(lit.Value), name.Name)
			}
		}
	}
}

func reflectGlobals(e ast.Expr) string {
	cl, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	name, ok := cl.Func.(*ast.Name)
	if !ok || (name.Name != "globals" && name.Name != "vars") || len(cl.Args) > 0 {
		return ""
	}
	return name.Name
}

func reflectCallName(e ast.Expr) string {
	cl, ok := e.(*ast.CallExpr)
	if !ok {
		return ""
	}
	name, ok := cl.Func.(*ast.Name)
	if !ok {
		return ""
	}
	return name.Name
}

func (c *Checker) reflectReadName(t *ast.SubscriptExpr) string {
	if globals := reflectGlobals(t.X); globals == "" {
		return ""
	}
	lit, ok := t.Index.(*ast.StringLit)
	if !ok {
		return ""
	}
	name := unquote(lit.Value)
	if _, ok := c.locked[name]; ok {
		return name
	}
	return ""
}

func unquote(s string) string {
	if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') && s[len(s)-1] == s[0] {
		return s[1 : len(s)-1]
	}
	return s
}
