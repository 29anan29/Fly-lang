// only.go：only 白名单编译期检查——块内只允许名单内模块/名字（E0044/E0045），跨文件收集引用。
package checker

import (
	"strings"

	"pyfly/internal/ast"
)

var onlyDeny = map[string]bool{
	"os": true, "subprocess": true, "sys": true, "eval": true, "exec": true,
	"compile": true, "__import__": true, "open": true, "getattr": true,
	"setattr": true, "delattr": true, "globals": true, "vars": true, "locals": true,
	"input": true, "socket": true, "shutil": true, "ctypes": true, "pickle": true,
	"marshal": true, "importlib": true, "runpy": true, "code": true, "pty": true,
	"requests": true, "urllib": true, "pathlib": true, "tempfile": true,
	"sqlite3": true, "base64": true, "shelve": true, "dbm": true, "webbrowser": true,
}

func (c *Checker) checkOnly(t *ast.OnlyStmt) {
	if len(t.Modules) == 0 {
		c.errorf(t.Pos_, "only 白名单不能为空（需至少一个模块，如 only (json):）")
	}
	refs := collectRefs(t.Body)
	allowed := map[string]bool{}
	for _, m := range t.Modules {
		allowed[m] = true
	}
	for _, r := range refs {
		if allowed[r.Name] {
			continue
		}
		if onlyDeny[r.Name] {
			c.errorf(r.Pos(), "only 块禁止访问 %s（不在白名单 %v）", r.Name, t.Modules)
		}
	}
	for _, st := range t.Body {
		c.checkStmt(st)
	}
}

func (c *Checker) checkTrace(t *ast.TraceStmt) {
	switch t.Level {
	case "DEBUG", "INFO", "WARNING", "WARN", "ERROR", "CRITICAL":
	default:
		c.errorf(t.Pos_, "trace 级别 %s 非法（支持 DEBUG/INFO/WARNING/ERROR/CRITICAL）", t.Level)
	}
	for _, st := range t.Body {
		c.checkStmt(st)
	}
	for _, st := range t.Body {
		if f, ok := st.(*ast.FuncDef); ok {
			if strings.HasPrefix(f.Name, "_fly_") {
				c.errorf(f.Pos_, "trace 块内函数名 %s 不能以 _fly_ 开头（保留前缀）", f.Name)
			}
			for _, p := range f.Params {
				if strings.HasPrefix(p.Name, "_fly_") {
					c.errorf(f.Pos_, "trace 块内参数名 %s 不能以 _fly_ 开头（保留前缀）", p.Name)
				}
			}
		}
	}
}

func (c *Checker) checkSealInstanceAssign(e ast.Expr) {
	t, ok := e.(*ast.AttrExpr)
	if !ok {
		return
	}
	if n, ok := t.X.(*ast.Name); ok {
		if sym, ok := c.cur.Lookup(n.Name); ok && sym.Seal {
			c.errorf(t.Pos_, "seal 类实例 %s 的属性 %s 不可修改", n.Name, t.Name)
		}
	}
}

func (c *Checker) markSealInstances(left []ast.Expr, right ast.Expr) {
	cl, ok := right.(*ast.CallExpr)
	if !ok {
		return
	}
	f, ok := cl.Func.(*ast.Name)
	if !ok {
		return
	}
	sym, ok := c.cur.Lookup(f.Name)
	if !ok || sym.Kind != KClass || !sym.Seal {
		return
	}
	for _, l := range left {
		if n, ok := l.(*ast.Name); ok {
			if s, ok := c.cur.Lookup(n.Name); ok {
				s.Seal = true
			}
		}
	}
}

type refCollector struct {
	refs []ast.Name
	skip map[string]bool
}

func collectRefs(stmts []ast.Stmt) []ast.Name {
	rc := &refCollector{skip: map[string]bool{}}
	for _, s := range stmts {
		rc.stmt(s)
	}
	return rc.refs
}

func (rc *refCollector) stmt(s ast.Stmt) {
	switch t := s.(type) {
	case *ast.ImportStmt:
		for _, it := range t.Items {
			rc.ref(it.Name, t.Pos_)
		}
	case *ast.FromImportStmt:
		rc.ref(t.Module, t.Pos_)
	case *ast.AssignStmt:
		for _, l := range t.Left {
			rc.target(l)
		}
		rc.expr(t.Right)
	case *ast.ExprStmt:
		rc.expr(t.X)
	case *ast.FuncDef:
		rc.skip[t.Name] = true
		for _, p := range t.Params {
			if p.Name != "" {
				rc.skip[p.Name] = true
			}
		}
		for _, d := range t.Decorators {
			rc.expr(d)
		}
		if t.ReturnType != nil {
			rc.expr(t.ReturnType)
		}
		for _, st := range t.Body {
			rc.stmt(st)
		}
	case *ast.ClassDef:
		rc.skip[t.Name] = true
		for _, b := range t.Bases {
			rc.expr(b)
		}
		for _, d := range t.Decorators {
			rc.expr(d)
		}
		for _, st := range t.Body {
			rc.stmt(st)
		}
	case *ast.IfStmt:
		rc.expr(t.Cond)
		for _, st := range t.Then {
			rc.stmt(st)
		}
		for _, el := range t.Elifs {
			rc.expr(el.Cond)
			for _, st := range el.Body {
				rc.stmt(st)
			}
		}
		for _, st := range t.Else {
			rc.stmt(st)
		}
	case *ast.ForStmt:
		rc.target(t.Target)
		rc.expr(t.Iter)
		for _, st := range t.Body {
			rc.stmt(st)
		}
		for _, st := range t.Else {
			rc.stmt(st)
		}
	case *ast.WhileStmt:
		rc.expr(t.Cond)
		for _, st := range t.Body {
			rc.stmt(st)
		}
		for _, st := range t.Else {
			rc.stmt(st)
		}
	case *ast.TryStmt:
		for _, st := range t.Body {
			rc.stmt(st)
		}
		for _, h := range t.Handlers {
			rc.expr(h.Type)
			if h.Name != "" {
				rc.skip[h.Name] = true
			}
			for _, st := range h.Body {
				rc.stmt(st)
			}
		}
		for _, st := range t.Else {
			rc.stmt(st)
		}
		for _, st := range t.Finally {
			rc.stmt(st)
		}
	case *ast.ReturnStmt:
		rc.expr(t.Value)
	case *ast.RaiseStmt:
		rc.expr(t.Exc)
		rc.expr(t.From)
	case *ast.DeleteStmt:
		for _, tg := range t.Targets {
			rc.expr(tg)
		}
	case *ast.LockStmt:
		rc.skip[t.Name] = true
		rc.expr(t.Value)
	case *ast.SafeStmt:
		for _, n := range t.Names {
			rc.skip[n] = true
		}
	case *ast.MaskStmt:
		for _, n := range t.Names {
			rc.skip[n] = true
		}
	case *ast.GuardStmt:
		rc.ref(t.Name, t.Pos_)
		rc.expr(t.Type)
		for _, cd := range t.Conds {
			rc.expr(cd)
		}
	case *ast.OnlyStmt:
		for _, st := range t.Body {
			rc.stmt(st)
		}
	case *ast.TraceStmt:
		for _, st := range t.Body {
			rc.stmt(st)
		}
	}
}

func (rc *refCollector) target(e ast.Expr) {
	switch t := e.(type) {
	case *ast.Name:
		rc.skip[t.Name] = true
	case *ast.TupleLit:
		for _, el := range t.Elems {
			rc.target(el)
		}
	case *ast.ListLit:
		for _, el := range t.Elems {
			rc.target(el)
		}
	case *ast.AttrExpr:
		rc.expr(t.X)
	case *ast.SubscriptExpr:
		rc.expr(t.X)
		rc.expr(t.Index)
	case *ast.UnaryOpExpr:
		rc.target(t.X)
	}
}

func (rc *refCollector) expr(e ast.Expr) {
	if e == nil {
		return
	}
	switch t := e.(type) {
	case *ast.Name:
		rc.ref(t.Name, t.Pos_)
	case *ast.ListLit:
		for _, el := range t.Elems {
			rc.expr(el)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			rc.expr(el)
		}
	case *ast.DictLit:
		for i := range t.Keys {
			rc.expr(t.Keys[i])
			rc.expr(t.Vals[i])
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			rc.expr(el)
		}
	case *ast.CallExpr:
		rc.expr(t.Func)
		for _, a := range t.Args {
			rc.expr(a)
		}
		rc.expr(t.Star)
		rc.expr(t.DblStar)
		for _, kw := range t.Kwargs {
			rc.expr(kw.Value)
		}
	case *ast.AttrExpr:
		rc.expr(t.X)
	case *ast.SubscriptExpr:
		rc.expr(t.X)
		rc.expr(t.Index)
	case *ast.SliceExpr:
		rc.expr(t.Lo)
		rc.expr(t.Hi)
		rc.expr(t.Step)
	case *ast.BinOpExpr:
		rc.expr(t.X)
		rc.expr(t.Y)
	case *ast.UnaryOpExpr:
		rc.expr(t.X)
	case *ast.BoolOpExpr:
		rc.expr(t.X)
		rc.expr(t.Y)
	case *ast.CompareExpr:
		rc.expr(t.X)
		for _, y := range t.Ys {
			rc.expr(y)
		}
	case *ast.CondExpr:
		rc.expr(t.Cond)
		rc.expr(t.Then)
		rc.expr(t.Else)
	case *ast.ListComp:
		rc.expr(t.Elem)
		for _, cl := range t.Clauses {
			rc.target(cl.Target)
			rc.expr(cl.Iter)
			for _, f := range cl.Ifs {
				rc.expr(f)
			}
		}
	}
}

func (rc *refCollector) ref(name string, pos ast.Position) {
	if rc.skip[name] {
		return
	}
	rc.refs = append(rc.refs, ast.Name{Pos_: pos, Name: name})
}
