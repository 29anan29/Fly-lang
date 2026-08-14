// guard.go：guard 编译期检查——声明式值校验（无代码块），校验引用名字必须先定义（E0043）。
package checker

import "flylang/internal/ast"

func (c *Checker) checkGuard(t *ast.GuardStmt) {
	if t.Name != "" {
		sym, ok := c.cur.Lookup(t.Name)
		if !ok {
			c.errorf(t.Pos_, "guard 变量 %s 未定义", t.Name)
			return
		}
		if t.Type != nil {
			tp, ok := t.Type.(*ast.Name)
			if !ok {
				c.errorf(t.Type.Pos(), "guard 类型必须是简单类型名（如 int、str）")
			} else if sym.Kind == KParam {
				if an, ok := sym.Anno.(*ast.Name); ok && an.Name != tp.Name {
					c.errorf(t.Pos_, "guard 类型 %s 与参数注解 %s 不一致", tp.Name, an.Name)
				}
			}
		}
	}
	if t.Type == nil && len(t.Conds) == 0 {
		c.errorf(t.Pos_, "guard 至少需要一个类型或条件")
		return
	}
	for _, cond := range t.Conds {
		c.resolveNames(cond)
	}
}

func (c *Checker) resolveNames(e ast.Expr) {
	switch t := e.(type) {
	case *ast.Name:
		if builtins[t.Name] {
			return
		}
		if _, ok := c.cur.Lookup(t.Name); ok {
			return
		}
		c.errorf(t.Pos_, "guard 条件中引用了未定义的名字 %s", t.Name)
	case *ast.ListLit:
		for _, el := range t.Elems {
			c.resolveNames(el)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			c.resolveNames(el)
		}
	case *ast.DictLit:
		for i := range t.Keys {
			c.resolveNames(t.Keys[i])
			c.resolveNames(t.Vals[i])
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			c.resolveNames(el)
		}
	case *ast.CallExpr:
		c.resolveNames(t.Func)
		for _, a := range t.Args {
			c.resolveNames(a)
		}
		c.resolveNames(t.Star)
		c.resolveNames(t.DblStar)
		for _, kw := range t.Kwargs {
			c.resolveNames(kw.Value)
		}
	case *ast.AttrExpr:
		c.resolveNames(t.X)
	case *ast.SubscriptExpr:
		c.resolveNames(t.X)
		c.resolveNames(t.Index)
	case *ast.SliceExpr:
		c.resolveNames(t.Lo)
		c.resolveNames(t.Hi)
		c.resolveNames(t.Step)
	case *ast.BinOpExpr:
		c.resolveNames(t.X)
		c.resolveNames(t.Y)
	case *ast.UnaryOpExpr:
		c.resolveNames(t.X)
	case *ast.BoolOpExpr:
		c.resolveNames(t.X)
		c.resolveNames(t.Y)
	case *ast.CompareExpr:
		c.resolveNames(t.X)
		for _, y := range t.Ys {
			c.resolveNames(y)
		}
	case *ast.CondExpr:
		c.resolveNames(t.Cond)
		c.resolveNames(t.Then)
		c.resolveNames(t.Else)
	case *ast.ListComp:
		c.resolveNames(t.Elem)
		for _, cl := range t.Clauses {
			c.resolveNames(cl.Iter)
			for _, f := range cl.Ifs {
				c.resolveNames(f)
			}
		}
	}
}
