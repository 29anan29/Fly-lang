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
	fnSym  *Symbol
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
	case *ast.SafeStmt:
		for _, n := range t.Names {
			c.cur.Define(n, &Symbol{Kind: KVar, Pos: t.Pos_})
		}
	case *ast.MaskStmt:
		for _, n := range t.Names {
			c.cur.Define(n, &Symbol{Kind: KVar, Pos: t.Pos_})
		}
	case *ast.OnlyStmt:
		for _, st := range t.Body {
			c.collectStmt(st)
		}
	case *ast.TraceStmt:
		for _, st := range t.Body {
			c.collectStmt(st)
		}
	case *ast.CageStmt:
		for _, st := range t.Body {
			c.collectStmt(st)
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
		c.cur.Define(t.Name, &Symbol{Kind: KClass, Pos: t.Pos_, Seal: t.Seal})
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
	case *ast.SafeStmt:
		c.checkTaintDecl(t.Names, true)
	case *ast.MaskStmt:
		c.checkTaintDecl(t.Names, false)
	case *ast.AssignStmt:
		for _, l := range t.Left {
			c.checkTarget(l)
		}
		rt, _ := c.exprTaint(t.Right)
		if t.Op != "=" {
			for _, l := range t.Left {
				if n, ok := l.(*ast.Name); ok {
					if sym, ok := c.cur.Lookup(n.Name); ok {
						rt = rt.union(sym.Taint)
					}
				}
			}
		}
		for _, l := range t.Left {
			c.defineTarget(l)
		}
		c.markSealInstances(t.Left, t.Right)
		for _, l := range t.Left {
			c.propagateTaint(l, rt)
		}
	case *ast.DeleteStmt:
		for _, tg := range t.Targets {
			if n, ok := tg.(*ast.Name); ok {
				if pos, ok := c.locked[n.Name]; ok {
					c.errorf(pos, "lock 变量 %s 不可删除", n.Name)
				}
			}
			c.checkSealInstanceAssign(tg)
		}
	case *ast.FuncDef:
		c.checkFunc(t)
	case *ast.ClassDef:
		cl := NewScope(c.cur)
		old := c.cur
		c.cur = cl
		if s, ok := c.cur.Lookup(t.Name); ok {
			s.Seal = t.Seal
		}
		for _, st := range t.Body {
			c.checkStmt(st)
		}
		c.cur = old
	case *ast.OnlyStmt:
		c.checkOnly(t)
	case *ast.TraceStmt:
		c.checkTrace(t)
	case *ast.CageStmt:
		for _, st := range t.Body {
			c.checkStmt(st)
		}
	case *ast.IfStmt:
		c.exprTaint(t.Cond)
		for _, st := range t.Then {
			c.checkStmt(st)
		}
		for _, el := range t.Elifs {
			c.exprTaint(el.Cond)
			for _, st := range el.Body {
				c.checkStmt(st)
			}
		}
		for _, st := range t.Else {
			c.checkStmt(st)
		}
	case *ast.ForStmt:
		c.exprTaint(t.Iter)
		c.checkTarget(t.Target)
		c.defineTarget(t.Target)
		for _, st := range t.Body {
			c.checkStmt(st)
		}
		for _, st := range t.Else {
			c.checkStmt(st)
		}
	case *ast.WhileStmt:
		c.exprTaint(t.Cond)
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
			c.exprTaint(h.Type)
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
		rt, _ := c.exprTaint(t.Value)
		if c.fnSym != nil {
			c.fnSym.Taint = c.fnSym.Taint.union(rt)
		}
	case *ast.RaiseStmt:
		c.exprTaint(t.Exc)
		c.exprTaint(t.From)
	case *ast.ExprStmt:
		c.exprTaint(t.X)
	}
}

func (c *Checker) propagateTaint(l ast.Expr, rt Taint) {
	switch n := l.(type) {
	case *ast.Name:
		if sym, ok := c.cur.Lookup(n.Name); ok {
			sym.Taint = rt
		}
	case *ast.TupleLit:
		for _, el := range n.Elems {
			c.propagateTaint(el, rt)
		}
	case *ast.ListLit:
		for _, el := range n.Elems {
			c.propagateTaint(el, rt)
		}
	case *ast.AttrExpr:
		c.taintObject(n.X, rt)
	case *ast.SubscriptExpr:
		c.taintObject(n.X, rt)
	}
}

func (c *Checker) taintObject(e ast.Expr, rt Taint) {
	if n, ok := e.(*ast.Name); ok {
		if sym, ok := c.cur.Lookup(n.Name); ok {
			sym.Taint = sym.Taint.union(rt)
		}
	}
}

func (c *Checker) checkFunc(t *ast.FuncDef) {
	fn := NewScope(c.cur)
	old := c.cur
	oldFn := c.fnSym
	c.cur = fn
	var fnSym *Symbol
	if s, ok := c.cur.Lookup(t.Name); ok {
		fnSym = s
	} else {
		fnSym = &Symbol{Kind: KFunc, Pos: t.Pos_}
		c.cur.Define(t.Name, fnSym)
	}
	c.fnSym = fnSym
	for _, p := range t.Params {
		if p.Name != "" {
			fn.Define(p.Name, &Symbol{Kind: KParam, Pos: t.Pos_, Anno: p.Anno})
		}
	}
	for _, st := range t.Body {
		c.checkStmt(st)
	}
	c.fnSym = oldFn
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
		c.exprTaint(t.X)
		c.exprTaint(t.Index)
		if name := c.reflectReadName(t); name != "" {
			c.errorf(c.locked[name], "lock 变量 %s 不可通过反射修改", name)
		}
	case *ast.AttrExpr:
		c.exprTaint(t.X)
		c.checkSealInstanceAssign(e)
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
