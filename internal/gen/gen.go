package gen

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"flylang/internal/ast"
	"flylang/internal/runtime"
)

type Gen struct {
	buf       bytes.Buffer
	indent    int
	onlyUsed  bool
	sealUsed  bool
	traceUsed bool
	plain     bool // 纯文本模式（guard 消息等）：不注入 _fly_* 兜底
	traceCtx  *traceCtx
	sealInit  bool
	nc        int
	types     *TypeInfer
	curFn     string // 当前生成的函数名（类型推导作用域）
}

type traceCtx struct {
	level string
	args  bool
	ret   bool
	name  string
}

func Generate(m *ast.Module) string {
	g := &Gen{types: NewTypeInfer(m)}
	g.types.Run()
	docEnd := 0
	if len(m.Stmts) > 0 {
		if es, ok := m.Stmts[0].(*ast.ExprStmt); ok {
			if _, ok := es.X.(*ast.StringLit); ok {
				docEnd = 1
			}
		}
	}
	guard := needsGuard(m.Stmts)
	only := needsOnly(m.Stmts)
	trace := needsTrace(m.Stmts)
	cage := needsCage(m.Stmts)
	// 沙箱恒注入：所有编译产物在沙箱内运行（拦截逃逸内建/反射链/危险模块导入）。
	// runtime 节随沙箱恒注入（sandbox 依赖 FlyRuntimeError 等定义）。
	sandbox := true
	rt := true
	for i, s := range m.Stmts {
		if i == docEnd {
			if docEnd == 1 {
				g.w("\n")
			}
			g.runtimePrelude(guard, only, trace, cage, rt, sandbox)
		}
		g.stmt(s)
	}
	return g.buf.String()
}

func (g *Gen) runtimePrelude(guard, only, trace, cage, rt, sandbox bool) {
	for _, n := range []string{"guard", "only", "trace", "cage", "runtime", "sandbox"} {
		need := (n == "guard" && guard) || (n == "only" && only) || (n == "trace" && trace) || (n == "cage" && cage) || (n == "runtime" && rt) || (n == "sandbox" && sandbox)
		if !need {
			continue
		}
		if sec := runtime.Section(n); sec != "" {
			g.w(sec)
			g.w("\n")
		}
	}
}

// needsRuntime 判断是否需要注入运行时兜底（_fly_binop/_fly_get 等）。
func needsRuntime(stmts []ast.Stmt) bool {
	found := false
	var walkExpr func(e ast.Expr)
	var walkStmt func(s ast.Stmt)
	walkExpr = func(e ast.Expr) {
		if e == nil || found {
			return
		}
		switch t := e.(type) {
		case *ast.Name, *ast.IntLit, *ast.FloatLit, *ast.StringLit, *ast.EllipsisLit:
		case *ast.ListLit:
			for _, el := range t.Elems {
				walkExpr(el)
			}
		case *ast.TupleLit:
			for _, el := range t.Elems {
				walkExpr(el)
			}
		case *ast.DictLit:
			for i := range t.Keys {
				walkExpr(t.Keys[i])
				walkExpr(t.Vals[i])
			}
		case *ast.SetLit:
			for _, el := range t.Elems {
				walkExpr(el)
			}
		case *ast.CallExpr:
			if n, ok := t.Func.(*ast.Name); ok && (n.Name == "int" || n.Name == "float") {
				found = true
				return
			}
			walkExpr(t.Func)
			for _, a := range t.Args {
				walkExpr(a)
			}
			walkExpr(t.Star)
			walkExpr(t.DblStar)
			for _, kw := range t.Kwargs {
				walkExpr(kw.Value)
			}
		case *ast.AttrExpr:
			found = true
		case *ast.SubscriptExpr:
			found = true
		case *ast.BinOpExpr:
			found = true
		case *ast.UnaryOpExpr:
			if t.Op != "not" {
				found = true
			}
		case *ast.CompareExpr:
			found = true
		case *ast.CondExpr:
			walkExpr(t.Cond)
			walkExpr(t.Then)
			walkExpr(t.Else)
		case *ast.SliceExpr:
			walkExpr(t.Lo)
			walkExpr(t.Hi)
			walkExpr(t.Step)
		case *ast.ListComp:
			found = true
		}
	}
	walkStmt = func(s ast.Stmt) {
		if found {
			return
		}
		switch t := s.(type) {
		case *ast.AssignStmt:
			walkExpr(t.Right)
		case *ast.LockStmt:
			walkExpr(t.Value)
		case *ast.ExprStmt:
			walkExpr(t.X)
		case *ast.FuncDef:
			for _, d := range t.Decorators {
				walkExpr(d)
			}
			for _, p := range t.Params {
				if p.Default != nil {
					walkExpr(p.Default)
				}
			}
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.ClassDef:
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.OnlyStmt:
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.TraceStmt:
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.CageStmt:
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.IfStmt:
			walkExpr(t.Cond)
			for _, st := range t.Then {
				walkStmt(st)
			}
			for _, el := range t.Elifs {
				walkExpr(el.Cond)
				for _, st := range el.Body {
					walkStmt(st)
				}
			}
			for _, st := range t.Else {
				walkStmt(st)
			}
		case *ast.ForStmt:
			found = true
			walkExpr(t.Iter)
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.WhileStmt:
			walkExpr(t.Cond)
			for _, st := range t.Body {
				walkStmt(st)
			}
		case *ast.TryStmt:
			for _, st := range t.Body {
				walkStmt(st)
			}
			for _, h := range t.Handlers {
				for _, st := range h.Body {
					walkStmt(st)
				}
			}
		case *ast.ReturnStmt:
			walkExpr(t.Value)
		case *ast.RaiseStmt:
			walkExpr(t.Exc)
		case *ast.GuardStmt:
			walkExpr(t.Type)
			for _, c := range t.Conds {
				walkExpr(c)
			}
		}
	}
	for _, s := range stmts {
		walkStmt(s)
		if found {
			return true
		}
	}
	return found
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
		case *ast.OnlyStmt:
			if needsOnly(t.Body) {
				return true
			}
		case *ast.TraceStmt:
			if needsTrace(t.Body) {
				return true
			}
		}
	}
	return false
}

func needsOnly(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.OnlyStmt); ok {
			return true
		}
		if needStmt(needsOnly, s) {
			return true
		}
	}
	return false
}

func needsTrace(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.TraceStmt); ok {
			return true
		}
		if needStmt(needsTrace, s) {
			return true
		}
	}
	return false
}

func needsCage(stmts []ast.Stmt) bool {
	for _, s := range stmts {
		if _, ok := s.(*ast.CageStmt); ok {
			return true
		}
		if needStmt(needsCage, s) {
			return true
		}
	}
	return false
}

func needStmt(scan func([]ast.Stmt) bool, s ast.Stmt) bool {
	switch t := s.(type) {
	case *ast.FuncDef:
		return scan(t.Body)
	case *ast.ClassDef:
		return scan(t.Body)
	case *ast.IfStmt:
		if scan(t.Then) || scan(t.Else) {
			return true
		}
		for _, el := range t.Elifs {
			if scan(el.Body) {
				return true
			}
		}
	case *ast.ForStmt:
		return scan(t.Body) || scan(t.Else)
	case *ast.WhileStmt:
		return scan(t.Body) || scan(t.Else)
	case *ast.TryStmt:
		if scan(t.Body) || scan(t.Else) || scan(t.Finally) {
			return true
		}
		for _, h := range t.Handlers {
			if scan(h.Body) {
				return true
			}
		}
	case *ast.OnlyStmt:
		return scan(t.Body)
	case *ast.TraceStmt:
		return scan(t.Body)
	case *ast.CageStmt:
		return scan(t.Body)
	}
	return false
}

func moduleRoot(name string) string {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return name[:i]
		}
	}
	return name
}

func (g *Gen) indentLine() {
	g.buf.WriteString(strings.Repeat("    ", g.indent))
}

func (g *Gen) render(e ast.Expr) string {
	if e == nil {
		return ""
	}
	sub := &Gen{plain: true}
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
		for i, it := range t.Items {
			if i > 0 {
				g.w("\n")
			}
			g.indentLine()
			bind := moduleRoot(it.Name)
			if it.Alias != "" {
				bind = it.Alias
			}
			g.w(bind + " = _fly_sb_import(\"" + it.Name + "\")\n")
		}
	case *ast.FromImportStmt:
		g.indentLine()
		modVar := "_fly_sb_mod_" + strings.ReplaceAll(t.Module, ".", "_")
		fromlist := make([]string, len(t.Items))
		for i, it := range t.Items {
			fromlist[i] = it.Name
		}
		g.w(modVar + " = _fly_sb_import(\"" + t.Module + "\", fromlist=(\"" + strings.Join(fromlist, "\",\"") + "\"))\n")
		for _, it := range t.Items {
			g.indentLine()
			bind := it.Name
			if it.Alias != "" {
				bind = it.Alias
			}
			g.w(bind + " = " + modVar + "." + it.Name + "\n")
		}
	case *ast.AssignStmt:
		if t.Op != "=" {
			g.augAssign(t)
			break
		}
		if len(t.Left) == 1 {
			if l, ok := t.Left[0].(*ast.SubscriptExpr); ok {
				g.indentLine()
				g.w("_fly_set(")
				g.expr(l.X, precCond)
				g.w(", ")
				g.indexArg(l.Index)
				g.w(", ")
				g.expr(t.Right, precCond)
				g.w(fmt.Sprintf(", %d, %d)", t.Pos_.Line, t.Pos_.Col))
				g.w("\n")
				break
			}
			if l, ok := t.Left[0].(*ast.AttrExpr); ok {
				g.indentLine()
				g.w("_fly_setattr(")
				g.expr(l.X, precCond)
				g.w(fmt.Sprintf(", %q, ", l.Name))
				g.expr(t.Right, precCond)
				g.w(fmt.Sprintf(", %d, %d)", t.Pos_.Line, t.Pos_.Col))
				g.w("\n")
				break
			}
		}
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
	case *ast.SafeStmt, *ast.MaskStmt:
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
		g.funcDef(t)
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
		if t.Seal {
			g.sealSuite(t)
		} else {
			g.suite(t.Body)
		}
	case *ast.OnlyStmt:
		g.onlyStmt(t)
	case *ast.TraceStmt:
		g.traceStmt(t)
	case *ast.CageStmt:
		g.cageStmt(t)
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
		g.w(" in _fly_iter(")
		g.expr(t.Iter, precCond)
		g.w(fmt.Sprintf(", %d, %d):", t.Pos_.Line, t.Pos_.Col))
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

func (g *Gen) funcDef(t *ast.FuncDef) {
	prevFn := g.curFn
	g.curFn = t.Name
	defer func() { g.curFn = prevFn }()
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
	if g.sealInit && t.Name == "__init__" {
		g.w("\n")
		g.indent++
		g.indentLine()
		g.w("object.__setattr__(self, '_fly_seal_initializing', True)\n")
		for _, s := range t.Body {
			g.stmt(s)
		}
		g.indentLine()
		g.w("object.__setattr__(self, '_fly_seal_initializing', False)\n")
		g.indent--
		return
	}
	g.suite(t.Body)
}

func (g *Gen) sealSuite(t *ast.ClassDef) {
	if len(t.Body) == 0 {
		g.w(" pass\n")
		g.indentLine()
		g.setattrMethod(false)
		g.indentLine()
		g.setattrMethod(true)
		return
	}
	g.w("\n")
	g.indent++
	old := g.sealInit
	g.sealInit = true
	for _, s := range t.Body {
		g.stmt(s)
	}
	g.sealInit = old
	g.setattrMethod(false)
	g.setattrMethod(true)
	g.indent--
}

func (g *Gen) setattrMethod(del bool) {
	m := "setattr"
	raise := `raise AttributeError("seal 类 %s 的属性 %s 不可修改" % (type(self).__name__, name))`
	sig := "(self, name, value)"
	if del {
		m = "delattr"
		raise = `raise AttributeError("seal 类 %s 的属性 %s 不可删除" % (type(self).__name__, name))`
		sig = "(self, name)"
	}
	g.indentLine()
	g.w("def __" + m + "__" + sig + ":\n")
	g.indent++
	g.indentLine()
	g.w(`if _fly_sb_builtins.getattr(self, "_fly_seal_initializing", False):` + "\n")
	g.indent++
	g.indentLine()
	g.w("object.__" + m + "__" + "(self, name")
	if !del {
		g.w(", value")
	}
	g.w(")\n")
	g.indent--
	g.indentLine()
	g.w("else:\n")
	g.indent++
	g.indentLine()
	g.w(raise + "\n")
	g.indent--
	g.indent--
}

func (g *Gen) onlyStmt(t *ast.OnlyStmt) {
	for _, m := range t.Modules {
		g.indentLine()
		g.w("import " + m + "\n")
	}
	g.nc++
	saved := "_fly_ob_" + string(rune('a'+g.nc))
	g.indentLine()
	g.w(saved + " = _fly_sb_module_globals.get(\"__builtins__\", _fly_builtins)\n")
	g.indentLine()
	g.w("__builtins__ = _FlyOnly(" + modsLit(t.Modules) + ")\n")
	for _, s := range t.Body {
		g.stmt(s)
		if f, ok := s.(*ast.FuncDef); ok {
			g.indentLine()
			g.w(f.Name + " = _fly_patch_builtins(" + f.Name + ", " + modsLit(t.Modules) + ")\n")
		}
	}
	g.indentLine()
	g.w("__builtins__ = " + saved + "\n")
}

func (g *Gen) cageStmt(t *ast.CageStmt) {
	args := []string{}
	if t.HasTime {
		args = append(args, strconv.FormatFloat(t.MaxTime, 'f', -1, 64))
	}
	if t.HasMem {
		args = append(args, strconv.FormatInt(t.MaxMemory, 10))
	}
	for _, s := range t.Body {
		if _, ok := s.(*ast.FuncDef); ok {
			g.indentLine()
			g.w("@_fly_cage(" + strings.Join(args, ", ") + ")\n")
		}
		g.stmt(s)
	}
}

func (g *Gen) traceStmt(t *ast.TraceStmt) {
	level := t.Level
	if level == "WARN" {
		level = "WARNING"
	}
	for _, s := range t.Body {
		g.traceBody(s, level, t.Args, t.Ret)
	}
}

func (g *Gen) traceBody(s ast.Stmt, level string, args, ret bool) {
	f, ok := s.(*ast.FuncDef)
	if !ok {
		g.stmt(s)
		return
	}
	g.traceFunc(f, level, args, ret)
}

func (g *Gen) traceFunc(f *ast.FuncDef, level string, args, ret bool) {
	g.nc++
	rt := "_fly_ret_" + string(rune('a'+g.nc))
	er := "_fly_err_" + string(rune('a'+g.nc))
	for _, d := range f.Decorators {
		g.indentLine()
		g.w("@")
		g.expr(d, precLowest)
		g.w("\n")
	}
	g.indentLine()
	g.w("def " + f.Name + "(")
	g.params(f.Params)
	g.w("):\n")
	g.indent++
	g.indentLine()
	if args {
		names := make([]string, 0, len(f.Params))
		for _, p := range f.Params {
			if p.Name != "" && !p.Star && !p.DblStar {
				names = append(names, p.Name)
			}
		}
		if len(names) > 0 {
			msg := "enter " + f.Name
			for _, n := range names {
				msg += ", " + n + "=%r"
			}
			g.w(`_fly_log.log(_fly_log.` + level + `, "` + msg + `", ` + strings.Join(names, ", ") + ")\n")
		} else {
			g.w(`_fly_log.log(_fly_log.` + level + `, "enter ` + f.Name + `")` + "\n")
		}
	} else {
		g.w(`_fly_log.log(_fly_log.` + level + `, "enter ` + f.Name + `")` + "\n")
	}
	g.indentLine()
	g.w("try:\n")
	g.indent++
	g.indentLine()
	g.w(rt + " = _fly_trace_impl_" + f.Name + "(")
	g.callArgs(f.Params)
	g.w(")\n")
	g.indent--
	g.indentLine()
	g.w("except BaseException as " + er + ":\n")
	g.indent++
	g.indentLine()
	g.w(`_fly_log.log(_fly_log.` + level + `, "exit ` + f.Name + `: raise %r", ` + er + ")\n")
	g.indentLine()
	g.w("raise\n")
	g.indent--
	if ret {
		g.indentLine()
		g.w(`_fly_log.log(_fly_log.` + level + `, "exit ` + f.Name + `: ret=%r", ` + rt + ")\n")
	} else {
		g.indentLine()
		g.w(`_fly_log.log(_fly_log.` + level + `, "exit ` + f.Name + `")` + "\n")
	}
	g.indentLine()
	g.w("return " + rt + "\n")
	g.indent--
	g.indentLine()
	g.w("def _fly_trace_impl_" + f.Name + "(")
	g.params(f.Params)
	g.w("):")
	g.suite(f.Body)
}

func (g *Gen) callArgs(params []ast.Param) {
	first := true
	for _, p := range params {
		if p.Name == "" {
			continue
		}
		if !first {
			g.w(", ")
		}
		first = false
		if p.Star {
			g.w("*")
		}
		if p.DblStar {
			g.w("**")
		}
		g.w(p.Name)
	}
}

func modsLit(mods []string) string {
	q := make([]string, len(mods))
	for i, m := range mods {
		q[i] = "'" + m + "'"
	}
	return "(" + strings.Join(q, ", ") + ")"
}

func (g *Gen) slicePart(e ast.Expr) {
	if e == nil {
		g.w("None")
		return
	}
	g.expr(e, precCond)
}

func (g *Gen) indexArg(e ast.Expr) {
	if sl, ok := e.(*ast.SliceExpr); ok {
		g.w("slice(")
		g.slicePart(sl.Lo)
		g.w(", ")
		g.slicePart(sl.Hi)
		if sl.Step != nil {
			g.w(", ")
			g.slicePart(sl.Step)
		}
		g.w(")")
		return
	}
	g.expr(e, precCond)
}

func (g *Gen) augAssign(t *ast.AssignStmt) {
	op := strings.TrimSuffix(t.Op, "=")
	if len(t.Left) == 1 {
		if l, ok := t.Left[0].(*ast.SubscriptExpr); ok {
			g.indentLine()
			g.w("_fly_set(")
			g.expr(l.X, precCond)
			g.w(", ")
			g.indexArg(l.Index)
			g.w(", _fly_binop(_fly_get(")
			g.expr(l.X, precCond)
			g.w(", ")
			g.indexArg(l.Index)
			g.w(fmt.Sprintf(", %d, %d), ", t.Pos_.Line, t.Pos_.Col))
			g.expr(t.Right, precCond)
			g.w(fmt.Sprintf(", %q, %d, %d), %d, %d)", opName(op), t.Pos_.Line, t.Pos_.Col, t.Pos_.Line, t.Pos_.Col))
			g.w("\n")
			return
		}
		if l, ok := t.Left[0].(*ast.AttrExpr); ok {
			g.indentLine()
			g.w("_fly_setattr(")
			g.expr(l.X, precCond)
			g.w(fmt.Sprintf(", %q, _fly_binop(_fly_attr(", l.Name))
			g.expr(l.X, precCond)
			g.w(fmt.Sprintf(", %q, %d, %d), ", l.Name, t.Pos_.Line, t.Pos_.Col))
			g.expr(t.Right, precCond)
			g.w(fmt.Sprintf(", %q, %d, %d), %d, %d)", opName(op), t.Pos_.Line, t.Pos_.Col, t.Pos_.Line, t.Pos_.Col))
			g.w("\n")
			return
		}
	}
	g.indentLine()
	g.expr(t.Left[0], precLowest)
	g.w(" " + t.Op + " ")
	g.expr(t.Right, precLowest)
	g.w("\n")
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

func opName(op string) string {
	m := map[string]string{
		"+": "add", "-": "sub", "*": "mul", "/": "truediv", "//": "floordiv",
		"%": "mod", "**": "pow", "<<": "lshift", ">>": "rshift",
		"&": "and_", "|": "or_", "^": "xor", "@": "matmul",
	}
	return m[op]
}

func unaryName(op string) string {
	if op == "-" {
		return "neg"
	}
	if op == "+" {
		return "pos"
	}
	return "invert"
}

func cmpName(op string) string {
	m := map[string]string{
		"<": "lt", "<=": "le", ">": "gt", ">=": "ge",
		"in": "contains", "not in": "contains",
	}
	name := m[op]
	if op == "not in" {
		name = "contains"
	}
	return name
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
		if !g.plain {
			if n, ok := t.Func.(*ast.Name); ok && (n.Name == "int" || n.Name == "float") {
				g.w("_fly_cast(" + n.Name)
				for _, a := range t.Args {
					g.w(", ")
					g.expr(a, precCond)
				}
				g.w(fmt.Sprintf(", line=%d, col=%d)", t.Pos_.Line, t.Pos_.Col))
				return
			}
		}
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
		if g.plain {
			g.expr(t.X, precPost)
			g.w("." + t.Name)
			break
		}
		g.w("_fly_attr(")
		g.expr(t.X, precCond)
		g.w(fmt.Sprintf(", %q, %d, %d)", t.Name, t.Pos_.Line, t.Pos_.Col))
	case *ast.SubscriptExpr:
		if g.plain {
			g.expr(t.X, precPost)
			g.w("[")
			g.expr(t.Index, precLowest)
			g.w("]")
			break
		}
		g.w("_fly_get(")
		g.expr(t.X, precCond)
		g.w(", ")
		if sl, ok := t.Index.(*ast.SliceExpr); ok {
			g.w("slice(")
			g.slicePart(sl.Lo)
			g.w(", ")
			g.slicePart(sl.Hi)
			if sl.Step != nil {
				g.w(", ")
				g.slicePart(sl.Step)
			}
			g.w(")")
		} else {
			g.expr(t.Index, precCond)
		}
		g.w(fmt.Sprintf(", %d, %d)", t.Pos_.Line, t.Pos_.Col))
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
		if g.plain {
			if t.Op == "**" {
				g.expr(t.X, precPower+1)
				g.w("**")
				g.expr(t.Y, precPower)
				break
			}
			p := binPrec(t.Op)
			g.expr(t.X, p)
			g.w(" " + t.Op + " ")
			g.expr(t.Y, p+1)
			break
		}
		if g.types.plainBinOp(t.Op, t.X, t.Y, g.curFn) {
			// 类型推导为 int/str：原生运算（热路径优化，豁免 _fly_binop 帧）
			p := binPrec(t.Op)
			g.expr(t.X, p)
			g.w(" " + t.Op + " ")
			g.expr(t.Y, p+1)
			break
		}
		g.w("_fly_binop(")
		g.expr(t.X, precCond)
		g.w(", ")
		g.expr(t.Y, precCond)
		g.w(fmt.Sprintf(", %q, %d, %d)", opName(t.Op), t.Pos_.Line, t.Pos_.Col))
	case *ast.UnaryOpExpr:
		if g.plain || t.Op == "not" {
			if t.Op == "not" {
				g.w("not ")
				g.expr(t.X, precCompare)
				break
			}
			g.w(t.Op + " ")
			g.expr(t.X, precUnary)
			break
		}
		g.w("_fly_unary(")
		g.expr(t.X, precCond)
		g.w(fmt.Sprintf(", %q, %d, %d)", unaryName(t.Op), t.Pos_.Line, t.Pos_.Col))
	case *ast.BoolOpExpr:
		p := precOrBool
		if t.Op == "and" {
			p = precAndBool
		}
		g.expr(t.X, p)
		g.w(" " + t.Op + " ")
		g.expr(t.Y, p)
	case *ast.CompareExpr:
		if g.plain {
			g.expr(t.X, precCompare)
			for i, op := range t.Ops {
				g.w(" " + op + " ")
				g.expr(t.Ys[i], precCompare)
			}
			break
		}
		for i, op := range t.Ops {
			if i > 0 {
				g.w(" and ")
			}
			left := t.X
			if i > 0 {
				left = t.Ys[i-1]
			}
			if op == "==" || op == "!=" || op == "is" || op == "is not" {
				g.expr(left, precCompare)
				g.w(" " + op + " ")
				g.expr(t.Ys[i], precCompare)
			} else if g.types.plainBinOp(op, left, t.Ys[i], g.curFn) {
				// 类型推导为 int/str：原生比较（豁免 lambda 惰性帧）
				g.expr(left, precCompare)
				g.w(" " + op + " ")
				g.expr(t.Ys[i], precCompare)
			} else if op == "in" || op == "not in" {
				if op == "not in" {
					g.w("not ")
				}
				// operator.contains(a, b) 语义为 b in a，交换参数
				g.w("_fly_cmp(lambda: ")
				g.expr(t.Ys[i], precLowest)
				g.w(", lambda: ")
				g.expr(left, precLowest)
				g.w(fmt.Sprintf(", %q, %d, %d)", "contains", t.Pos_.Line, t.Pos_.Col))
			} else {
				g.w("_fly_cmp(lambda: ")
				g.expr(left, precLowest)
				g.w(", lambda: ")
				g.expr(t.Ys[i], precLowest)
				g.w(fmt.Sprintf(", %q, %d, %d)", cmpName(op), t.Pos_.Line, t.Pos_.Col))
			}
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
			if g.plain {
				g.w(" in ")
				g.expr(cl.Iter, precCond)
			} else {
				g.w(" in _fly_iter(")
				g.expr(cl.Iter, precCond)
				g.w(fmt.Sprintf(", %d, %d)", t.Pos_.Line, t.Pos_.Col))
			}
			for _, f := range cl.Ifs {
				g.w(" if ")
				g.expr(f, precCond)
			}
		}
		g.w("]")
	}
}
