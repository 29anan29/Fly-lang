package checker

import (
	"pyfly/internal/ast"
)

// runtimeCheck 一趟静态检查，覆盖可静态确定的运行时错误：
// 未定义名称（模块级顺序敏感、函数体宽松）、参数个数不匹配、常量除零、
// 字面量类型操作非法。保证通过 check 的代码无原生 Python 运行时崩溃
// （其余动态场景由 gen 注入 _fly_* 兜底函数统一转 FlyRuntimeError）。
func (c *Checker) runtimeCheck(m *ast.Module) {
	u := &uCheck{c: c, globals: collectAll(c.global)}
	defined := make(map[string]bool)
	u.moduleSeq(m.Stmts, defined)
}

func collectAll(root *Scope) map[string]bool {
	all := make(map[string]bool)
	var walk func(s *Scope)
	walk = func(s *Scope) {
		for n := range s.Names {
			all[n] = true
		}
		if s.Parent != nil {
			walk(s.Parent)
		}
	}
	walk(root)
	return all
}

type uCheck struct {
	c       *Checker
	globals map[string]bool
	star    bool // 出现 from x import * → 未定义检查放行全部名字
	inOnly  int  // only 块内：名字访问由 only 白名单检查负责
}

func (u *uCheck) defined(name string, defined map[string]bool) bool {
	return defined[name] || u.globals[name] || builtins[name] || u.star
}

// moduleSeq 模块级/类体顺序敏感检查：名字须在引用前已定义。
func (u *uCheck) moduleSeq(stmts []ast.Stmt, defined map[string]bool) {
	for _, s := range stmts {
		u.stmtSeq(s, defined)
	}
}

func (u *uCheck) stmtSeq(s ast.Stmt, defined map[string]bool) {
	switch t := s.(type) {
	case *ast.ImportStmt:
		for _, it := range t.Items {
			defined[importedName(it)] = true
		}
	case *ast.FromImportStmt:
		for _, it := range t.Items {
			if it.Name == "*" {
				u.star = true
				continue
			}
			defined[importedName(it)] = true
		}
	case *ast.AssignStmt:
		for _, l := range t.Left {
			u.assignTargetSeq(l, defined, false)
		}
		u.exprSeq(t.Right, defined, false)
		for _, l := range t.Left {
			u.defineTarget(l, defined)
		}
	case *ast.DeleteStmt:
		for _, tg := range t.Targets {
			if n, ok := tg.(*ast.Name); ok {
				u.checkName(n, defined)
				delete(defined, n.Name)
			}
		}
	case *ast.LockStmt:
		u.exprSeq(t.Value, defined, false)
		defined[t.Name] = true
	case *ast.SafeStmt:
		for _, n := range t.Names {
			if !u.defined(n, defined) {
				u.errorf(t.Pos_, "未定义的名字 %s（safe 需要先赋值）", n)
			}
		}
	case *ast.MaskStmt:
		for _, n := range t.Names {
			if !u.defined(n, defined) {
				u.errorf(t.Pos_, "未定义的名字 %s（mask 需要先赋值）", n)
			}
		}
	case *ast.GuardStmt:
	case *ast.FuncDef:
		for _, d := range t.Decorators {
			u.exprSeq(d, defined, false)
		}
		defined[t.Name] = true
		u.fnBody(t, defined)
	case *ast.ClassDef:
		for _, d := range t.Decorators {
			u.exprSeq(d, defined, false)
		}
		for _, b := range t.Bases {
			u.exprSeq(b, defined, false)
		}
		defined[t.Name] = true
		u.classBody(t, defined)
	case *ast.OnlyStmt:
		for _, mod := range t.Modules {
			defined[mod] = true
		}
		u.inOnly++
		for _, st := range t.Body {
			u.stmtSeq(st, defined)
		}
		u.inOnly--
	case *ast.TraceStmt:
		for _, st := range t.Body {
			u.stmtSeq(st, defined)
		}
	case *ast.CageStmt:
		for _, st := range t.Body {
			u.stmtSeq(st, defined)
		}
	case *ast.IfStmt:
		u.exprSeq(t.Cond, defined, false)
		u.moduleSeq(t.Then, defined)
		for _, el := range t.Elifs {
			u.exprSeq(el.Cond, defined, false)
			u.moduleSeq(el.Body, defined)
		}
		u.moduleSeq(t.Else, defined)
	case *ast.ForStmt:
		u.exprSeq(t.Iter, defined, false)
		u.defineTarget(t.Target, defined)
		u.moduleSeq(t.Body, defined)
		u.moduleSeq(t.Else, defined)
	case *ast.WhileStmt:
		u.exprSeq(t.Cond, defined, false)
		u.moduleSeq(t.Body, defined)
		u.moduleSeq(t.Else, defined)
	case *ast.TryStmt:
		u.moduleSeq(t.Body, defined)
		for _, h := range t.Handlers {
			u.exprSeq(h.Type, defined, false)
			if h.Name != "" {
				defined[h.Name] = true
			}
			u.moduleSeq(h.Body, defined)
		}
		u.moduleSeq(t.Else, defined)
		u.moduleSeq(t.Finally, defined)
	case *ast.ReturnStmt:
		u.exprSeq(t.Value, defined, false)
	case *ast.RaiseStmt:
		u.exprSeq(t.Exc, defined, false)
		u.exprSeq(t.From, defined, false)
	case *ast.ExprStmt:
		u.exprSeq(t.X, defined, false)
	}
}

// classBody 类体顺序敏感：可见 = 类内前序 + 模块级全量 + 内置。
// 方法体按函数规则单独宽松检查。
func (u *uCheck) classBody(t *ast.ClassDef, outer map[string]bool) {
	defined := make(map[string]bool)
	for n := range outer {
		defined[n] = true
	}
	u.moduleSeq(t.Body, defined)
}

// fnBody 函数体宽松检查：可见 = 参数 + 函数内全部局部 + 外层可见集 + 内置。
func (u *uCheck) fnBody(t *ast.FuncDef, outer map[string]bool) {
	visible := make(map[string]bool)
	for n := range outer {
		visible[n] = true
	}
	for _, p := range t.Params {
		if p.Name != "" {
			visible[p.Name] = true
		}
	}
	u.collectLocals(t.Body, visible)
	for _, st := range t.Body {
		u.stmtFn(st, visible)
	}
}

func (u *uCheck) collectLocals(stmts []ast.Stmt, visible map[string]bool) {
	for _, s := range stmts {
		switch t := s.(type) {
		case *ast.ImportStmt:
			for _, it := range t.Items {
				visible[importedName(it)] = true
			}
		case *ast.FromImportStmt:
			for _, it := range t.Items {
				if it.Name == "*" {
					u.star = true
					continue
				}
				visible[importedName(it)] = true
			}
		case *ast.AssignStmt:
			for _, l := range t.Left {
				u.defineTarget(l, visible)
			}
		case *ast.LockStmt:
			visible[t.Name] = true
		case *ast.SafeStmt:
			for _, n := range t.Names {
				visible[n] = true
			}
		case *ast.MaskStmt:
			for _, n := range t.Names {
				visible[n] = true
			}
		case *ast.FuncDef:
			visible[t.Name] = true
			u.collectLocals(t.Body, visible)
		case *ast.ClassDef:
			visible[t.Name] = true
		case *ast.OnlyStmt:
			u.collectLocals(t.Body, visible)
		case *ast.TraceStmt:
			u.collectLocals(t.Body, visible)
		case *ast.CageStmt:
			u.collectLocals(t.Body, visible)
		case *ast.IfStmt:
			u.collectLocals(t.Then, visible)
			for _, el := range t.Elifs {
				u.collectLocals(el.Body, visible)
			}
			u.collectLocals(t.Else, visible)
		case *ast.ForStmt:
			u.defineTarget(t.Target, visible)
			u.collectLocals(t.Body, visible)
			u.collectLocals(t.Else, visible)
		case *ast.WhileStmt:
			u.collectLocals(t.Body, visible)
			u.collectLocals(t.Else, visible)
		case *ast.TryStmt:
			u.collectLocals(t.Body, visible)
			for _, h := range t.Handlers {
				if h.Name != "" {
					visible[h.Name] = true
				}
				u.collectLocals(h.Body, visible)
			}
			u.collectLocals(t.Else, visible)
			u.collectLocals(t.Finally, visible)
		}
	}
}

// stmtFn 函数体内语句检查（宽松模式：visible 即全部可见名）。
func (u *uCheck) stmtFn(s ast.Stmt, visible map[string]bool) {
	switch t := s.(type) {
	case *ast.AssignStmt:
		for _, l := range t.Left {
			u.assignTargetSeq(l, visible, true)
		}
		u.exprSeq(t.Right, visible, true)
	case *ast.DeleteStmt:
		for _, tg := range t.Targets {
			if n, ok := tg.(*ast.Name); ok {
				u.checkName(n, visible)
				delete(visible, n.Name)
			}
		}
	case *ast.LockStmt:
		u.exprSeq(t.Value, visible, true)
	case *ast.GuardStmt:
	case *ast.FuncDef:
		u.fnBody(t, visible)
	case *ast.ClassDef:
		u.classBody(t, visible)
	case *ast.OnlyStmt:
		for _, st := range t.Body {
			u.stmtFn(st, visible)
		}
	case *ast.TraceStmt:
		for _, st := range t.Body {
			u.stmtFn(st, visible)
		}
	case *ast.CageStmt:
		for _, st := range t.Body {
			u.stmtFn(st, visible)
		}
	case *ast.IfStmt:
		u.exprSeq(t.Cond, visible, true)
		for _, st := range t.Then {
			u.stmtFn(st, visible)
		}
		for _, el := range t.Elifs {
			u.exprSeq(el.Cond, visible, true)
			for _, st := range el.Body {
				u.stmtFn(st, visible)
			}
		}
		for _, st := range t.Else {
			u.stmtFn(st, visible)
		}
	case *ast.ForStmt:
		u.exprSeq(t.Iter, visible, true)
		for _, st := range t.Body {
			u.stmtFn(st, visible)
		}
		for _, st := range t.Else {
			u.stmtFn(st, visible)
		}
	case *ast.WhileStmt:
		u.exprSeq(t.Cond, visible, true)
		for _, st := range t.Body {
			u.stmtFn(st, visible)
		}
		for _, st := range t.Else {
			u.stmtFn(st, visible)
		}
	case *ast.TryStmt:
		for _, st := range t.Body {
			u.stmtFn(st, visible)
		}
		for _, h := range t.Handlers {
			u.exprSeq(h.Type, visible, true)
			for _, st := range h.Body {
				u.stmtFn(st, visible)
			}
		}
		for _, st := range t.Else {
			u.stmtFn(st, visible)
		}
		for _, st := range t.Finally {
			u.stmtFn(st, visible)
		}
	case *ast.ReturnStmt:
		u.exprSeq(t.Value, visible, true)
	case *ast.RaiseStmt:
		u.exprSeq(t.Exc, visible, true)
		u.exprSeq(t.From, visible, true)
	case *ast.ExprStmt:
		u.exprSeq(t.X, visible, true)
	}
}

func (u *uCheck) defineTarget(e ast.Expr, defined map[string]bool) {
	switch t := e.(type) {
	case *ast.Name:
		defined[t.Name] = true
	case *ast.TupleLit:
		for _, el := range t.Elems {
			u.defineTarget(el, defined)
		}
	case *ast.ListLit:
		for _, el := range t.Elems {
			u.defineTarget(el, defined)
		}
	}
}

// assignTargetSeq 赋值左侧：Name 是定义不检查；下标/属性访问的基表达式需已定义。
func (u *uCheck) assignTargetSeq(e ast.Expr, defined map[string]bool, fnMode bool) {
	switch t := e.(type) {
	case *ast.Name:
	case *ast.TupleLit:
		for _, el := range t.Elems {
			u.assignTargetSeq(el, defined, fnMode)
		}
	case *ast.ListLit:
		for _, el := range t.Elems {
			u.assignTargetSeq(el, defined, fnMode)
		}
	case *ast.SubscriptExpr:
		u.exprSeq(t.X, defined, fnMode)
		u.exprSeq(t.Index, defined, fnMode)
	case *ast.AttrExpr:
		u.exprSeq(t.X, defined, fnMode)
	default:
		u.exprSeq(e, defined, fnMode)
	}
}

// exprSeq 递归检查表达式：未定义名称、常量除零、字面量类型操作、参数个数。
func (u *uCheck) exprSeq(e ast.Expr, defined map[string]bool, fnMode bool) {
	switch t := e.(type) {
	case *ast.Name:
		u.checkName(t, defined)
	case *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.EllipsisLit:
	case *ast.ListLit:
		for _, el := range t.Elems {
			u.exprSeq(el, defined, fnMode)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			u.exprSeq(el, defined, fnMode)
		}
	case *ast.DictLit:
		for i := range t.Keys {
			u.exprSeq(t.Keys[i], defined, fnMode)
			u.exprSeq(t.Vals[i], defined, fnMode)
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			u.exprSeq(el, defined, fnMode)
		}
	case *ast.CallExpr:
		u.exprSeq(t.Func, defined, fnMode)
		for _, a := range t.Args {
			u.exprSeq(a, defined, fnMode)
		}
		u.exprSeq(t.Star, defined, fnMode)
		u.exprSeq(t.DblStar, defined, fnMode)
		for _, kw := range t.Kwargs {
			u.exprSeq(kw.Value, defined, fnMode)
		}
		u.checkArgs(t)
	case *ast.AttrExpr:
		u.exprSeq(t.X, defined, fnMode)
	case *ast.SliceExpr:
		u.exprSeq(t.Lo, defined, fnMode)
		u.exprSeq(t.Hi, defined, fnMode)
		u.exprSeq(t.Step, defined, fnMode)
	case *ast.SubscriptExpr:
		u.exprSeq(t.X, defined, fnMode)
		u.exprSeq(t.Index, defined, fnMode)
		u.checkSub(t)
	case *ast.BinOpExpr:
		u.exprSeq(t.X, defined, fnMode)
		u.exprSeq(t.Y, defined, fnMode)
		u.checkBinop(t)
	case *ast.UnaryOpExpr:
		u.exprSeq(t.X, defined, fnMode)
		u.checkUnary(t)
	case *ast.BoolOpExpr:
		u.exprSeq(t.X, defined, fnMode)
		u.exprSeq(t.Y, defined, fnMode)
	case *ast.CompareExpr:
		u.exprSeq(t.X, defined, fnMode)
		for i, y := range t.Ys {
			u.exprSeq(y, defined, fnMode)
			u.checkCmp(t.Ops[i], t.X, y)
		}
	case *ast.CondExpr:
		u.exprSeq(t.Cond, defined, fnMode)
		u.exprSeq(t.Then, defined, fnMode)
		u.exprSeq(t.Else, defined, fnMode)
	case *ast.ListComp:
		inner := make(map[string]bool)
		for n := range defined {
			inner[n] = true
		}
		for _, cl := range t.Clauses {
			u.exprSeq(cl.Iter, inner, true)
			u.defineTarget(cl.Target, inner)
			for _, f := range cl.Ifs {
				u.exprSeq(f, inner, true)
			}
		}
		u.exprSeq(t.Elem, inner, true)
	}
}

func (u *uCheck) checkName(t *ast.Name, defined map[string]bool) {
	if u.inOnly > 0 {
		return
	}
	if !u.defined(t.Name, defined) {
		u.errorf(t.Pos_, "未定义的名字 %s", t.Name)
	}
}

func (u *uCheck) errorf(pos ast.Position, format string, args ...interface{}) {
	u.c.errorf(pos, format, args...)
}

// checkArgs 参数个数检查：仅对本地定义的函数（KFunc 有 Func 定义）。
func (u *uCheck) checkArgs(t *ast.CallExpr) {
	n, ok := t.Func.(*ast.Name)
	if !ok {
		return
	}
	sym, ok := u.c.global.Lookup(n.Name)
	if !ok || sym.Kind != KFunc || sym.Func == nil {
		return
	}
	if t.Star != nil || t.DblStar != nil {
		return
	}
	fn := sym.Func
	req := 0
	total := 0
	names := make([]string, 0, len(fn.Params))
	for _, p := range fn.Params {
		if p.Name == "" {
			continue
		}
		if !p.DblStar {
			names = append(names, p.Name)
		}
		total++
		if p.Default == nil && !p.Star && !p.DblStar {
			req++
		}
	}
	if len(t.Args) < req {
		provided := len(t.Args)
		for _, kw := range t.Kwargs {
			for _, p := range fn.Params {
				if p.Name == kw.Name && p.Default == nil && !p.Star && !p.DblStar {
					provided++
				}
			}
		}
		if provided < req {
			u.errorf(t.Pos_, "函数 %s 需要至少 %d 个参数（实际 %d 个）", n.Name, req, len(t.Args))
			return
		}
	}
	if len(t.Args) > len(names) {
		u.errorf(t.Pos_, "函数 %s 最多接受 %d 个位置参数（实际 %d 个）", n.Name, len(names), len(t.Args))
		return
	}
	for _, kw := range t.Kwargs {
		found := false
		for _, pn := range names {
			if pn == kw.Name {
				found = true
			}
		}
		if !found {
			u.errorf(t.Pos_, "函数 %s 没有名为 %s 的参数", n.Name, kw.Name)
			return
		}
	}
	if len(t.Args) <= len(names) {
		for _, kw := range t.Kwargs {
			for i := 0; i < len(t.Args); i++ {
				if fn.Params[i].Name == kw.Name {
					u.errorf(t.Pos_, "函数 %s 参数 %s 重复传值", n.Name, kw.Name)
					return
				}
			}
		}
	}
}

func (u *uCheck) checkBinop(t *ast.BinOpExpr) {
	x, y := litType(t.X), litType(t.Y)
	if x == "" || y == "" {
		return
	}
	if t.Op == "%" && x == "str" {
		return
	}
	if (t.Op == "/" || t.Op == "//" || t.Op == "%") && isZero(t.Y) && numType(x) {
		u.errorf(t.Pos_, "常量表达式除数为零")
		return
	}
	if !binopOK(t.Op, x, y) {
		u.errorf(t.Pos_, "运算符 %s 不支持 %s 与 %s", t.Op, x, y)
	}
}

func (u *uCheck) checkUnary(t *ast.UnaryOpExpr) {
	if t.Op == "not" {
		return
	}
	x := litType(t.X)
	if x == "" {
		return
	}
	if t.Op == "~" && x != "int" && x != "bool" {
		u.errorf(t.Pos_, "运算符 ~ 不支持 %s", x)
	}
	if (t.Op == "-" || t.Op == "+") && !numType(x) {
		u.errorf(t.Pos_, "运算符 %s 不支持 %s", t.Op, x)
	}
}

func (u *uCheck) checkCmp(op string, x, y ast.Expr) {
	xt, yt := litType(x), litType(y)
	if xt == "" || yt == "" {
		return
	}
	if op == "in" || op == "not in" {
		if yt == "int" || yt == "float" || yt == "bool" || yt == "none" {
			u.errorf(y.Pos(), "in 右侧不支持 %s", yt)
		}
		return
	}
	if !cmpOK(op, xt, yt) {
		u.errorf(x.Pos(), "运算符 %s 不支持 %s 与 %s", op, xt, yt)
	}
}

func (u *uCheck) checkSub(t *ast.SubscriptExpr) {
	xt := litType(t.X)
	if xt == "" {
		return
	}
	idx := ""
	if _, ok := t.Index.(*ast.SliceExpr); ok {
		idx = "slice"
	} else {
		idx = litType(t.Index)
	}
	if !subOK(xt, idx) {
		u.errorf(t.Pos_, "%s 不可下标访问", xt)
	}
}

func litType(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.IntLit:
		return "int"
	case *ast.FloatLit:
		return "float"
	case *ast.StringLit:
		return "str"
	case *ast.ListLit:
		return "list"
	case *ast.DictLit:
		return "dict"
	case *ast.SetLit:
		return "set"
	case *ast.TupleLit:
		return "tuple"
	case *ast.Name:
		switch t.Name {
		case "None":
			return "none"
		case "True", "False":
			return "bool"
		}
	}
	return ""
}

func isZero(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.IntLit:
		return t.Value == "0"
	case *ast.FloatLit:
		return t.Value == "0" || t.Value == "0.0"
	}
	return false
}

func numType(t string) bool {
	return t == "int" || t == "float" || t == "bool"
}

func binopOK(op, x, y string) bool {
	switch op {
	case "+":
		return numType(x) && numType(y) || (x == y && (x == "str" || x == "list" || x == "tuple"))
	case "-", "/", "//", "%":
		return numType(x) && numType(y)
	case "*":
		if numType(x) && numType(y) {
			return true
		}
		return (x == "str" || x == "list" || x == "tuple") && y == "int" ||
			x == "int" && (y == "str" || y == "list" || y == "tuple")
	case "**":
		return numType(x) && numType(y)
	case "<<", ">>", "&", "|", "^":
		return (x == "int" || x == "bool") && (y == "int" || y == "bool")
	}
	return true
}

func cmpOK(op, x, y string) bool {
	if numType(x) && numType(y) {
		return true
	}
	if x == y {
		return x == "str" || x == "list" || x == "tuple"
	}
	return false
}

func subOK(x, idx string) bool {
	switch x {
	case "str", "list", "tuple":
		return idx == "int" || idx == "slice"
	case "dict":
		return true
	}
	return false
}
