package gen

import "pyfly/internal/ast"

// Type 是变量/表达式的推导类型。推导为已知类型时，gen 可豁免 _fly_*
// 兜底、直接生成原生运算（热路径优化）；未知/冲突则保持运行时护栏。
type Type int

const (
	TUnknown Type = iota
	TInt
	TFloat
	TStr
	TBool
	TNone
	TList
	TDict
	TSet
	TTuple
)

type TypeInfer struct {
	m          *ast.Module
	moduleVars map[string]Type
	fnVars     map[string]map[string]Type
	paramTypes map[string][]Type // fn → 形参类型（调用点聚合）
	argSet     map[string]bool   // paramTypes 中该函数是否已有明确类型
	retTypes   map[string]Type   // fn → 返回类型
	fnDefs     map[string]*ast.FuncDef
}

func NewTypeInfer(m *ast.Module) *TypeInfer {
	fnDefs := map[string]*ast.FuncDef{}
	var walk func([]ast.Stmt)
	walk = func(stmts []ast.Stmt) {
		for _, s := range stmts {
			if f, ok := s.(*ast.FuncDef); ok {
				fnDefs[f.Name] = f
			}
			if c, ok := s.(*ast.ClassDef); ok {
				walk(c.Body)
			}
		}
	}
	walk(m.Stmts)
	return &TypeInfer{
		m:          m,
		moduleVars: map[string]Type{},
		fnVars:     map[string]map[string]Type{},
		paramTypes: map[string][]Type{},
		argSet:     map[string]bool{},
		retTypes:   map[string]Type{},
		fnDefs:     fnDefs,
	}
}

func mergeType(a, b Type) Type {
	if a == TUnknown || b == TUnknown {
		return TUnknown
	}
	if a != b {
		return TUnknown
	}
	return a
}

// setType 合并推导结果：首次赋值直接写入，冲突/未知永久 TUnknown。
func setType(m map[string]Type, name string, ty Type) {
	if cur, ok := m[name]; ok {
		m[name] = mergeType(cur, ty)
	} else {
		m[name] = ty
	}
}

func callableRet(name string) Type {
	switch name {
	case "int", "len", "hash", "ord", "abs", "sum", "round", "pow":
		return TInt
	case "str", "repr", "chr", "format", "hex", "oct", "bin":
		return TStr
	case "float":
		return TFloat
	case "bool", "isinstance", "callable", "hasattr", "issubclass":
		return TBool
	case "input":
		return TStr
	case "range":
		return TInt
	case "list", "sorted", "reversed", "filter", "map", "zip":
		return TList
	case "dict", "enumerate":
		return TDict
	case "set", "frozenset":
		return TSet
	case "tuple":
		return TTuple
	}
	return TUnknown
}

func (ti *TypeInfer) exprType(e ast.Expr, scope map[string]Type, fn string) Type {
	switch t := e.(type) {
	case *ast.IntLit:
		return TInt
	case *ast.FloatLit:
		return TFloat
	case *ast.StringLit:
		return TStr
	case *ast.Name:
		if scope != nil {
			if ty, ok := scope[t.Name]; ok {
				return ty
			}
		}
		if fn != "" {
			if fv, ok := ti.fnVars[fn]; ok {
				if ty, ok := fv[t.Name]; ok {
					return ty
				}
			}
		}
		if ty, ok := ti.moduleVars[t.Name]; ok {
			return ty
		}
		return TUnknown
	case *ast.ListLit:
		return TList
	case *ast.DictLit:
		return TDict
	case *ast.SetLit:
		return TSet
	case *ast.BinOpExpr:
		xt := ti.exprType(t.X, scope, fn)
		yt := ti.exprType(t.Y, scope, fn)
		switch t.Op {
		case "+":
			if xt == TInt && yt == TInt || xt == TStr && yt == TStr {
				return xt
			}
		case "-", "*", "//", "%", "<<", ">>", "&", "|", "^":
			if xt == TInt && yt == TInt {
				return TInt
			}
		case "/":
			return TFloat
		case "**":
			if xt == TInt && yt == TInt {
				return TInt
			}
		}
		return TUnknown
	case *ast.UnaryOpExpr:
		if t.Op == "not" {
			return TBool
		}
		xt := ti.exprType(t.X, scope, fn)
		if xt == TInt {
			return TInt
		}
		if xt == TFloat {
			return TFloat
		}
		return TUnknown
	case *ast.BoolOpExpr:
		return mergeType(ti.exprType(t.X, scope, fn), ti.exprType(t.Y, scope, fn))
	case *ast.CondExpr:
		return mergeType(ti.exprType(t.Then, scope, fn), ti.exprType(t.Else, scope, fn))
	case *ast.CallExpr:
		if n, ok := t.Func.(*ast.Name); ok {
			if ret, ok := ti.fnDefs[n.Name]; ok {
				_ = ret
				if ty, ok := ti.retTypes[n.Name]; ok {
					return ty
				}
				return TUnknown
			}
			return callableRet(n.Name)
		}
		return TUnknown
	case *ast.SubscriptExpr:
		return TUnknown
	case *ast.AttrExpr:
		return TUnknown
	case *ast.SliceExpr:
		return TUnknown
	case *ast.ListComp:
		return TList
	}
	return TUnknown
}

// binOpType 返回 + - * // % 的推导类型（用于增强赋值）。
func (ti *TypeInfer) binOpType(op string, a, b Type) Type {
	switch op {
	case "+":
		if a == TInt && b == TInt || a == TStr && b == TStr {
			return a
		}
	case "-", "*", "//", "%":
		if a == TInt && b == TInt {
			return TInt
		}
	}
	return TUnknown
}

// iterElemType 返回 for 循环迭代变量的推导类型。
func (ti *TypeInfer) iterElemType(iter ast.Expr, scope map[string]Type, fn string) Type {
	if c, ok := iter.(*ast.CallExpr); ok {
		if n, ok := c.Func.(*ast.Name); ok && n.Name == "range" {
			return TInt
		}
	}
	return ti.exprType(iter, scope, fn)
}

func (ti *TypeInfer) collectStmts(stmts []ast.Stmt, scope map[string]Type, fn string) {
	for _, s := range stmts {
		switch t := s.(type) {
		case *ast.AssignStmt:
			rt := ti.exprType(t.Right, scope, fn)
			if len(t.Left) == 1 {
				if n, ok := t.Left[0].(*ast.Name); ok {
					if t.Op == "=" {
						setType(scope, n.Name, rt)
					} else {
						lt := scope[n.Name]
						setType(scope, n.Name, ti.binOpType(trimAug(t.Op), lt, rt))
					}
				}
			}
		case *ast.ForStmt:
			elem := ti.iterElemType(t.Iter, scope, fn)
			if name, ok := t.Target.(*ast.Name); ok {
				setType(scope, name.Name, elem)
			}
			ti.collectStmts(t.Body, scope, fn)
			if t.Else != nil {
				ti.collectStmts(t.Else, scope, fn)
			}
		case *ast.WhileStmt:
			ti.collectStmts(t.Body, scope, fn)
			if t.Else != nil {
				ti.collectStmts(t.Else, scope, fn)
			}
		case *ast.IfStmt:
			for _, b := range t.Elifs {
				ti.collectStmts(b.Body, scope, fn)
			}
			ti.collectStmts(t.Then, scope, fn)
			if t.Else != nil {
				ti.collectStmts(t.Else, scope, fn)
			}
		case *ast.TryStmt:
			ti.collectStmts(t.Body, scope, fn)
			for _, h := range t.Handlers {
				ti.collectStmts(h.Body, scope, fn)
			}
			if t.Else != nil {
				ti.collectStmts(t.Else, scope, fn)
			}
			if t.Finally != nil {
				ti.collectStmts(t.Finally, scope, fn)
			}
		case *ast.ReturnStmt:
			if t.Value != nil {
				rt := ti.exprType(t.Value, scope, fn)
				if cur, ok := ti.retTypes[fn]; ok {
					ti.retTypes[fn] = mergeType(cur, rt)
				} else {
					ti.retTypes[fn] = rt
				}
			}
		case *ast.ExprStmt, *ast.RaiseStmt, *ast.OnlyStmt, *ast.TraceStmt,
			*ast.CageStmt, *ast.GuardStmt, *ast.LockStmt, *ast.MaskStmt,
			*ast.ImportStmt, *ast.FromImportStmt:
			// 不推导
		}
	}
}

func (ti *TypeInfer) collectCalls(e ast.Expr, scope map[string]Type, fn string) {
	switch t := e.(type) {
	case *ast.Name, *ast.IntLit, *ast.FloatLit, *ast.StringLit:
		return
	case *ast.CallExpr:
		if n, ok := t.Func.(*ast.Name); ok {
			if _, isFn := ti.fnDefs[n.Name]; isFn {
				cur := ti.paramTypes[n.Name]
				for len(cur) < len(t.Args) {
					cur = append(cur, TUnknown)
				}
				for i, a := range t.Args {
					at := ti.exprType(a, scope, fn)
					if cur[i] == TUnknown && !ti.argSet[n.Name] {
						cur[i] = at
						ti.argSet[n.Name] = true
					} else {
						cur[i] = mergeType(cur[i], at)
					}
				}
				ti.paramTypes[n.Name] = cur
			}
		}
		for _, a := range t.Args {
			ti.collectCalls(a, scope, fn)
		}
	case *ast.BinOpExpr:
		ti.collectCalls(t.X, scope, fn)
		ti.collectCalls(t.Y, scope, fn)
	case *ast.UnaryOpExpr:
		ti.collectCalls(t.X, scope, fn)
	case *ast.BoolOpExpr:
		ti.collectCalls(t.X, scope, fn)
		ti.collectCalls(t.Y, scope, fn)
	case *ast.CondExpr:
		ti.collectCalls(t.Cond, scope, fn)
		ti.collectCalls(t.Then, scope, fn)
		ti.collectCalls(t.Else, scope, fn)
	case *ast.AttrExpr:
		ti.collectCalls(t.X, scope, fn)
	case *ast.SubscriptExpr:
		ti.collectCalls(t.X, scope, fn)
		ti.collectCalls(t.Index, scope, fn)
	case *ast.SliceExpr:
		ti.collectCalls(t.Lo, scope, fn)
		ti.collectCalls(t.Hi, scope, fn)
		ti.collectCalls(t.Step, scope, fn)
	case *ast.ListLit:
		for _, el := range t.Elems {
			ti.collectCalls(el, scope, fn)
		}
	case *ast.DictLit:
		for i, k := range t.Keys {
			ti.collectCalls(k, scope, fn)
			if i < len(t.Vals) {
				ti.collectCalls(t.Vals[i], scope, fn)
			}
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			ti.collectCalls(el, scope, fn)
		}
	case *ast.ListComp:
		ti.collectCalls(t.Elem, scope, fn)
		for _, cl := range t.Clauses {
			ti.collectCalls(cl.Iter, scope, fn)
			for _, f := range cl.Ifs {
				ti.collectCalls(f, scope, fn)
			}
		}
	}
}

func (ti *TypeInfer) runRound() {
	mod := map[string]Type{}
	for k := range ti.moduleVars {
		if ti.moduleVars[k] == TUnknown {
			mod[k] = TUnknown
		}
	}
	ti.moduleVars = mod
	ti.collectStmts(ti.m.Stmts, ti.moduleVars, "")
	for _, s := range ti.m.Stmts {
		ti.collectCallsExprStmt(s, ti.moduleVars, "")
	}
	for name, f := range ti.fnDefs {
		scope := map[string]Type{}
		params := ti.paramTypes[name]
		for i, p := range f.Params {
			if i < len(params) {
				scope[p.Name] = params[i]
			}
		}
		prev := ti.fnVars[name]
		if prev == nil {
			prev = map[string]Type{}
		}
		kept := map[string]Type{}
		for k, v := range prev {
			if v == TUnknown {
				kept[k] = TUnknown
			}
		}
		ti.fnVars[name] = kept
		ti.collectStmts(f.Body, scope, name)
		for k, v := range scope {
			if prevV, ok := prev[k]; ok {
				scope[k] = mergeType(prevV, v)
			}
		}
		ti.fnVars[name] = scope
		for _, st := range f.Body {
			ti.collectCallsExprStmt(st, scope, name)
		}
	}
}

// collectCallsExprStmt 收集语句中嵌套的调用点。
func (ti *TypeInfer) collectCallsExprStmt(s ast.Stmt, scope map[string]Type, fn string) {
	switch t := s.(type) {
	case *ast.ExprStmt:
		ti.collectCalls(t.X, scope, fn)
	case *ast.AssignStmt:
		ti.collectCalls(t.Right, scope, fn)
		for _, l := range t.Left {
			ti.collectCalls(l, scope, fn)
		}
	case *ast.ReturnStmt:
		if t.Value != nil {
			ti.collectCalls(t.Value, scope, fn)
		}
	case *ast.IfStmt:
		for _, b := range t.Elifs {
			ti.collectCalls(b.Cond, scope, fn)
			for _, st := range b.Body {
				ti.collectCallsExprStmt(st, scope, fn)
			}
		}
		ti.collectCalls(t.Cond, scope, fn)
		for _, st := range t.Then {
			ti.collectCallsExprStmt(st, scope, fn)
		}
		if t.Else != nil {
			for _, st := range t.Else {
				ti.collectCallsExprStmt(st, scope, fn)
			}
		}
	case *ast.ForStmt:
		ti.collectCalls(t.Iter, scope, fn)
		for _, st := range t.Body {
			ti.collectCallsExprStmt(st, scope, fn)
		}
	case *ast.WhileStmt:
		ti.collectCalls(t.Cond, scope, fn)
		for _, st := range t.Body {
			ti.collectCallsExprStmt(st, scope, fn)
		}
	case *ast.TryStmt:
		for _, st := range t.Body {
			ti.collectCallsExprStmt(st, scope, fn)
		}
		for _, h := range t.Handlers {
			for _, st := range h.Body {
				ti.collectCallsExprStmt(st, scope, fn)
			}
		}
	}
}

func (ti *TypeInfer) Run() {
	for round := 0; round < 5; round++ {
		ti.runRound()
	}
}

// trimAug 去掉增强赋值的尾缀（"+=" → "+"）。
func trimAug(op string) string {
	if len(op) > 1 && op[len(op)-1] == '=' {
		return op[:len(op)-1]
	}
	return op
}

// varType 返回 fn 作用域（或模块级）中变量的推导类型。
func (ti *TypeInfer) varType(name, fn string) Type {
	if fn != "" {
		if scope, ok := ti.fnVars[fn]; ok {
			if ty, ok := scope[name]; ok {
				return ty
			}
		}
	}
	return ti.moduleVars[name]
}

// plainBinOp 判定操作数推导为 int/str 时可原生生成（豁免 _fly_binop）。
func (ti *TypeInfer) plainBinOp(op string, x, y ast.Expr, fn string) bool {
	xt := ti.exprType(x, nil, fn)
	yt := ti.exprType(y, nil, fn)
	if xt == TUnknown || yt == TUnknown {
		return false
	}
	switch op {
	case "+":
		return xt == yt && (xt == TInt || xt == TStr)
	case "-", "*", "//", "%":
		return xt == TInt && yt == TInt
	case "<", "<=", ">", ">=":
		return xt == yt && (xt == TInt || xt == TStr)
	}
	return false
}

// flySBReflect 与 internal/runtime/fly_runtime.py 的 _FLY_SB_REFLECT 同步
// （Rust 侧 src/typeinfer.rs FLY_SB_REFLECT，三处必须一致）。
var flySBReflect = map[string]bool{
	"__class__": true, "__bases__": true, "__base__": true, "__mro__": true,
	"__subclasses__": true, "__globals__": true, "__code__": true,
	"__closure__": true, "__dict__": true, "__reduce__": true,
	"__reduce_ex__": true, "__getattribute__": true, "__setattr__": true,
	"__delattr__": true, "__init_subclass__": true, "__prepare__": true,
	"__builtins__": true, "__traceback__": true, "gi_frame": true,
	"ag_frame": true, "cr_frame": true, "f_globals": true, "f_locals": true,
	"__loader__": true,
}

// plainAttr 判定属性访问可原生生成（豁免 _fly_attr）：x 推导类型非
// Unknown（typeinfer 只会从字面量/内建调用/容器推导出非模块类型，
// import 绑定恒 Unknown）+ name 不在反射名单。
func (ti *TypeInfer) plainAttr(x ast.Expr, name, fn string) bool {
	return ti.exprType(x, nil, fn) != TUnknown && !flySBReflect[name]
}

// plainSubscr 判定下标访问可原生生成（豁免 _fly_get/_fly_set）：
// index 推导类型非 Unknown 且非 Str——反射名单全为字符串。
func (ti *TypeInfer) plainSubscr(index ast.Expr, fn string) bool {
	kt := ti.exprType(index, nil, fn)
	return kt != TUnknown && kt != TStr
}

// plainIter 判定 for 迭代可原生生成（豁免 _fly_iter）：_fly_iter 只做
// 迭代包装与错误行列号包装，无安全拦截——容器类型与 range 调用恒可迭代，
// 豁免仅可能改变错误信息，不影响沙箱安全。
func (ti *TypeInfer) plainIter(iter ast.Expr, fn string) bool {
	switch ti.exprType(iter, nil, fn) {
	case TList, TDict, TSet, TTuple, TStr:
		return true
	}
	if c, ok := iter.(*ast.CallExpr); ok {
		if n, ok := c.Func.(*ast.Name); ok && n.Name == "range" {
			return true
		}
	}
	return false
}
