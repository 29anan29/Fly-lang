package checker

import (
	"pyfly/internal/ast"
)

type Taint struct {
	Safe bool
	Mask bool
}

func (t Taint) union(o Taint) Taint {
	return Taint{Safe: t.Safe || o.Safe, Mask: t.Mask || o.Mask}
}

func (t Taint) dirty() bool {
	return t.Safe || t.Mask
}

var sanitizers = map[string]bool{
	"int": true, "float": true, "bool": true,
}

func isSanitizer(name string) bool {
	return sanitizers[name]
}

func (c *Checker) exprTaint(e ast.Expr) (Taint, string) {
	if e == nil {
		return Taint{}, ""
	}
	switch t := e.(type) {
	case *ast.Name:
		if sym, ok := c.cur.Lookup(t.Name); ok {
			return sym.Taint, t.Name
		}
		return Taint{}, ""
	case *ast.StringLit:
		return c.fstringTaint(t)
	case *ast.IntLit, *ast.FloatLit, *ast.EllipsisLit:
		return Taint{}, ""
	case *ast.ListLit:
		return c.unionTaint(t.Elems)
	case *ast.TupleLit:
		return c.unionTaint(t.Elems)
	case *ast.SetLit:
		return c.unionTaint(t.Elems)
	case *ast.DictLit:
		return c.unionTaint(append(append([]ast.Expr{}, t.Keys...), t.Vals...))
	case *ast.CallExpr:
		return c.callTaint(t)
	case *ast.AttrExpr:
		if t.Name == "environ" {
			if n, ok := t.X.(*ast.Name); ok && n.Name == "os" {
				return Taint{Safe: true}, "os.environ"
			}
		}
		bt, bn := c.exprTaint(t.X)
		if n, ok := t.X.(*ast.Name); ok {
			if sym, ok := c.cur.Lookup(n.Name); ok {
				if at, ok := sym.Attrs[t.Name]; ok {
					return bt.union(at), firstHint(bn, n.Name+"."+t.Name)
				}
			}
		}
		return bt, bn
	case *ast.SubscriptExpr:
		c.exprTaint(t.Index)
		if name := c.reflectReadName(t); name != "" {
			c.errorf(c.locked[name], "lock 变量 %s 不可通过 %s() 反射读取", name, reflectCallName(t.X))
		}
		return c.exprTaint(t.X)
	case *ast.SliceExpr:
		var parts []ast.Expr
		for _, p := range []ast.Expr{t.Lo, t.Hi, t.Step} {
			if p != nil {
				parts = append(parts, p)
			}
		}
		return c.unionTaint(parts)
	case *ast.BinOpExpr:
		xt, xn := c.exprTaint(t.X)
		yt, yn := c.exprTaint(t.Y)
		return xt.union(yt), firstHint(xn, yn)
	case *ast.UnaryOpExpr:
		return c.exprTaint(t.X)
	case *ast.BoolOpExpr:
		return c.exprTaint(t.X)
	case *ast.CompareExpr:
		for _, y := range t.Ys {
			c.exprTaint(y)
		}
		return Taint{}, ""
	case *ast.CondExpr:
		tt, tn := c.exprTaint(t.Then)
		et, en := c.exprTaint(t.Else)
		c.exprTaint(t.Cond)
		return tt.union(et), firstHint(tn, en)
	case *ast.ListComp:
		et, en := c.exprTaint(t.Elem)
		for _, cl := range t.Clauses {
			c.exprTaint(cl.Iter)
		}
		return et, en
	}
	return Taint{}, ""
}

func (c *Checker) unionTaint(exprs []ast.Expr) (Taint, string) {
	var res Taint
	var hint string
	for _, e := range exprs {
		t, n := c.exprTaint(e)
		res = res.union(t)
		if hint == "" {
			hint = n
		}
	}
	return res, hint
}

func firstHint(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (c *Checker) callTaint(t *ast.CallExpr) (Taint, string) {
	c.checkReflectCall(t)
	recvTaint, recvHint := Taint{}, ""
	if a, ok := t.Func.(*ast.AttrExpr); ok {
		recvTaint, recvHint = c.exprTaint(a.X)
	}
	var argTaint Taint
	var argHint string
	for _, a := range t.Args {
		at, an := c.exprTaint(a)
		argTaint = argTaint.union(at)
		if argHint == "" {
			argHint = an
		}
	}
	c.exprTaint(t.Star)
	c.exprTaint(t.DblStar)
	for _, kw := range t.Kwargs {
		kt, kn := c.exprTaint(kw.Value)
		argTaint = argTaint.union(kt)
		if argHint == "" {
			argHint = kn
		}
	}

	if a, ok := t.Func.(*ast.AttrExpr); ok {
		if n, ok := a.X.(*ast.Name); ok {
			if sym, ok := c.cur.Lookup(n.Name); ok && sym.Kind == KClass && sym.Scope != nil {
				if m, ok := sym.Scope.Lookup(a.Name); ok && m.Kind == KFunc {
					return c.checkCallFunc(t, n.Name+"."+a.Name, m)
				}
			}
		}
	}
	name, mod := c.callName(t.Func)
	switch mod {
	case "pickle":
		switch name {
		case "loads", "load", "Unpickler":
			c.checkSafeSink(t, argTaint, argHint, "pickle."+name)
		}
		return Taint{}, ""
	case "logging":
		c.checkMaskSink(t, argTaint, argHint, "logging")
		return Taint{}, ""
	case "os":
		switch name {
		case "system", "popen", "spawnl", "spawnlp", "spawnv", "spawnvp":
			c.checkSafeSink(t, argTaint, argHint, "os."+name)
		}
		if name == "popen" || name == "read" {
			return Taint{Safe: true}, "os." + name + "()"
		}
		return Taint{}, ""
	case "subprocess":
		c.checkSafeSink(t, argTaint, argHint, "subprocess."+name)
		if name == "check_output" || name == "run" || name == "check_call" {
			return Taint{Safe: true}, "subprocess." + name + "()"
		}
		return Taint{}, ""
	case "requests":
		switch name {
		case "get", "post", "put", "delete", "patch":
			return Taint{Safe: true}, "requests." + name + "()"
		}
		return Taint{}, ""
	case "urllib":
		if name == "urlopen" {
			return Taint{Safe: true}, "urllib." + name + "()"
		}
		return Taint{}, ""
	}
	switch name {
	case "print":
		c.checkMaskSink(t, argTaint, argHint, "print")
		return Taint{}, ""
	case "eval", "exec", "compile":
		if argTaint.Safe {
			c.errorf(t.Pos_, "未净化的外部输入 %s 流入 %s（危险汇点）", argHint, name)
		}
		if argHint != "" {
			c.markSinkParam(argHint)
		}
		return Taint{}, ""
	case "execute", "executemany":
		c.checkSafeSink(t, argTaint, argHint, "execute")
		return Taint{}, ""
	case "input":
		return Taint{Safe: true}, "input()"
	case "open", "urlopen", "read_text", "read", "readline", "readlines", "recv":
		return Taint{Safe: true}, name + "()"
	}
	if name != "" {
		if isSanitizer(name) {
			return Taint{}, ""
		}
		if sym, ok := c.cur.Lookup(name); ok && sym.Kind == KFunc {
			return c.checkCallFunc(t, name, sym)
		}
	}
	return recvTaint, recvHint
}

func (c *Checker) callName(f ast.Expr) (name, mod string) {
	switch t := f.(type) {
	case *ast.Name:
		if sym, ok := c.cur.Lookup(t.Name); ok && sym.Module != "" {
			if sym.Orig != "" {
				return sym.Orig, sym.Module
			}
			return t.Name, sym.Module
		}
		return t.Name, ""
	case *ast.AttrExpr:
		name = t.Name
		if m, ok := t.X.(*ast.Name); ok {
			if sym, ok := c.cur.Lookup(m.Name); ok && sym.Module != "" {
				return name, sym.Module
			}
			return name, m.Name
		}
		return name, ""
	}
	return "", ""
}

func (c *Checker) checkSafeSink(t *ast.CallExpr, argTaint Taint, hint, sink string) {
	if argTaint.Safe {
		c.errorf(t.Pos_, "未净化的外部输入 %s 流入 %s（危险汇点）", hint, sink)
	}
	if hint != "" {
		c.markSinkParam(hint)
	}
}

func (c *Checker) checkMaskSink(t *ast.CallExpr, argTaint Taint, hint, sink string) {
	if argTaint.Mask {
		c.errorf(t.Pos_, "敏感数据 %s 不可流入 %s（输出上下文）", hint, sink)
	}
	if hint != "" {
		c.markSinkParam(hint)
	}
}

func (c *Checker) markSinkParam(name string) {
	if c.fnSym == nil || c.fnSym.Params == nil {
		return
	}
	for _, pn := range c.fnSym.Params {
		if pn == name {
			if c.fnSym.SinkParams == nil {
				c.fnSym.SinkParams = make(map[string]bool)
			}
			c.fnSym.SinkParams[pn] = true
			return
		}
	}
}

// checkCallFunc 自定义函数调用的 taint 传播：
// 1. 返回值 taint = 函数返回 taint ∪ 实参 taint（参数透传）；
// 2. 实参流入函数体内汇点的敏感/外部数据在调用点拦截。
func (c *Checker) checkCallFunc(t *ast.CallExpr, name string, fnSym *Symbol) (Taint, string) {
	if fnSym.SinkParams != nil {
		for i, a := range t.Args {
			if i < len(fnSym.Params) && fnSym.SinkParams[fnSym.Params[i]] {
				at, an := c.exprTaint(a)
				if at.Mask {
					c.errorf(t.Pos_, "敏感数据 %s 流入函数 %s 的参数 %s（输出上下文）", an, name, fnSym.Params[i])
				}
				if at.Safe {
					c.errorf(t.Pos_, "未净化的外部输入 %s 流入函数 %s 的参数 %s（危险汇点）", an, name, fnSym.Params[i])
				}
			}
		}
	}
	argTaint, argHint := Taint{}, ""
	for _, a := range t.Args {
		at, an := c.exprTaint(a)
		argTaint = argTaint.union(at)
		if argHint == "" {
			argHint = an
		}
	}
	res := fnSym.Taint
	if fnSym.RetParam {
		res = res.union(argTaint)
	}
	if res.dirty() && argHint == "" {
		return res, name
	}
	return res, argHint
}

var fstringNameStart = func(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

var fstringNamePart = func(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9')
}

func (c *Checker) fstringTaint(t *ast.StringLit) (Taint, string) {
	v := t.Value
	if len(v) == 0 || (v[0] != 'f' && v[0] != 'F') {
		return Taint{}, ""
	}
	body := v[1:]
	if len(body) == 0 || (body[0] != '"' && body[0] != '\'') {
		return Taint{}, ""
	}
	var res Taint
	var hint string
	i := 1
	depth := 0
	for i < len(body) {
		ch := body[i]
		if ch == '\\' {
			i += 2
			continue
		}
		if depth == 0 {
			if ch == '{' && i+1 < len(body) && body[i+1] == '{' {
				i += 2
				continue
			}
			if ch == '{' {
				depth = 1
				i++
				continue
			}
			if ch == body[0] {
				break
			}
			i++
			continue
		}
		if ch == '{' {
			depth++
			i++
			continue
		}
		if ch == '}' {
			depth--
			if depth == 0 {
				i++
				continue
			}
		}
		if fstringNameStart(ch) {
			j := i
			for j < len(body) && fstringNamePart(body[j]) {
				j++
			}
			name := body[i:j]
			i = j
			if sym, ok := c.cur.Lookup(name); ok && sym.Taint.dirty() {
				res = res.union(sym.Taint)
				if hint == "" {
					hint = name
				}
			}
			continue
		}
		i++
	}
	return res, hint
}

func (c *Checker) checkTaintDecl(names []string, safe bool) {
	for _, n := range names {
		sym, ok := c.cur.Lookup(n)
		if !ok {
			sym = &Symbol{Kind: KVar, Pos: ast.Position{}}
			c.cur.Define(n, sym)
		}
		if safe {
			sym.Taint.Safe = true
		} else {
			sym.Taint.Mask = true
		}
	}
}
