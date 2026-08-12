package checker

import (
	"strings"

	"flylang/internal/ast"
	"flylang/internal/parser"
)

// 沙箱逃逸拦截：与 fly_runtime.py sandbox 节的运行时名单保持一致（双保险）。
// 编译期拦截可静态确定的逃逸模式，运行时 _fly_* 兜底动态场景。

// escapeBuiltins 危险内建：直接调用即沙箱逃逸风险。
var escapeBuiltins = map[string]bool{
	"eval": true, "exec": true, "compile": true, "open": true,
	"__import__": true, "getattr": true, "globals": true, "locals": true,
	"vars": true, "input": true, "breakpoint": true, "help": true,
	"dir": true, "memoryview": true, "__loader__": true,
}

// escapeReflect 反射属性链：属性访问/下标访问命中即逃逸风险。
// __traceback__/__builtins__ 为模块/异常对象上的 builtins 字典与帧逃逸入口，
// gi_frame 等生成器帧属性（Python 语法不支持 yield 时不可达，仍双名单防护）。
var escapeReflect = map[string]bool{
	"__class__": true, "__bases__": true, "__base__": true, "__mro__": true,
	"__subclasses__": true, "__globals__": true, "__code__": true,
	"__closure__": true, "__dict__": true, "__reduce__": true,
	"__reduce_ex__": true, "__getattribute__": true, "__setattr__": true,
	"__delattr__": true, "__init_subclass__": true, "__prepare__": true,
	"__builtins__": true, "__traceback__": true, "gi_frame": true,
	"ag_frame": true, "cr_frame": true, "f_globals": true, "f_locals": true,
	"__loader__": true,
}

// escapeModAttrs 模块级危险属性：白名单模块对象上取用即逃逸风险
// （attrgetter/itemgetter 可用点路径直接穿透任意对象反射链）。
var escapeModAttrs = map[string]bool{
	"attrgetter": true, "itemgetter": true,
}

// escapeModules 危险模块：import 即沙箱逃逸风险（与运行时 _FLY_SB_BLOCKED_MODS 一致）。
var escapeModules = map[string]bool{
	"os": true, "sys": true, "subprocess": true, "socket": true,
	"ctypes": true, "shutil": true, "tempfile": true, "pty": true,
	"importlib": true, "imp": true, "marshal": true, "copyreg": true,
	"shelve": true, "multiprocessing": true, "pickle": true,
	"pickletools": true, "pathlib": true, "glob": true, "io": true,
	"codecs": true, "builtins": true, "csv": true, "sqlite3": true,
	"urllib": true, "ftplib": true, "telnetlib": true, "smtplib": true,
	"poplib": true, "imaplib": true, "http": true, "ssl": true,
	"zipfile": true, "tarfile": true, "gzip": true, "bz2": true,
	"lzma": true, "readline": true, "site": true, "pydoc": true,
	"gc": true, "dis": true, "inspect": true, "platform": true,
	"sysconfig": true, "pwd": true, "grp": true, "spwd": true,
	"getpass": true, "mmap": true, "fcntl": true, "select": true,
	"termios": true, "tty": true, "types": true, "trace": true,
	"tracemalloc": true, "faulthandler": true, "codeop": true,
	"code": true, "pkgutil": true, "py_compile": true, "compileall": true,
	"dbm": true, "email": true, "webbrowser": true, "cgi": true,
	"cgitb": true, "configparser": true,
}

// escapeCheck 一趟遍历：拦截危险内建调用、反射属性链、__builtins__ 访问、危险模块导入。
func (c *Checker) escapeCheck(m *ast.Module) {
	e := &escapeCheck{c: c, modBinds: map[string]bool{}}
	for _, s := range m.Stmts {
		e.stmt(s, 0)
	}
}

type escapeCheck struct {
	c           *Checker
	modBinds    map[string]bool
	posOverride *ast.Position
}

func (e *escapeCheck) errorf(pos ast.Position, format string, args ...interface{}) {
	if e.posOverride != nil {
		pos = *e.posOverride
	}
	e.c.errorf(pos, format, args...)
}

func (e *escapeCheck) stmt(s ast.Stmt, inOnly int) {
	switch t := s.(type) {
	case *ast.ImportStmt:
		for _, it := range t.Items {
			if inOnly == 0 {
				e.checkModule(it.Name, t.Pos_)
				bind := moduleRoot(it.Name)
				if it.Alias != "" {
					bind = it.Alias
				}
				e.modBinds[bind] = true
			}
		}
	case *ast.FromImportStmt:
		if inOnly == 0 {
			e.checkModule(t.Module, t.Pos_)
		}
	case *ast.AssignStmt:
		for _, l := range t.Left {
			e.assignTarget(l, inOnly)
		}
		e.expr(t.Right, inOnly)
	case *ast.LockStmt:
		e.expr(t.Value, inOnly)
	case *ast.GuardStmt:
		e.expr(t.Type, inOnly)
		for _, c := range t.Conds {
			e.expr(c, inOnly)
		}
	case *ast.ExprStmt:
		e.expr(t.X, inOnly)
	case *ast.FuncDef:
		for _, d := range t.Decorators {
			e.expr(d, inOnly)
		}
		for _, p := range t.Params {
			if p.Default != nil {
				e.expr(p.Default, inOnly)
			}
		}
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
	case *ast.ClassDef:
		for _, d := range t.Decorators {
			e.expr(d, inOnly)
		}
		for _, b := range t.Bases {
			e.expr(b, inOnly)
		}
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
	case *ast.OnlyStmt:
		for _, st := range t.Body {
			e.stmt(st, inOnly+1)
		}
	case *ast.TraceStmt:
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
	case *ast.CageStmt:
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
	case *ast.IfStmt:
		e.expr(t.Cond, inOnly)
		for _, st := range t.Then {
			e.stmt(st, inOnly)
		}
		for _, el := range t.Elifs {
			e.expr(el.Cond, inOnly)
			for _, st := range el.Body {
				e.stmt(st, inOnly)
			}
		}
		for _, st := range t.Else {
			e.stmt(st, inOnly)
		}
	case *ast.ForStmt:
		e.assignTarget(t.Target, inOnly)
		e.expr(t.Iter, inOnly)
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
		for _, st := range t.Else {
			e.stmt(st, inOnly)
		}
	case *ast.WhileStmt:
		e.expr(t.Cond, inOnly)
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
		for _, st := range t.Else {
			e.stmt(st, inOnly)
		}
	case *ast.TryStmt:
		for _, st := range t.Body {
			e.stmt(st, inOnly)
		}
		for _, h := range t.Handlers {
			e.expr(h.Type, inOnly)
			for _, st := range h.Body {
				e.stmt(st, inOnly)
			}
		}
		for _, st := range t.Else {
			e.stmt(st, inOnly)
		}
		for _, st := range t.Finally {
			e.stmt(st, inOnly)
		}
	case *ast.ReturnStmt:
		e.expr(t.Value, inOnly)
	case *ast.RaiseStmt:
		e.expr(t.Exc, inOnly)
		e.expr(t.From, inOnly)
	case *ast.DeleteStmt:
		for _, tg := range t.Targets {
			e.assignTarget(tg, inOnly)
		}
	}
}

// assignTarget 赋值/循环/删除目标：Name 是纯绑定不检查，其余表达式继续遍历。
func (e *escapeCheck) assignTarget(x ast.Expr, inOnly int) {
	switch t := x.(type) {
	case *ast.Name:
	case *ast.TupleLit:
		for _, el := range t.Elems {
			e.assignTarget(el, inOnly)
		}
	case *ast.ListLit:
		for _, el := range t.Elems {
			e.assignTarget(el, inOnly)
		}
	default:
		e.expr(x, inOnly)
	}
}

func (e *escapeCheck) checkModule(name string, pos ast.Position) {
	if root := moduleRoot(name); escapeModules[root] {
		e.errorf(pos, "禁止导入危险模块 %s（沙箱逃逸风险）", root)
	}
}

func moduleRoot(name string) string {
	for i := 0; i < len(name); i++ {
		if name[i] == '.' {
			return name[:i]
		}
	}
	return name
}

// checkFString f-string 在词法层是整体 STRING token，内部表达式进不了 AST：
// 对花括号内表达式文本二次解析后复用 expr 遍历；解析失败（转义/复杂语法）时
// 文本级名单匹配兜底。
func (e *escapeCheck) checkFString(t *ast.StringLit, inOnly int) {
	raw := t.Value
	if len(raw) < 2 || (raw[0] != 'f' && raw[0] != 'F') {
		return
	}
	body := strValue(raw[1:])
	for _, expr := range fstringExprs(body) {
		p := parser.New("___fly_fs = " + expr + "\n")
		m := p.ParseModule()
		if p.Error() != nil {
			e.checkFStringText(expr, t.Pos_)
			continue
		}
		if as, ok := m.Stmts[0].(*ast.AssignStmt); ok {
			old := e.posOverride
			e.posOverride = &t.Pos_
			e.expr(as.Right, inOnly)
			e.posOverride = old
		}
	}
}

// checkFStringText 文本级兜底：名单名作为完整标识符出现即拦。
func (e *escapeCheck) checkFStringText(expr string, pos ast.Position) {
	for name := range escapeBuiltins {
		if hasWord(expr, name) {
			e.errorf(pos, "f-string 内禁止访问内建 %s（沙箱逃逸风险）", name)
			return
		}
	}
	for name := range escapeReflect {
		if hasWord(expr, name) {
			e.errorf(pos, "f-string 内禁止反射访问 %s（沙箱逃逸风险）", name)
			return
		}
	}
}

func hasWord(s, w string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], w)
		if j < 0 {
			return false
		}
		j += i
		i = j + len(w)
		okL := j == 0 || !isIdentChar(s[j-1])
		okR := i >= len(s) || !isIdentChar(s[i])
		if okL && okR {
			return true
		}
	}
}

func isIdentChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// fstringExprs 提取 f-string 主体中所有 {expr} 的表达式文本（跳过 {{/}} 转义，
// 平衡嵌套花括号）。
func fstringExprs(body string) []string {
	var out []string
	depth := 0
	start := -1
	for i := 0; i < len(body); i++ {
		c := body[i]
		if depth == 0 && c != '{' {
			continue
		}
		if c == '{' {
			if i+1 < len(body) && body[i+1] == '{' {
				i++
				continue
			}
			if depth == 0 {
				start = i + 1
			}
			depth++
		} else if c == '}' {
			if i+1 < len(body) && body[i+1] == '}' {
				i++
				continue
			}
			depth--
			if depth == 0 && start >= 0 {
				out = append(out, body[start:i])
			}
		}
	}
	return out
}

// strValue 从字符串字面量文本（lexer 保留原文，含前缀与引号）提取纯字符串值。
// 转义变体（如 "\u005f_class__"）编译期无法解码，由运行时 _fly_get 兜底拦截。
func strValue(lit string) string {
	s := lit
	for len(s) > 0 && strings.ContainsRune("fFbBrRuU", rune(s[0])) {
		s = s[1:]
	}
	var q string
	if len(s) >= 3 && s[:3] == `"""` {
		q = `"""`
	} else if len(s) >= 3 && s[:3] == `'''` {
		q = `'''`
	} else if len(s) >= 2 && (s[0] == '"' || s[0] == '\'') {
		q = s[:1]
	}
	if q != "" && strings.HasSuffix(s, q) {
		return s[len(q) : len(s)-len(q)]
	}
	return s
}

func (e *escapeCheck) expr(x ast.Expr, inOnly int) {
	if x == nil {
		return
	}
	switch t := x.(type) {
	case *ast.Name:
		// 危险内建名出现在任何读取位置（调用/别名/参数）都是逃逸风险：
		// 模块顶层帧缓存 builtins，运行时代理无法拦截顶层别名绑定。
		if t.Name == "__builtins__" {
			e.errorf(t.Pos_, "禁止访问 __builtins__（沙箱逃逸风险）")
		} else if inOnly == 0 && escapeBuiltins[t.Name] {
			e.errorf(t.Pos_, "禁止访问内建 %s（沙箱逃逸风险）", t.Name)
		}
	case *ast.IntLit, *ast.FloatLit, *ast.EllipsisLit:
	case *ast.StringLit:
		e.checkFString(t, inOnly)
	case *ast.ListLit:
		for _, el := range t.Elems {
			e.expr(el, inOnly)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			e.expr(el, inOnly)
		}
	case *ast.DictLit:
		for i := range t.Keys {
			e.expr(t.Keys[i], inOnly)
			e.expr(t.Vals[i], inOnly)
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			e.expr(el, inOnly)
		}
	case *ast.CallExpr:
		if n, ok := t.Func.(*ast.Name); ok && inOnly == 0 && escapeBuiltins[n.Name] {
			e.errorf(n.Pos_, "禁止调用内建 %s（沙箱逃逸风险）", n.Name)
		} else {
			e.expr(t.Func, inOnly)
		}
		for _, a := range t.Args {
			e.expr(a, inOnly)
		}
		e.expr(t.Star, inOnly)
		e.expr(t.DblStar, inOnly)
		for _, kw := range t.Kwargs {
			e.expr(kw.Value, inOnly)
		}
	case *ast.AttrExpr:
		if escapeReflect[t.Name] {
			e.errorf(t.Pos_, "禁止反射访问属性 %s（沙箱逃逸风险）", t.Name)
		} else if n, ok := t.X.(*ast.Name); ok && e.modBinds[n.Name] {
			// 白名单模块对象上的危险子模块/属性（random.os、operator.attrgetter 等）
			if escapeModules[t.Name] || escapeModAttrs[t.Name] {
				e.errorf(t.Pos_, "禁止访问模块属性 %s（沙箱逃逸风险）", t.Name)
			}
		}
		e.expr(t.X, inOnly)
	case *ast.SubscriptExpr:
		if k, ok := t.Index.(*ast.StringLit); ok {
			if v := strValue(k.Value); escapeReflect[v] {
				e.errorf(k.Pos_, "禁止反射下标访问 %s（沙箱逃逸风险）", v)
			}
		}
		e.expr(t.X, inOnly)
		e.expr(t.Index, inOnly)
	case *ast.SliceExpr:
		e.expr(t.Lo, inOnly)
		e.expr(t.Hi, inOnly)
		e.expr(t.Step, inOnly)
	case *ast.BinOpExpr:
		e.expr(t.X, inOnly)
		e.expr(t.Y, inOnly)
	case *ast.UnaryOpExpr:
		e.expr(t.X, inOnly)
	case *ast.BoolOpExpr:
		e.expr(t.X, inOnly)
		e.expr(t.Y, inOnly)
	case *ast.CompareExpr:
		e.expr(t.X, inOnly)
		for _, y := range t.Ys {
			e.expr(y, inOnly)
		}
	case *ast.CondExpr:
		e.expr(t.Cond, inOnly)
		e.expr(t.Then, inOnly)
		e.expr(t.Else, inOnly)
	case *ast.ListComp:
		e.expr(t.Elem, inOnly)
		for _, cl := range t.Clauses {
			e.expr(cl.Target, inOnly)
			e.expr(cl.Iter, inOnly)
			for _, f := range cl.Ifs {
				e.expr(f, inOnly)
			}
		}
	}
}
