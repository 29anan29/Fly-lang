// analyze.rs：fly analyze 的静态分析器——基于 AST 与源码度量
// 循环复杂度（McCabe）/认知复杂度/嵌套/函数长度/参数/重复/错误处理/
// 注释比例/命名规范，输出 100 制评分与分项扣分（与 Go 版 internal/analyze 行为一致）。
use crate::ast::{Expr, Module, Stmt};
use crate::diagnostic::Position;
use crate::fmt;
use crate::parser;

#[derive(Debug, Clone, Default)]
pub struct Metrics {
    pub cyclomatic: i64,
    pub cognitive: i64,
    pub max_nest: i64,
    pub func_count: i64,
    pub max_func_len: i64,
    pub max_params: i64,
    pub try_count: i64,
    pub raise_count: i64,
    pub lines: i64,
    pub code_lines: i64,
    pub comment_rate: f64,
    pub repeat_rate: f64,
    pub name_rate: f64,
    pub functions: Vec<FuncMetric>,
}

#[derive(Debug, Clone)]
pub struct FuncMetric {
    pub name: String,
    pub line: i64,
    pub length: i64,
    pub params: i64,
    pub cyclo: i64,
    pub cognit: i64,
    pub nest: i64,
    pub complex: bool,
}

// Analyze 对单个文件源码执行分析；语法错误返回 None（调用方跳过并报告）。
pub fn analyze(src: &str) -> Option<Metrics> {
    let (mod_, err) = parser::parse(src);
    if err.is_some() {
        return None;
    }
    let mod_ = mod_?;
    let lines: Vec<&str> = src.split('\n').collect();
    let mut metric = Metrics {
        lines: lines.len() as i64,
        ..Default::default()
    };
    let comments = fmt::comment_lines(src);
    let mut comment_set = std::collections::HashSet::new();
    for l in &comments {
        comment_set.insert(*l);
    }
    let mut a = Analyzer::default();
    for s in &mod_.stmts {
        a.stmt(s, 0);
    }
    metric.cyclomatic = a.cyclo + 1;
    metric.cognitive = a.cog;
    metric.max_nest = a.max;
    metric.func_count = a.fns.len() as i64;
    metric.try_count = a.try_;
    metric.raise_count = a.raise;
    metric.functions = a.fns;
    for f in &metric.functions {
        if f.length > metric.max_func_len {
            metric.max_func_len = f.length;
        }
        if f.params > metric.max_params {
            metric.max_params = f.params;
        }
    }
    for (i, l) in lines.iter().enumerate() {
        let t = l.trim();
        if t.is_empty() || comment_set.contains(&(i + 1)) {
            continue;
        }
        metric.code_lines += 1;
    }
    metric.comment_rate = comments.len() as f64 / lines.len() as f64;
    metric.repeat_rate = repeat_rate(&lines, &comment_set);
    metric.name_rate = name_rate(&mod_);
    Some(metric)
}

fn stmt_pos(s: &Stmt) -> Position {
    match s {
        Stmt::Import { pos, .. }
        | Stmt::FromImport { pos, .. }
        | Stmt::Lock { pos, .. }
        | Stmt::Safe { pos, .. }
        | Stmt::Mask { pos, .. }
        | Stmt::Guard { pos, .. }
        | Stmt::Assign { pos, .. }
        | Stmt::ExprStmt { pos, .. }
        | Stmt::FuncDef { pos, .. }
        | Stmt::ClassDef { pos, .. }
        | Stmt::Only { pos, .. }
        | Stmt::Trace { pos, .. }
        | Stmt::Cage { pos, .. }
        | Stmt::If { pos, .. }
        | Stmt::For { pos, .. }
        | Stmt::While { pos, .. }
        | Stmt::Return { pos, .. }
        | Stmt::Raise { pos, .. }
        | Stmt::Try { pos, .. }
        | Stmt::Pass { pos }
        | Stmt::Break { pos }
        | Stmt::Continue { pos }
        | Stmt::Delete { pos, .. } => *pos,
    }
}

#[derive(Default)]
struct Analyzer {
    cyclo: i64,
    cog: i64,
    max: i64,
    fns: Vec<FuncMetric>,
    try_: i64,
    raise: i64,
    cur_max_line: i64,
}

impl Analyzer {
    fn stmt(&mut self, s: &Stmt, depth: i64) {
        if depth + 1 > self.max {
            self.max = depth + 1;
        }
        let ln = stmt_pos(s).line as i64;
        if ln > self.cur_max_line {
            self.cur_max_line = ln;
        }
        match s {
            Stmt::FuncDef { pos, name, params, body, .. } => {
                self.cog += 1 + depth;
                let base = self.cyclo;
                let cog_base = self.cog;
                let max_base = self.max;
                let start = pos.line as i64;
                let prev_max = self.cur_max_line;
                for p in params {
                    if let Some(d) = &p.default {
                        self.expr(d, depth + 1);
                    }
                }
                for b in body {
                    self.stmt(b, depth + 1);
                }
                let cyclo = self.cyclo - base;
                let cognit = self.cog - cog_base;
                let nest = self.max - max_base;
                self.fns.push(FuncMetric {
                    name: name.clone(),
                    line: start,
                    length: self.cur_max_line - start + 1,
                    params: params.len() as i64,
                    cyclo,
                    cognit,
                    nest,
                    complex: cyclo > 10 || cognit > 15 || nest > 4,
                });
                self.cur_max_line = prev_max;
            }
            Stmt::If { cond, then, elifs, els, .. } => {
                self.cyclo += 1;
                self.cog += 1 + depth;
                self.expr(cond, depth + 1);
                for b in then {
                    self.stmt(b, depth + 1);
                }
                for e in elifs {
                    self.cyclo += 1;
                    self.cog += 1 + depth;
                    self.expr(&e.cond, depth + 1);
                    for b in &e.body {
                        self.stmt(b, depth + 1);
                    }
                }
                for b in els {
                    self.stmt(b, depth + 1);
                }
            }
            Stmt::For { target, iter, body, els, .. } => {
                self.cyclo += 1;
                self.cog += 1 + depth;
                self.expr(target, depth + 1);
                self.expr(iter, depth + 1);
                for b in body {
                    self.stmt(b, depth + 1);
                }
                for b in els {
                    self.stmt(b, depth + 1);
                }
            }
            Stmt::While { cond, body, els, .. } => {
                self.cyclo += 1;
                self.cog += 1 + depth;
                self.expr(cond, depth + 1);
                for b in body {
                    self.stmt(b, depth + 1);
                }
                for b in els {
                    self.stmt(b, depth + 1);
                }
            }
            Stmt::Try { body, handlers, els, finally, .. } => {
                self.cyclo += 1;
                self.cog += 1 + depth;
                self.try_ += 1;
                for b in body {
                    self.stmt(b, depth + 1);
                }
                for h in handlers {
                    self.cyclo += 1;
                    self.cog += 1 + depth;
                    for b in &h.body {
                        self.stmt(b, depth + 1);
                    }
                }
                for b in els {
                    self.stmt(b, depth + 1);
                }
                for b in finally {
                    self.stmt(b, depth + 1);
                }
            }
            Stmt::Raise { exc, from, .. } => {
                self.raise += 1;
                if let Some(e) = exc {
                    self.expr(e, depth + 1);
                }
                if let Some(e) = from {
                    self.expr(e, depth + 1);
                }
            }
            Stmt::Assign { left, right, .. } => {
                for l in left {
                    self.expr(l, depth + 1);
                }
                self.expr(right, depth + 1);
            }
            Stmt::ExprStmt { x, .. } => self.expr(x, depth + 1),
            Stmt::Guard { conds, .. } => {
                self.cyclo += 1;
                for c in conds {
                    self.expr(c, depth + 1);
                }
            }
            Stmt::ClassDef { body, .. } => {
                self.cog += 1 + depth;
                for b in body {
                    self.stmt(b, depth + 1);
                }
            }
            Stmt::Lock { value, .. } => {
                if let Some(v) = value {
                    self.expr(v, depth + 1);
                }
            }
            Stmt::Return { value, .. } => {
                if let Some(v) = value {
                    self.expr(v, depth + 1);
                }
            }
            Stmt::Delete { targets, .. } => {
                for x in targets {
                    self.expr(x, depth + 1);
                }
            }
            Stmt::Only { body, .. } | Stmt::Trace { body, .. } | Stmt::Cage { body, .. } => {
                for b in body {
                    self.stmt(b, depth + 1);
                }
            }
            Stmt::Import { .. }
            | Stmt::FromImport { .. }
            | Stmt::Safe { .. }
            | Stmt::Mask { .. }
            | Stmt::Pass { .. }
            | Stmt::Break { .. }
            | Stmt::Continue { .. } => {}
        }
    }

    fn expr(&mut self, e: &Expr, depth: i64) {
        match e {
            Expr::Call { func, args, kwargs, star, dbl_star, .. } => {
                self.expr(func, depth);
                for x in args {
                    self.expr(x, depth);
                }
                if let Some(x) = star {
                    self.expr(x, depth);
                }
                if let Some(x) = dbl_star {
                    self.expr(x, depth);
                }
                for kw in kwargs {
                    self.expr(&kw.value, depth);
                }
            }
            Expr::Attr { x, .. } => self.expr(x, depth),
            Expr::Subscript { x, index, .. } => {
                self.expr(x, depth);
                self.expr(index, depth);
            }
            Expr::Slice { lo, hi, step, .. } => {
                if let Some(e) = lo {
                    self.expr(e, depth);
                }
                if let Some(e) = hi {
                    self.expr(e, depth);
                }
                if let Some(e) = step {
                    self.expr(e, depth);
                }
            }
            Expr::BinOp { x, y, .. } => {
                self.expr(x, depth);
                self.expr(y, depth);
            }
            Expr::UnaryOp { x, .. } => self.expr(x, depth),
            Expr::BoolOp { op, x, y, .. } => {
                if op == "and" || op == "or" {
                    self.cyclo += 1;
                    self.cog += 1 + depth;
                }
                self.expr(x, depth);
                self.expr(y, depth);
            }
            Expr::Compare { x, ys, .. } => {
                self.expr(x, depth);
                for y in ys {
                    self.expr(y, depth);
                }
            }
            Expr::Cond { cond, then, els, .. } => {
                self.cyclo += 1;
                self.cog += 1 + depth;
                self.expr(cond, depth);
                self.expr(then, depth);
                self.expr(els, depth);
            }
            Expr::ListLit { elems, .. } | Expr::TupleLit { elems, .. } | Expr::SetLit { elems, .. } => {
                for el in elems {
                    self.expr(el, depth);
                }
            }
            Expr::DictLit { keys, vals, .. } => {
                for (k, v) in keys.iter().zip(vals.iter()) {
                    self.expr(k, depth);
                    self.expr(v, depth);
                }
            }
            Expr::ListComp { elem, clauses, .. } => {
                self.expr(elem, depth);
                for c in clauses {
                    self.cyclo += 1;
                    self.cog += 1 + depth;
                    self.expr(&c.iter, depth);
                    for cf in &c.ifs {
                        self.cyclo += 1;
                        self.cog += 1 + depth;
                        self.expr(cf, depth);
                    }
                }
            }
            Expr::Name { .. } | Expr::IntLit { .. } | Expr::FloatLit { .. } | Expr::StringLit { .. } | Expr::EllipsisLit { .. } => {}
        }
    }
}

// repeat_rate 重复代码占比：3 行 n-gram 指纹（跳过空行/注释），出现 >1 次的码行比例。
fn repeat_rate(lines: &[&str], comment_set: &std::collections::HashSet<usize>) -> f64 {
    let mut code: Vec<&str> = Vec::with_capacity(lines.len());
    for (i, l) in lines.iter().enumerate() {
        let t = l.trim();
        if t.is_empty() || comment_set.contains(&(i + 1)) {
            code.push("\x00");
        } else {
            code.push(t);
        }
    }
    let mut counts: std::collections::HashMap<String, i64> = std::collections::HashMap::new();
    let n = code.len();
    for i in 0..n.saturating_sub(2) {
        if code[i] == "\x00" || code[i + 1] == "\x00" || code[i + 2] == "\x00" {
            continue;
        }
        let key = format!("{}\x01{}\x01{}", code[i], code[i + 1], code[i + 2]);
        *counts.entry(key).or_insert(0) += 1;
    }
    if n == 0 {
        return 0.0;
    }
    let mut dup = 0i64;
    let mut seen: std::collections::HashSet<usize> = std::collections::HashSet::new();
    for i in 0..n.saturating_sub(2) {
        if code[i] == "\x00" {
            continue;
        }
        let key = format!("{}\x01{}\x01{}", code[i], code[i + 1], code[i + 2]);
        if counts.get(&key).copied().unwrap_or(0) > 1 && !seen.contains(&i) {
            for j in i..i + 3 {
                if !seen.contains(&j) {
                    seen.insert(j);
                    dup += 1;
                }
            }
        }
    }
    dup as f64 / n as f64
}

// snake 判断：^[a-z_][a-z0-9_]*$（Go 版 regexp 等价）。
fn is_snake(n: &str) -> bool {
    let mut chars = n.chars();
    match chars.next() {
        Some(c) if c.is_ascii_lowercase() || c == '_' => {}
        _ => return false,
    }
    chars.all(|c| c.is_ascii_lowercase() || c.is_ascii_digit() || c == '_')
}

// name_rate 非 snake_case 标识符占比（函数名/参数名/赋值目标名）。
fn name_rate(mod_: &Module) -> f64 {
    let mut total = 0i64;
    let mut bad = 0i64;
    let check = |n: &str, total: &mut i64, bad: &mut i64| {
        if n.is_empty() {
            return;
        }
        *total += 1;
        if !is_snake(n) {
            *bad += 1;
        }
    };
    for s in &mod_.stmts {
        match s {
            Stmt::FuncDef { name, params, .. } => {
                check(name, &mut total, &mut bad);
                for p in params {
                    check(&p.name, &mut total, &mut bad);
                }
            }
            Stmt::ClassDef { name, .. } => check(name, &mut total, &mut bad),
            _ => {}
        }
    }
    if total == 0 {
        return 0.0;
    }
    bad as f64 / total as f64
}

// Score 计算 100 制评分与糟糕指数（扣分权重与 Go 版一致）。
pub fn score(m: &Metrics) -> (f64, f64) {
    let mut b = 0.0f64;
    let deduct = |w: f64, b: &mut f64| *b += w;
    if m.cyclomatic > 10 {
        deduct((m.cyclomatic - 10) as f64 / 5.0 * 4.0, &mut b);
    }
    if m.cognitive > 15 {
        deduct((m.cognitive - 15) as f64 / 5.0 * 3.0, &mut b);
    }
    if m.max_nest > 4 {
        deduct((m.max_nest - 4) as f64 * 1.5, &mut b);
    }
    if m.max_func_len > 50 {
        deduct((m.max_func_len - 50) as f64 / 10.0 * 1.5, &mut b);
    }
    if m.lines > 500 {
        deduct((m.lines - 500) as f64 / 100.0 * 1.5, &mut b);
    }
    if m.max_params > 5 {
        deduct((m.max_params - 5) as f64 * 2.0, &mut b);
    }
    deduct(m.repeat_rate * 20.0, &mut b);
    if m.func_count > 0 {
        let covered = m.try_count as f64 / m.func_count as f64;
        deduct((1.0 - covered) * 6.0, &mut b);
    }
    if m.comment_rate < 0.05 {
        deduct(4.0, &mut b);
    } else if m.comment_rate < 0.10 {
        deduct(2.0, &mut b);
    } else if m.comment_rate > 0.40 {
        deduct((m.comment_rate - 0.40) * 10.0, &mut b);
    }
    deduct(m.name_rate * 20.0, &mut b);
    if b > 100.0 {
        b = 100.0;
    }
    (100.0 - b, b)
}

// Level 按评分给出等级文案（对齐 Go 版口径）。
pub fn level(score: f64) -> &'static str {
    if score >= 90.0 {
        "清流 - 代码洁癖者的骄傲"
    } else if score >= 75.0 {
        "略带清香 - 偶尔飘过一丝酸爽"
    } else if score >= 60.0 {
        "屎气扑鼻 - 代码开始散发气味，谨慎维护"
    } else {
        "屎山 - 建议大重构"
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn analyze_simple() {
        let src = "def foo(x):\n    if x > 0:\n        return x\n    return 0\n";
        let m = analyze(src).unwrap();
        assert_eq!(m.cyclomatic, 2);
        assert_eq!(m.func_count, 1);
        assert_eq!(m.functions[0].name, "foo");
        assert!(!m.functions[0].complex);
    }

    #[test]
    fn analyze_syntax_error() {
        assert!(analyze("def foo(:\n").is_none());
    }

    #[test]
    fn score_bounds() {
        let m = Metrics::default();
        let (s, b) = score(&m);
        assert_eq!(s, 96.0);
        assert_eq!(b, 4.0);
    }

    #[test]
    fn is_snake_ok() {
        assert!(is_snake("foo_bar"));
        assert!(is_snake("_x1"));
        assert!(!is_snake("FooBar"));
        assert!(!is_snake("foo-bar"));
    }
}