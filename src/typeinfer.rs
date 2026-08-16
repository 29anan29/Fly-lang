// typeinfer.rs：编译产物热路径优化——静态类型推导（变量/表达式类型猜测），
// 推导为 int/str 时 gen 豁免 _fly_* 运行时兜底、直接生成原生运算。
// 直译 Go 版 internal/gen/typeinfer.go（5 轮不动点迭代 + 调用点参数聚合）。
use crate::ast::{Expr, Module, Stmt};
use std::collections::HashMap;

// Type 是变量/表达式的推导类型。
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Type {
    Unknown,
    Int,
    Float,
    Str,
    Bool,
    None,
    List,
    Dict,
    Set,
    Tuple,
}

pub struct TypeInfer {
    m: Module,
    module_vars: HashMap<String, Type>,
    fn_vars: HashMap<String, HashMap<String, Type>>,
    param_types: HashMap<String, Vec<Type>>,
    arg_set: HashMap<String, bool>,
    ret_types: HashMap<String, Type>,
    fn_defs: HashMap<String, Stmt>,
}

impl TypeInfer {
    pub fn new(m: Module) -> Self {
        let mut fn_defs = HashMap::new();
        fn walk(stmts: &[Stmt], out: &mut HashMap<String, Stmt>) {
            for s in stmts {
                match s {
                    Stmt::FuncDef { name, .. } => {
                        out.insert(name.clone(), s.clone());
                    }
                    Stmt::ClassDef { body, .. } => walk(body, out),
                    _ => {}
                }
            }
        }
        walk(&m.stmts, &mut fn_defs);
        TypeInfer {
            m,
            module_vars: HashMap::new(),
            fn_vars: HashMap::new(),
            param_types: HashMap::new(),
            arg_set: HashMap::new(),
            ret_types: HashMap::new(),
            fn_defs,
        }
    }

    // Run 执行 5 轮迭代推导（与 Go 版一致）。
    pub fn run(&mut self) {
        for _ in 0..5 {
            self.run_round();
        }
    }

    // Stmts 返回模块顶层语句（供 gen 遍历）。
    pub fn stmts(&self) -> &[Stmt] {
        &self.m.stmts
    }

    // PlainBinOp 判定操作数推导为 int/str 时可原生生成（豁免 _fly_binop）。
    pub fn plain_bin_op(&self, op: &str, x: &Expr, y: &Expr, fn_name: &str) -> bool {
        let xt = expr_type(self, x, None, fn_name);
        let yt = expr_type(self, y, None, fn_name);
        if xt == Type::Unknown || yt == Type::Unknown {
            return false;
        }
        match op {
            "+" => xt == yt && (xt == Type::Int || xt == Type::Str),
            "-" | "*" | "//" | "%" => xt == Type::Int && yt == Type::Int,
            "<" | "<=" | ">" | ">=" => xt == yt && (xt == Type::Int || xt == Type::Str),
            _ => false,
        }
    }

    // PlainAttr 判定属性访问可原生生成（豁免 _fly_attr）：
    // x 推导类型非 Unknown（typeinfer 只会从字面量/内建调用/容器推导出
    // Int/Str/Bool/Float/None/List/Dict/Set/Tuple，import 绑定恒 Unknown，
    // 故非 Unknown 恒为非模块对象）+ name 字面量不在反射名单
    // （名单必须与 fly_runtime.py 的 _FLY_SB_REFLECT 一致，见下方单测）。
    pub fn plain_attr(&self, x: &Expr, name: &str, fn_name: &str) -> bool {
        let xt = expr_type(self, x, None, fn_name);
        xt != Type::Unknown && !FLY_SB_REFLECT.iter().any(|r| *r == name)
    }

// PlainSubscr 判定下标访问可原生生成（豁免 _fly_get/_fly_set）：
    // index 推导类型非 Unknown 且非 Str——反射名单全为字符串，
    // 非 str 的 key 恒不在名单（_fly_get 运行时拦截仅针对 str key）。
    pub fn plain_subscr(&self, _x: &Expr, index: &Expr, fn_name: &str) -> bool {
        let kt = expr_type(self, index, None, fn_name);
        kt != Type::Unknown && kt != Type::Str
    }

    // PlainIter 判定 for 迭代可原生生成（豁免 _fly_iter）：
    // _fly_iter 只做迭代包装与错误行列号包装，无安全拦截——
    // 容器类型（List/Dict/Set/Tuple/Str）与 range 调用恒可迭代，
    // 豁免仅可能改变错误信息（TypeError 不带行列号），不影响沙箱安全。
    pub fn plain_iter(&self, iter: &Expr, fn_name: &str) -> bool {
        let it = expr_type(self, iter, None, fn_name);
        if matches!(
            it,
            Type::List | Type::Dict | Type::Set | Type::Tuple | Type::Str
        ) {
            return true;
        }
        matches!(iter, Expr::Call { func, .. } if matches!(
            func.as_ref(),
            Expr::Name { name, .. } if name == "range"
        ))
    }
}

// FLY_SB_REFLECT 与 internal/runtime/fly_runtime.py 的 _FLY_SB_REFLECT 同步。
// gen 豁免属性访问护栏时静态检查名单；名单不同步会导致豁免绕过运行时拦截，
// 修改任一处必须同步（单测 reflect_list_sync 断言一致）。
pub const FLY_SB_REFLECT: [&str; 24] = [
    "__class__", "__bases__", "__base__", "__mro__", "__subclasses__",
    "__globals__", "__code__", "__closure__", "__dict__", "__reduce__",
    "__reduce_ex__", "__getattribute__", "__setattr__", "__delattr__",
    "__init_subclass__", "__prepare__", "__builtins__", "__traceback__",
    "gi_frame", "ag_frame", "cr_frame", "f_globals", "f_locals", "__loader__",
];

fn merge_type(a: Type, b: Type) -> Type {
    if a == Type::Unknown || b == Type::Unknown || a != b {
        return Type::Unknown;
    }
    a
}

// SetType 合并推导结果：首次赋值直接写入，冲突/未知永久 Unknown。
fn set_type(m: &mut HashMap<String, Type>, name: &str, ty: Type) {
    match m.get_mut(name) {
        Some(cur) => *cur = merge_type(*cur, ty),
        None => {
            m.insert(name.to_string(), ty);
        }
    }
}

fn callable_ret(name: &str) -> Type {
    match name {
        "int" | "len" | "hash" | "ord" | "abs" | "sum" | "round" | "pow" => Type::Int,
        "str" | "repr" | "chr" | "format" | "hex" | "oct" | "bin" => Type::Str,
        "float" => Type::Float,
        "bool" | "isinstance" | "callable" | "hasattr" | "issubclass" => Type::Bool,
        "input" => Type::Str,
        "range" => Type::Int,
        "list" | "sorted" | "reversed" | "filter" | "map" | "zip" => Type::List,
        "dict" | "enumerate" => Type::Dict,
        "set" | "frozenset" => Type::Set,
        "tuple" => Type::Tuple,
        _ => Type::Unknown,
    }
}

fn expr_type(ti: &TypeInfer, e: &Expr, scope: Option<&HashMap<String, Type>>, fn_name: &str) -> Type {
    match e {
        Expr::IntLit { .. } => Type::Int,
        Expr::FloatLit { .. } => Type::Float,
        Expr::StringLit { .. } => Type::Str,
        Expr::Name { name, .. } => {
            if let Some(sc) = scope {
                if let Some(ty) = sc.get(name) {
                    return *ty;
                }
            }
            if !fn_name.is_empty() {
                if let Some(fv) = ti.fn_vars.get(fn_name) {
                    if let Some(ty) = fv.get(name) {
                        return *ty;
                    }
                }
            }
            ti.module_vars.get(name).copied().unwrap_or(Type::Unknown)
        }
        Expr::ListLit { .. } => Type::List,
        Expr::DictLit { .. } => Type::Dict,
        Expr::SetLit { .. } => Type::Set,
        Expr::BinOp { op, x, y, .. } => {
            let xt = expr_type(ti, x, scope, fn_name);
            let yt = expr_type(ti, y, scope, fn_name);
            match op.as_str() {
                "+" => {
                    if (xt == Type::Int && yt == Type::Int) || (xt == Type::Str && yt == Type::Str) {
                        return xt;
                    }
                }
                "-" | "*" | "//" | "%" | "<<" | ">>" | "&" | "|" | "^" => {
                    if xt == Type::Int && yt == Type::Int {
                        return Type::Int;
                    }
                }
                "/" => return Type::Float,
                "**" => {
                    if xt == Type::Int && yt == Type::Int {
                        return Type::Int;
                    }
                }
                _ => {}
            }
            Type::Unknown
        }
        Expr::UnaryOp { op, x, .. } => {
            if op == "not" {
                return Type::Bool;
            }
            let xt = expr_type(ti, x, scope, fn_name);
            if xt == Type::Int {
                return Type::Int;
            }
            if xt == Type::Float {
                return Type::Float;
            }
            Type::Unknown
        }
        Expr::BoolOp { x, y, .. } => {
            merge_type(expr_type(ti, x, scope, fn_name), expr_type(ti, y, scope, fn_name))
        }
        Expr::Cond { then, els, .. } => {
            merge_type(expr_type(ti, then, scope, fn_name), expr_type(ti, els, scope, fn_name))
        }
        Expr::Call { func, .. } => {
            if let Expr::Name { name, .. } = func.as_ref() {
                if ti.fn_defs.contains_key(name) {
                    return ti.ret_types.get(name).copied().unwrap_or(Type::Unknown);
                }
                return callable_ret(name);
            }
            Type::Unknown
        }
        Expr::ListComp { .. } => Type::List,
        Expr::Subscript { .. }
        | Expr::Attr { .. }
        | Expr::Slice { .. }
        | Expr::EllipsisLit { .. }
        | Expr::TupleLit { .. }
        | Expr::Compare { .. } => Type::Unknown,
    }
}

// BinOpType 返回 + - * // % 的推导类型（用于增强赋值）。
fn bin_op_type(op: &str, a: Type, b: Type) -> Type {
    match op {
        "+" => {
            if (a == Type::Int && b == Type::Int) || (a == Type::Str && b == Type::Str) {
                return a;
            }
        }
        "-" | "*" | "//" | "%" => {
            if a == Type::Int && b == Type::Int {
                return Type::Int;
            }
        }
        _ => {}
    }
    Type::Unknown
}

// IterElemType 返回 for 循环迭代变量的推导类型。
fn iter_elem_type(ti: &TypeInfer, iter: &Expr, scope: Option<&HashMap<String, Type>>, fn_name: &str) -> Type {
    if let Expr::Call { func, .. } = iter {
        if let Expr::Name { name, .. } = func.as_ref() {
            if name == "range" {
                return Type::Int;
            }
        }
    }
    expr_type(ti, iter, scope, fn_name)
}

fn collect_stmts(ti: &mut TypeInfer, stmts: &[Stmt], scope: &mut HashMap<String, Type>, fn_name: &str) {
    for s in stmts {
        match s {
            Stmt::Assign { left, op, right, .. } => {
                let rt = expr_type(ti, right, Some(scope), fn_name);
                if left.len() == 1 {
                    if let Expr::Name { name, .. } = &left[0] {
                        if op == "=" {
                            set_type(scope, name, rt);
                        } else {
                            let lt = scope.get(name).copied().unwrap_or(Type::Unknown);
                            set_type(scope, name, bin_op_type(trim_aug(op), lt, rt));
                        }
                    }
                }
            }
            Stmt::For { target, iter, body, els, .. } => {
                let elem = iter_elem_type(ti, iter, Some(scope), fn_name);
                if let Expr::Name { name, .. } = target.as_ref() {
                    set_type(scope, name, elem);
                }
                collect_stmts(ti, body, scope, fn_name);
                if !els.is_empty() {
                    collect_stmts(ti, els, scope, fn_name);
                }
            }
            Stmt::While { body, els, .. } => {
                collect_stmts(ti, body, scope, fn_name);
                if !els.is_empty() {
                    collect_stmts(ti, els, scope, fn_name);
                }
            }
            Stmt::If { then, elifs, els, .. } => {
                for el in elifs {
                    collect_stmts(ti, &el.body, scope, fn_name);
                }
                collect_stmts(ti, then, scope, fn_name);
                if !els.is_empty() {
                    collect_stmts(ti, els, scope, fn_name);
                }
            }
            Stmt::Try { body, handlers, els, finally, .. } => {
                collect_stmts(ti, body, scope, fn_name);
                for h in handlers {
                    collect_stmts(ti, &h.body, scope, fn_name);
                }
                if !els.is_empty() {
                    collect_stmts(ti, els, scope, fn_name);
                }
                if !finally.is_empty() {
                    collect_stmts(ti, finally, scope, fn_name);
                }
            }
            Stmt::Return { value, .. } => {
                if let Some(v) = value {
                    let rt = expr_type(ti, v, Some(scope), fn_name);
                    match ti.ret_types.get(fn_name) {
                        Some(cur) => {
                            ti.ret_types.insert(fn_name.to_string(), merge_type(*cur, rt));
                        }
                        None => {
                            ti.ret_types.insert(fn_name.to_string(), rt);
                        }
                    }
                }
            }
            _ => {}
        }
    }
}

fn collect_calls(ti: &mut TypeInfer, e: &Expr, scope: Option<&HashMap<String, Type>>, fn_name: &str) {
    match e {
        Expr::Name { .. } | Expr::IntLit { .. } | Expr::FloatLit { .. } | Expr::StringLit { .. } => {}
        Expr::Call { func, args, .. } => {
            if let Expr::Name { name, .. } = func.as_ref() {
                if ti.fn_defs.contains_key(name) {
                    let mut cur = ti.param_types.get(name).cloned().unwrap_or_default();
                    while cur.len() < args.len() {
                        cur.push(Type::Unknown);
                    }
                    for (i, a) in args.iter().enumerate() {
                        let at = expr_type(ti, a, scope, fn_name);
                        if cur[i] == Type::Unknown && !ti.arg_set.get(name).copied().unwrap_or(false) {
                            cur[i] = at;
                            ti.arg_set.insert(name.clone(), true);
                        } else {
                            cur[i] = merge_type(cur[i], at);
                        }
                    }
                    ti.param_types.insert(name.clone(), cur);
                }
            }
            for a in args {
                collect_calls(ti, a, scope, fn_name);
            }
        }
        Expr::BinOp { x, y, .. } => {
            collect_calls(ti, x, scope, fn_name);
            collect_calls(ti, y, scope, fn_name);
        }
        Expr::UnaryOp { x, .. } => collect_calls(ti, x, scope, fn_name),
        Expr::BoolOp { x, y, .. } => {
            collect_calls(ti, x, scope, fn_name);
            collect_calls(ti, y, scope, fn_name);
        }
        Expr::Cond { cond, then, els, .. } => {
            collect_calls(ti, cond, scope, fn_name);
            collect_calls(ti, then, scope, fn_name);
            collect_calls(ti, els, scope, fn_name);
        }
        Expr::Attr { x, .. } => collect_calls(ti, x, scope, fn_name),
        Expr::Subscript { x, index, .. } => {
            collect_calls(ti, x, scope, fn_name);
            collect_calls(ti, index, scope, fn_name);
        }
        Expr::Slice { lo, hi, step, .. } => {
            if let Some(e) = lo {
                collect_calls(ti, e, scope, fn_name);
            }
            if let Some(e) = hi {
                collect_calls(ti, e, scope, fn_name);
            }
            if let Some(e) = step {
                collect_calls(ti, e, scope, fn_name);
            }
        }
        Expr::ListLit { elems, .. } => {
            for el in elems {
                collect_calls(ti, el, scope, fn_name);
            }
        }
        Expr::DictLit { keys, vals, .. } => {
            for (i, k) in keys.iter().enumerate() {
                collect_calls(ti, k, scope, fn_name);
                if i < vals.len() {
                    collect_calls(ti, &vals[i], scope, fn_name);
                }
            }
        }
        Expr::SetLit { elems, .. } => {
            for el in elems {
                collect_calls(ti, el, scope, fn_name);
            }
        }
        Expr::ListComp { elem, clauses, .. } => {
            collect_calls(ti, elem, scope, fn_name);
            for cl in clauses {
                collect_calls(ti, &cl.iter, scope, fn_name);
                for f in &cl.ifs {
                    collect_calls(ti, f, scope, fn_name);
                }
            }
        }
        Expr::TupleLit { elems, .. } => {
            for el in elems {
                collect_calls(ti, el, scope, fn_name);
            }
        }
        Expr::EllipsisLit { .. } | Expr::Compare { .. } => {}
    }
}

// CollectCallsExprStmt 收集语句中嵌套的调用点。
fn collect_calls_stmt(ti: &mut TypeInfer, s: &Stmt, scope: &HashMap<String, Type>, fn_name: &str) {
    match s {
        Stmt::ExprStmt { x, .. } => collect_calls(ti, x, Some(scope), fn_name),
        Stmt::Assign { left, right, .. } => {
            collect_calls(ti, right, Some(scope), fn_name);
            for l in left {
                collect_calls(ti, l, Some(scope), fn_name);
            }
        }
        Stmt::Return { value, .. } => {
            if let Some(v) = value {
                collect_calls(ti, v, Some(scope), fn_name);
            }
        }
        Stmt::If { cond, then, elifs, els, .. } => {
            for el in elifs {
                collect_calls(ti, &el.cond, Some(scope), fn_name);
                for st in &el.body {
                    collect_calls_stmt(ti, st, scope, fn_name);
                }
            }
            collect_calls(ti, cond, Some(scope), fn_name);
            for st in then {
                collect_calls_stmt(ti, st, scope, fn_name);
            }
            if !els.is_empty() {
                for st in els {
                    collect_calls_stmt(ti, st, scope, fn_name);
                }
            }
        }
        Stmt::For { iter, body, .. } => {
            collect_calls(ti, iter, Some(scope), fn_name);
            for st in body {
                collect_calls_stmt(ti, st, scope, fn_name);
            }
        }
        Stmt::While { cond, body, .. } => {
            collect_calls(ti, cond, Some(scope), fn_name);
            for st in body {
                collect_calls_stmt(ti, st, scope, fn_name);
            }
        }
        Stmt::Try { body, handlers, .. } => {
            for st in body {
                collect_calls_stmt(ti, st, scope, fn_name);
            }
            for h in handlers {
                for st in &h.body {
                    collect_calls_stmt(ti, st, scope, fn_name);
                }
            }
        }
        _ => {}
    }
}

impl TypeInfer {
    fn run_round(&mut self) {
        let mut mv: HashMap<String, Type> = HashMap::new();
        for (k, v) in &self.module_vars {
            if *v == Type::Unknown {
                mv.insert(k.clone(), Type::Unknown);
            }
        }
        self.module_vars = mv;
        let stmts = std::mem::replace(&mut self.m.stmts, Vec::new());
        let mut mv = std::mem::replace(&mut self.module_vars, HashMap::new());
        collect_stmts(self, &stmts, &mut mv, "");
        for s in &stmts {
            collect_calls_stmt(self, s, &mv, "");
        }
        self.module_vars = mv;
        self.m.stmts = stmts;

        let fn_defs = self.fn_defs.clone();
        let mut names: Vec<&String> = fn_defs.keys().collect();
        names.sort();
        for name in names {
            let f = match fn_defs.get(name).unwrap() {
                Stmt::FuncDef { params, body, .. } => (params, body),
                _ => unreachable!(),
            };
            let mut scope: HashMap<String, Type> = HashMap::new();
            let params = self.param_types.get(name).cloned().unwrap_or_default();
            for (i, p) in f.0.iter().enumerate() {
                if i < params.len() && !p.name.is_empty() {
                    scope.insert(p.name.clone(), params[i]);
                }
            }
            let prev = self.fn_vars.get(name).cloned().unwrap_or_default();
            let mut kept: HashMap<String, Type> = HashMap::new();
            for (k, v) in &prev {
                if *v == Type::Unknown {
                    kept.insert(k.clone(), Type::Unknown);
                }
            }
            self.fn_vars.insert(name.clone(), kept);
            collect_stmts(self, f.1, &mut scope, name);
            let mut merged_scope = scope.clone();
            for (k, v) in &scope {
                if let Some(pv) = prev.get(k) {
                    let merged = merge_type(*pv, *v);
                    merged_scope.insert(k.clone(), merged);
                }
            }
            self.fn_vars.insert(name.clone(), merged_scope);
            let fscope = self.fn_vars.get(name).cloned().unwrap_or_default();
            for st in f.1 {
                collect_calls_stmt(self, st, &fscope, name);
            }
        }
    }
}

// TrimAug 去掉增强赋值的尾缀（"+=" → "+"）。
fn trim_aug(op: &str) -> &str {
    if op.len() > 1 && op.ends_with('=') {
        &op[..op.len() - 1]
    } else {
        op
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    // 名单与 internal/runtime/fly_runtime.py 的 _FLY_SB_REFLECT 逐项同步，
    // gen 豁免依赖此名单（豁免仅对不在名单的 name 生效）。
    #[test]
    fn reflect_list_sync() {
        let runtime = include_str!("../internal/runtime/fly_runtime.py");
        let mut runtime_names: Vec<&str> = Vec::new();
        let mut in_block = false;
        for line in runtime.lines() {
            let t = line.trim();
            if t.starts_with("_FLY_SB_REFLECT") {
                in_block = true;
                continue;
            }
            if in_block && t == "))" {
                break;
            }
            if in_block && t.starts_with('"') {
                for part in t.split(',') {
                    let p = part.trim().trim_matches('"');
                    if !p.is_empty() {
                        runtime_names.push(p);
                    }
                }
            }
        }
        let mut expect: Vec<&str> = FLY_SB_REFLECT.to_vec();
        expect.sort_unstable();
        runtime_names.sort_unstable();
        let got: Vec<&str> = runtime_names
            .into_iter()
            .filter(|n| {
                (n.starts_with("__") && n.ends_with("__"))
                    || *n == "gi_frame"
                    || *n == "ag_frame"
                    || *n == "cr_frame"
                    || *n == "f_globals"
                    || *n == "f_locals"
            })
            .collect();
        assert_eq!(got, expect, "FLY_SB_REFLECT 与 fly_runtime.py 名单不一致");
    }
}
