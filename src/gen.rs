// gen.rs：Rust 版代码生成器——AST → Python 文本输出（直译 Go 版 internal/gen/gen.go）。
// 职责：8 安全关键字展开（guard/only/trace/cage/seal）、沙箱注入（runtime/sandbox 两节恒注入）、
// 类型推导热路径豁免（int/str 原生运算）、产物审计注释（--keep-annotations）。
// 不依赖 checker：纯 AST 变换，语义检查走 checkd（与 check 同管线）。
use crate::ast::{Expr, Module, Param, Stmt};
use crate::typeinfer::TypeInfer;

// GenOpts 控制代码生成选项。
#[derive(Debug, Clone, Copy, Default)]
pub struct GenOpts {
    // KeepAnnotations 在产物中保留安全关键字审计注释（zero-residual 关键字的审计痕迹）。
    pub keep_annotations: bool,
}

// Generate 生成 Python 源码（默认选项）。
pub fn generate(m: Module) -> String {
    generate_opts(m, GenOpts::default())
}

// GenerateOpts 生成 Python 源码。
pub fn generate_opts(m: Module, opts: GenOpts) -> String {
    let mut ti = TypeInfer::new(m);
    ti.run();
    let mut g = Gen {
        buf: String::new(),
        indent: 0,
        plain: false,
        keep_ann: opts.keep_annotations,
        seal_init: false,
        nc: 0,
        types: &ti,
        cur_fn: String::new(),
    };
    let stmts = ti.stmts();
    let mut doc_end = 0usize;
    if let Some(first) = stmts.first() {
        if let Stmt::ExprStmt { x, .. } = first {
            if matches!(x.as_ref(), Expr::StringLit { .. }) {
                doc_end = 1;
            }
        }
    }
    let guard = needs_guard(stmts);
    let only = needs_only(stmts);
    let trace = needs_trace(stmts);
    let cage = needs_cage(stmts);
    // 沙箱恒注入：所有编译产物在沙箱内运行（拦截逃逸内建/反射链/危险模块导入）。
    let sandbox = true;
    let rt = true;
    for (i, s) in stmts.iter().enumerate() {
        if i == doc_end {
            if doc_end == 1 {
                g.w("\n");
            }
            g.runtime_prelude(guard, only, trace, cage, rt, sandbox);
            if g.keep_ann {
                g.w(&annotations(stmts, &ti));
            }
        }
        g.stmt(s);
    }
    g.buf
}

// Section 按 "# fly:section:<name>" 标记提取运行时片段；未知节返回空串。
pub fn section(name: &str) -> String {
    let mut out = String::new();
    let mut in_sec = false;
    for ln in FLY_RUNTIME.lines() {
        if let Some(rest) = ln.strip_prefix("# fly:section:") {
            in_sec = rest == name;
            continue;
        }
        if in_sec {
            out.push_str(ln);
            out.push('\n');
        }
    }
    out.trim_end_matches('\n').to_string() + "\n"
}

// FLY_RUNTIME 内嵌沙箱运行时（与 Go 版 go:embed 同一文件）。
const FLY_RUNTIME: &str = include_str!("../internal/runtime/fly_runtime.py");

struct Gen<'a> {
    buf: String,
    indent: usize,
    plain: bool,
    keep_ann: bool,
    seal_init: bool,
    nc: usize,
    types: &'a TypeInfer,
    cur_fn: String,
}

impl Gen<'_> {
    fn w(&mut self, s: &str) {
        self.buf.push_str(s);
    }

    fn indent_line(&mut self) {
        self.buf.push_str(&"    ".repeat(self.indent));
    }
}

// annotations 收集各安全关键字的声明位置，生成审计注释块（零残留关键字的产物痕迹）。
fn annotations(stmts: &[Stmt], types: &TypeInfer) -> String {
    let mut sb = String::new();
    fn walk(sb: &mut String, s: &Stmt, types: &TypeInfer) {
        match s {
            Stmt::Safe { names, pos, .. } => {
                for n in names {
                    sb.push_str(&format!("# fly-safe: {} 声明于 {}:{}\n", n, pos.line, pos.col));
                }
            }
            Stmt::Mask { names, pos, .. } => {
                for n in names {
                    sb.push_str(&format!("# fly-mask: {} 声明于 {}:{}\n", n, pos.line, pos.col));
                }
            }
            Stmt::Lock { name, pos, .. } => {
                sb.push_str(&format!("# fly-lock: {} 声明于 {}:{}\n", name, pos.line, pos.col));
            }
            Stmt::Guard { pos, .. } => {
                let msg = guard_msg_plain(s, types);
                sb.push_str(&format!("# fly-guard: {} 声明于 {}:{}\n", msg, pos.line, pos.col));
            }
            Stmt::Only { modules, body, pos } => {
                sb.push_str(&format!(
                    "# fly-only: 白名单 [{}] 开始于 {}:{}\n",
                    modules.join(" "),
                    pos.line,
                    pos.col
                ));
                for s in body {
                    walk(sb, s, types);
                }
            }
            Stmt::ClassDef { name, body, seal, pos, .. } => {
                if *seal {
                    sb.push_str(&format!("# fly-seal: 类 {} 定义于 {}:{}\n", name, pos.line, pos.col));
                }
                for s in body {
                    walk(sb, s, types);
                }
            }
            Stmt::Trace { level, body, pos, .. } => {
                sb.push_str(&format!("# fly-trace: level={} 开始于 {}:{}\n", level, pos.line, pos.col));
                for s in body {
                    walk(sb, s, types);
                }
            }
            Stmt::Cage { body, pos, .. } => {
                sb.push_str(&format!("# fly-cage: 开始于 {}:{}\n", pos.line, pos.col));
                for s in body {
                    walk(sb, s, types);
                }
            }
            Stmt::FuncDef { body, .. } => {
                for s in body {
                    walk(sb, s, types);
                }
            }
            Stmt::If { then, elifs, els, .. } => {
                for s in then {
                    walk(sb, s, types);
                }
                for s in els {
                    walk(sb, s, types);
                }
                for el in elifs {
                    for s in &el.body {
                        walk(sb, s, types);
                    }
                }
            }
            Stmt::For { body, els, .. } => {
                for s in body {
                    walk(sb, s, types);
                }
                for s in els {
                    walk(sb, s, types);
                }
            }
            Stmt::While { body, els, .. } => {
                for s in body {
                    walk(sb, s, types);
                }
                for s in els {
                    walk(sb, s, types);
                }
            }
            Stmt::Try { body, handlers, els, finally, .. } => {
                for s in body {
                    walk(sb, s, types);
                }
                for s in els {
                    walk(sb, s, types);
                }
                for s in finally {
                    walk(sb, s, types);
                }
                for h in handlers {
                    for s in &h.body {
                        walk(sb, s, types);
                    }
                }
            }
            _ => {}
        }
    }
    for s in stmts {
        walk(&mut sb, s, types);
    }
    if sb.is_empty() {
        return String::new();
    }
    sb.push('\n');
    sb
}

// guard_msg_plain 用纯文本模式渲染 guard 消息（plain 子 Gen，与 Go annotations 行为一致）。
fn guard_msg_plain(t: &Stmt, types: &TypeInfer) -> String {
    let Stmt::Guard { name, ty, conds, .. } = t else {
        return String::new();
    };
    let mut g = Gen {
        buf: String::new(),
        indent: 0,
        plain: true,
        keep_ann: false,
        seal_init: false,
        nc: 0,
        types,
        cur_fn: String::new(),
    };
    let mut sb = String::from("guard");
    let mut parts = 0usize;
    if !name.is_empty() {
        sb.push_str(" ");
        sb.push_str(name);
        parts += 1;
    }
    if let Some(ty) = ty {
        sb.push_str(": ");
        sb.push_str(&g.render(ty));
        parts += 1;
    }
    for cond in conds {
        if parts > 0 {
            sb.push(',');
        }
        sb.push(' ');
        sb.push_str(&g.render(cond));
        parts += 1;
    }
    sb
}

impl Gen<'_> {
    fn runtime_prelude(&mut self, guard: bool, only: bool, trace: bool, cage: bool, rt: bool, sandbox: bool) {
        for (name, need) in [
            ("guard", guard),
            ("only", only),
            ("trace", trace),
            ("cage", cage),
            ("runtime", rt),
            ("sandbox", sandbox),
        ] {
            if !need {
                continue;
            }
            let sec = section(name);
            if !sec.is_empty() {
                self.w(&sec);
                self.w("\n");
            }
        }
    }

    fn render(&mut self, e: &Expr) -> String {
        let mut sub = Gen {
            buf: String::new(),
            indent: 0,
            plain: true,
            keep_ann: false,
            seal_init: false,
            nc: 0,
            types: self.types,
            cur_fn: String::new(),
        };
        sub.expr(e, PREC_LOWEST);
        sub.buf.trim().to_string()
    }

    fn guard_msg(&mut self, t: &Stmt) -> String {
        let Stmt::Guard { name, ty, conds, .. } = t else {
            return String::new();
        };
        let mut sb = String::from("guard");
        let mut parts = 0usize;
        if !name.is_empty() {
            sb.push_str(" ");
            sb.push_str(name);
            parts += 1;
        }
        if let Some(ty) = ty {
            sb.push_str(": ");
            sb.push_str(&self.render(ty));
            parts += 1;
        }
        for cond in conds {
            if parts > 0 {
                sb.push(',');
            }
            sb.push(' ');
            sb.push_str(&self.render(cond));
            parts += 1;
        }
        sb
    }

    fn suite(&mut self, body: &[Stmt]) {
        if body.is_empty() {
            self.w(" pass\n");
            return;
        }
        self.w("\n");
        self.indent += 1;
        for s in body {
            self.stmt(s);
        }
        self.indent -= 1;
    }

    fn params(&mut self, params: &[Param]) {
        for (i, p) in params.iter().enumerate() {
            if i > 0 {
                self.w(", ");
            }
            if p.star {
                self.w("*");
            }
            if p.dbl_star {
                self.w("**");
            }
            if !p.name.is_empty() {
                self.w(&p.name);
            }
            if let Some(anno) = &p.anno {
                self.w(": ");
                self.expr(anno, PREC_LOWEST);
            }
            if let Some(def) = &p.default {
                self.w("=");
                self.expr(def, PREC_LOWEST);
            }
        }
    }

    fn func_def(&mut self, t: &Stmt) {
        let Stmt::FuncDef { name, params, return_type, body, decorators, .. } = t else {
            return;
        };
        let prev_fn = std::mem::replace(&mut self.cur_fn, name.clone());
        for d in decorators {
            self.indent_line();
            self.w("@");
            self.expr(d, PREC_LOWEST);
            self.w("\n");
        }
        self.indent_line();
        self.w("def ");
        self.w(name);
        self.w("(");
        self.params(params);
        self.w(")");
        if let Some(rt) = return_type {
            self.w(" -> ");
            self.expr(rt, PREC_LOWEST);
        }
        self.w(":");
        if self.seal_init && name == "__init__" {
            self.w("\n");
            self.indent += 1;
            self.indent_line();
            self.w("object.__setattr__(self, '_fly_seal_initializing', True)\n");
            for s in body {
                self.stmt(s);
            }
            self.indent_line();
            self.w("object.__setattr__(self, '_fly_seal_initializing', False)\n");
            self.indent -= 1;
        } else {
            self.suite(body);
        }
        self.cur_fn = prev_fn;
    }

    fn seal_suite(&mut self, t: &Stmt) {
        let Stmt::ClassDef { body, .. } = t else {
            return;
        };
        if body.is_empty() {
            self.w(" pass\n");
            self.indent_line();
            self.setattr_method(false);
            self.indent_line();
            self.setattr_method(true);
            return;
        }
        self.w("\n");
        self.indent += 1;
        let old = self.seal_init;
        self.seal_init = true;
        for s in body {
            self.stmt(s);
        }
        self.seal_init = old;
        self.setattr_method(false);
        self.setattr_method(true);
        self.indent -= 1;
    }

    fn setattr_method(&mut self, del: bool) {
        let (m, sig, raise) = if del {
            (
                "delattr",
                "(self, name)",
                r#"raise AttributeError("seal 类 %s 的属性 %s 不可删除" % (type(self).__name__, name))"#,
            )
        } else {
            (
                "setattr",
                "(self, name, value)",
                r#"raise AttributeError("seal 类 %s 的属性 %s 不可修改" % (type(self).__name__, name))"#,
            )
        };
        self.indent_line();
        self.w("def __");
        self.w(m);
        self.w("__");
        self.w(sig);
        self.w(":\n");
        self.indent += 1;
        self.indent_line();
        self.w(r#"if _fly_sb_builtins.getattr(self, "_fly_seal_initializing", False):"#);
        self.w("\n");
        self.indent += 1;
        self.indent_line();
        self.w("object.__");
        self.w(m);
        self.w("__(self, name");
        if !del {
            self.w(", value");
        }
        self.w(")\n");
        self.indent -= 1;
        self.indent_line();
        self.w("else:\n");
        self.indent += 1;
        self.indent_line();
        self.w(raise);
        self.w("\n");
        self.indent -= 1;
        self.indent -= 1;
    }

    fn only_stmt(&mut self, t: &Stmt) {
        let Stmt::Only { modules, body, .. } = t else {
            return;
        };
        for m in modules {
            self.indent_line();
            self.w("import ");
            self.w(m);
            self.w("\n");
        }
        self.nc += 1;
        let saved = format!("_fly_ob_{}", char::from_u32('a' as u32 + self.nc as u32).unwrap_or('a'));
        self.indent_line();
        self.w(&saved);
        self.w(" = _fly_sb_module_globals.get(\"__builtins__\", _fly_builtins)\n");
        self.indent_line();
        self.w("__builtins__ = _FlyOnly(");
        self.w(&mods_lit(modules));
        self.w(")\n");
        for s in body {
            self.stmt(s);
            if let Stmt::FuncDef { name, .. } = s {
                self.indent_line();
                self.w(name);
                self.w(" = _fly_patch_builtins(");
                self.w(name);
                self.w(", ");
                self.w(&mods_lit(modules));
                self.w(")\n");
            }
        }
        self.indent_line();
        self.w("__builtins__ = ");
        self.w(&saved);
        self.w("\n");
    }

    fn cage_stmt(&mut self, t: &Stmt) {
        let Stmt::Cage { has_time, max_time, has_mem, max_memory, body, .. } = t else {
            return;
        };
        let mut args: Vec<String> = Vec::new();
        if *has_time {
            args.push(format!("{}", max_time));
        }
        if *has_mem {
            args.push(format!("{}", max_memory));
        }
        for s in body {
            if matches!(s, Stmt::FuncDef { .. }) {
                self.indent_line();
                self.w("@_fly_cage(");
                self.w(&args.join(", "));
                self.w(")\n");
            }
            self.stmt(s);
        }
    }

    fn trace_stmt(&mut self, t: &Stmt) {
        let Stmt::Trace { level, args, ret, body, .. } = t else {
            return;
        };
        let level = if level == "WARN" { "WARNING" } else { level.as_str() };
        for s in body {
            self.trace_body(s, level, *args, *ret);
        }
    }

    fn trace_body(&mut self, s: &Stmt, level: &str, args: bool, ret: bool) {
        let Stmt::FuncDef { .. } = s else {
            self.stmt(s);
            return;
        };
        self.trace_func(s, level, args, ret);
    }

    fn trace_func(&mut self, f: &Stmt, level: &str, args: bool, ret: bool) {
        let Stmt::FuncDef { name, params, body, decorators, .. } = f else {
            return;
        };
        self.nc += 1;
        let idx = self.nc;
        let rt = format!("_fly_ret_{}", char::from_u32('a' as u32 + idx as u32).unwrap_or('a'));
        let er = format!("_fly_err_{}", char::from_u32('a' as u32 + idx as u32).unwrap_or('a'));
        for d in decorators {
            self.indent_line();
            self.w("@");
            self.expr(d, PREC_LOWEST);
            self.w("\n");
        }
        self.indent_line();
        self.w("def ");
        self.w(name);
        self.w("(");
        self.params(params);
        self.w("):\n");
        self.indent += 1;
        self.indent_line();
        if args {
            let mut names: Vec<&str> = Vec::new();
            for p in params {
                if !p.name.is_empty() && !p.star && !p.dbl_star {
                    names.push(&p.name);
                }
            }
            if names.is_empty() {
                self.w(&format!("_fly_log.log(_fly_log.{}", level));
                self.w(&format!(r#", "enter {}")"#, name));
                self.w("\n");
            } else {
                let mut msg = format!("enter {}", name);
                for n in &names {
                    msg.push_str(", ");
                    msg.push_str(n);
                    msg.push_str("=%r");
                }
                self.w(&format!("_fly_log.log(_fly_log.{}", level));
                self.w(", \"");
                self.w(&msg);
                self.w("\", ");
                self.w(&names.join(", "));
                self.w(")\n");
            }
        } else {
            self.w(&format!("_fly_log.log(_fly_log.{}", level));
            self.w(&format!(r#", "enter {}")"#, name));
            self.w("\n");
        }
        self.indent_line();
        self.w("try:\n");
        self.indent += 1;
        self.indent_line();
        self.w(&rt);
        self.w(" = _fly_trace_impl_");
        self.w(name);
        self.w("(");
        self.call_args(params);
        self.w(")\n");
        self.indent -= 1;
        self.indent_line();
        self.w("except BaseException as ");
        self.w(&er);
        self.w(":\n");
        self.indent += 1;
        self.indent_line();
        self.w(&format!("_fly_log.log(_fly_log.{}", level));
        self.w(&format!(r#", "exit {}: raise %r""#, name));
        self.w(", ");
        self.w(&er);
        self.w(")\n");
        self.indent_line();
        self.w("raise\n");
        self.indent -= 1;
        if ret {
            self.indent_line();
            self.w(&format!("_fly_log.log(_fly_log.{}", level));
            self.w(&format!(r#", "exit {}: ret=%r""#, name));
            self.w(", ");
            self.w(&rt);
            self.w(")\n");
        } else {
            self.indent_line();
            self.w(&format!("_fly_log.log(_fly_log.{}", level));
            self.w(&format!(r#", "exit {}")"#, name));
            self.w("\n");
        }
        self.indent_line();
        self.w("return ");
        self.w(&rt);
        self.w("\n");
        self.indent -= 1;
        self.indent_line();
        self.w("def _fly_trace_impl_");
        self.w(name);
        self.w("(");
        self.params(params);
        self.w("):");
        self.suite(body);
    }

    fn call_args(&mut self, params: &[Param]) {
        let mut first = true;
        for p in params {
            if p.name.is_empty() {
                continue;
            }
            if !first {
                self.w(", ");
            }
            first = false;
            if p.star {
                self.w("*");
            }
            if p.dbl_star {
                self.w("**");
            }
            self.w(&p.name);
        }
    }

    fn slice_part(&mut self, e: Option<&Expr>) {
        match e {
            None => self.w("None"),
            Some(e) => self.expr(e, PREC_COND),
        }
    }

    fn index_arg(&mut self, e: &Expr) {
        if let Expr::Slice { lo, hi, step, .. } = e {
            self.w("slice(");
            self.slice_part(lo.as_deref());
            self.w(", ");
            self.slice_part(hi.as_deref());
            if let Some(step) = step {
                self.w(", ");
                self.slice_part(Some(step));
            }
            self.w(")");
            return;
        }
        self.expr(e, PREC_COND);
    }

    fn aug_assign(&mut self, t: &Stmt) {
        let Stmt::Assign { left, op, right, pos } = t else {
            return;
        };
        let base = op.trim_end_matches('=');
        if left.len() == 1 {
            if let Expr::Subscript { x, index, .. } = &left[0] {
                self.indent_line();
                self.w("_fly_set(");
                self.expr(x, PREC_COND);
                self.w(", ");
                self.index_arg(index);
                self.w(", _fly_binop(_fly_get(");
                self.expr(x, PREC_COND);
                self.w(", ");
                self.index_arg(index);
                self.w(&format!(", {}, {}), ", pos.line, pos.col));
                self.expr(right, PREC_COND);
                self.w(&format!(", {:?}, {}, {}), {}, {})", op_name(base), pos.line, pos.col, pos.line, pos.col));
                self.w("\n");
                return;
            }
            if let Expr::Attr { x, name, .. } = &left[0] {
                self.indent_line();
                self.w("_fly_setattr(");
                self.expr(x, PREC_COND);
                self.w(&format!(", {}, _fly_binop(_fly_attr(", py_quote(name)));
                self.expr(x, PREC_COND);
                self.w(&format!(", {}, {}, {}), ", py_quote(name), pos.line, pos.col));
                self.expr(right, PREC_COND);
                self.w(&format!(", {:?}, {}, {}), {}, {})", op_name(base), pos.line, pos.col, pos.line, pos.col));
                self.w("\n");
                return;
            }
        }
        self.indent_line();
        self.expr(&left[0], PREC_LOWEST);
        self.w(" ");
        self.w(op);
        self.w(" ");
        self.expr(right, PREC_LOWEST);
        self.w("\n");
    }

    fn stmt(&mut self, s: &Stmt) {
        match s {
            Stmt::Import { items, pos } => {
                for (i, it) in items.iter().enumerate() {
                    if i > 0 {
                        self.w("\n");
                    }
                    self.indent_line();
                    let bind = if it.alias.is_empty() {
                        it.module().to_string()
                    } else {
                        it.alias.clone()
                    };
                    self.w(&format!("{} = _fly_sb_import(\"{}\", {}, {})\n", bind, it.name, pos.line, pos.col));
                }
            }
            Stmt::FromImport { module, items, pos } => {
                self.indent_line();
                let mod_var = format!("_fly_sb_mod_{}", module.replace('.', "_"));
                let fromlist: Vec<&str> = items.iter().map(|it| it.name.as_str()).collect();
                self.w(&format!(
                    "{} = _fly_sb_import(\"{}\", {}, {}, fromlist=(\"{}\"))\n",
                    mod_var,
                    module,
                    pos.line,
                    pos.col,
                    fromlist.join("\",\"")
                ));
                for it in items {
                    self.indent_line();
                    let bind = if it.alias.is_empty() {
                        it.name.clone()
                    } else {
                        it.alias.clone()
                    };
                    self.w(&format!("{} = {}.{}\n", bind, mod_var, it.name));
                }
            }
            Stmt::Assign { left, op, right, pos } => {
                if op != "=" {
                    self.aug_assign(s);
                    return;
                }
                if left.len() == 1 {
                    if let Expr::Subscript { x, index, .. } = &left[0] {
                        self.indent_line();
                        self.w("_fly_set(");
                        self.expr(x, PREC_COND);
                        self.w(", ");
                        self.index_arg(index);
                        self.w(", ");
                        self.expr(right, PREC_COND);
                        self.w(&format!(", {}, {})", pos.line, pos.col));
                        self.w("\n");
                        return;
                    }
                    if let Expr::Attr { x, name, .. } = &left[0] {
                        self.indent_line();
                        self.w("_fly_setattr(");
                        self.expr(x, PREC_COND);
                        self.w(&format!(", {}, ", py_quote(name)));
                        self.expr(right, PREC_COND);
                        self.w(&format!(", {}, {})", pos.line, pos.col));
                        self.w("\n");
                        return;
                    }
                }
                self.indent_line();
                for (i, l) in left.iter().enumerate() {
                    if i > 0 {
                        self.w(" = ");
                    }
                    self.expr(l, PREC_LOWEST);
                }
                self.w(" ");
                self.w(op);
                self.w(" ");
                self.expr(right, PREC_LOWEST);
                self.w("\n");
            }
            Stmt::Lock { name, value, .. } => {
                if let Some(v) = value {
                    self.indent_line();
                    self.w(name);
                    self.w(" = ");
                    self.expr(v, PREC_LOWEST);
                    self.w("\n");
                }
            }
            Stmt::Safe { .. } | Stmt::Mask { .. } => {}
            Stmt::Guard { name, ty, conds, .. } => {
                self.indent_line();
                self.w("if not (");
                let mut first = true;
                if !name.is_empty() {
                    if let Some(ty) = ty {
                        self.w("isinstance(");
                        self.w(name);
                        self.w(", ");
                        self.expr(ty, PREC_LOWEST);
                        self.w(")");
                        first = false;
                    }
                }
                for cond in conds {
                    if !first {
                        self.w(" and ");
                    }
                    self.w("(");
                    self.expr(cond, PREC_LOWEST);
                    self.w(")");
                    first = false;
                }
                self.w("):\n");
                self.indent += 1;
                self.indent_line();
                let quoted = py_quote(&self.guard_msg(s));
                self.w("raise GuardError(");
                self.w(&quoted);
                self.w(")\n");
                self.indent -= 1;
            }
            Stmt::ExprStmt { x, .. } => {
                self.indent_line();
                self.expr(x, PREC_LOWEST);
                self.w("\n");
            }
            Stmt::FuncDef { .. } => self.func_def(s),
            Stmt::ClassDef { name, bases, body, decorators, seal, .. } => {
                for d in decorators {
                    self.indent_line();
                    self.w("@");
                    self.expr(d, PREC_LOWEST);
                    self.w("\n");
                }
                self.indent_line();
                self.w("class ");
                self.w(name);
                if !bases.is_empty() {
                    self.w("(");
                    for (i, b) in bases.iter().enumerate() {
                        if i > 0 {
                            self.w(", ");
                        }
                        self.expr(b, PREC_LOWEST);
                    }
                    self.w(")");
                }
                self.w(":");
                if *seal {
                    self.seal_suite(s);
                } else {
                    self.suite(body);
                }
            }
            Stmt::Only { .. } => self.only_stmt(s),
            Stmt::Trace { .. } => self.trace_stmt(s),
            Stmt::Cage { .. } => self.cage_stmt(s),
            Stmt::If { cond, then, elifs, els, .. } => {
                self.indent_line();
                self.w("if ");
                self.expr(cond, PREC_LOWEST);
                self.w(":");
                self.suite(then);
                for el in elifs {
                    self.indent_line();
                    self.w("elif ");
                    self.expr(&el.cond, PREC_LOWEST);
                    self.w(":");
                    self.suite(&el.body);
                }
                if !els.is_empty() {
                    self.indent_line();
                    self.w("else:");
                    self.suite(els);
                }
            }
            Stmt::For { target, iter, body, els, pos } => {
                self.indent_line();
                self.w("for ");
                self.expr(target, PREC_LOWEST);
                self.w(" in _fly_iter(");
                self.expr(iter, PREC_COND);
                self.w(&format!(", {}, {}):", pos.line, pos.col));
                self.suite(body);
                if !els.is_empty() {
                    self.indent_line();
                    self.w("else:");
                    self.suite(els);
                }
            }
            Stmt::While { cond, body, els, .. } => {
                self.indent_line();
                self.w("while ");
                self.expr(cond, PREC_LOWEST);
                self.w(":");
                self.suite(body);
                if !els.is_empty() {
                    self.indent_line();
                    self.w("else:");
                    self.suite(els);
                }
            }
            Stmt::Return { value, .. } => {
                self.indent_line();
                self.w("return");
                if let Some(v) = value {
                    self.w(" ");
                    self.expr(v, PREC_LOWEST);
                }
                self.w("\n");
            }
            Stmt::Raise { exc, from, .. } => {
                self.indent_line();
                self.w("raise");
                if let Some(e) = exc {
                    self.w(" ");
                    self.expr(e, PREC_LOWEST);
                }
                if let Some(f) = from {
                    self.w(" from ");
                    self.expr(f, PREC_LOWEST);
                }
                self.w("\n");
            }
            Stmt::Try { body, handlers, els, finally, .. } => {
                self.indent_line();
                self.w("try:");
                self.suite(body);
                for h in handlers {
                    self.indent_line();
                    self.w("except");
                    if let Some(ty) = &h.ty {
                        self.w(" ");
                        self.expr(ty, PREC_LOWEST);
                        if !h.name.is_empty() {
                            self.w(" as ");
                            self.w(&h.name);
                        }
                    }
                    self.w(":");
                    self.suite(&h.body);
                }
                if !els.is_empty() {
                    self.indent_line();
                    self.w("else:");
                    self.suite(els);
                }
                if !finally.is_empty() {
                    self.indent_line();
                    self.w("finally:");
                    self.suite(finally);
                }
            }
            Stmt::Pass { .. } => {
                self.indent_line();
                self.w("pass\n");
            }
            Stmt::Break { .. } => {
                self.indent_line();
                self.w("break\n");
            }
            Stmt::Continue { .. } => {
                self.indent_line();
                self.w("continue\n");
            }
            Stmt::Delete { targets, .. } => {
                self.indent_line();
                self.w("del ");
                for (i, tg) in targets.iter().enumerate() {
                    if i > 0 {
                        self.w(", ");
                    }
                    self.expr(tg, PREC_LOWEST);
                }
                self.w("\n");
            }
        }
    }

    fn expr(&mut self, e: &Expr, parent: i32) {
        if prec_of(e) < parent {
            self.w("(");
            self.expr(e, PREC_LOWEST);
            self.w(")");
            return;
        }
        match e {
            Expr::Name { name, .. } => self.w(name),
            Expr::IntLit { value, .. } => self.w(value),
            Expr::FloatLit { value, .. } => self.w(value),
            Expr::StringLit { value, .. } => self.w(value),
            Expr::EllipsisLit { .. } => self.w("..."),
            Expr::ListLit { elems, .. } => {
                self.w("[");
                for (i, el) in elems.iter().enumerate() {
                    if i > 0 {
                        self.w(", ");
                    }
                    self.expr(el, PREC_COND);
                }
                self.w("]");
            }
            Expr::TupleLit { elems, paren, .. } => {
                if *paren {
                    self.w("(");
                }
                for (i, el) in elems.iter().enumerate() {
                    if i > 0 {
                        self.w(", ");
                    }
                    self.expr(el, PREC_COND);
                }
                if *paren {
                    if elems.len() == 1 {
                        self.w(",");
                    }
                    self.w(")");
                } else if elems.len() == 1 {
                    self.w(",");
                }
            }
            Expr::DictLit { keys, vals, .. } => {
                self.w("{");
                for (i, k) in keys.iter().enumerate() {
                    if i > 0 {
                        self.w(", ");
                    }
                    self.expr(k, PREC_COND);
                    self.w(": ");
                    self.expr(&vals[i], PREC_COND);
                }
                self.w("}");
            }
            Expr::SetLit { elems, .. } => {
                self.w("{");
                for (i, el) in elems.iter().enumerate() {
                    if i > 0 {
                        self.w(", ");
                    }
                    self.expr(el, PREC_COND);
                }
                self.w("}");
            }
            Expr::Call { pos, func, args, kwargs, star, dbl_star } => {
                if !self.plain {
                    if let Expr::Name { name, .. } = func.as_ref() {
                        if name == "int" || name == "float" {
                            self.w("_fly_cast(");
                            self.w(name);
                            for a in args {
                                self.w(", ");
                                self.expr(a, PREC_COND);
                            }
                            self.w(&format!(", line={}, col={})", pos.line, pos.col));
                            return;
                        }
                    }
                }
                self.expr(func, PREC_POST);
                self.w("(");
                let mut first = true;
                for a in args {
                    if !first {
                        self.w(", ");
                    }
                    first = false;
                    self.expr(a, PREC_COND);
                }
                if let Some(st) = star {
                    if !first {
                        self.w(", ");
                    }
                    first = false;
                    self.w("*");
                    self.expr(st, PREC_COND);
                }
                if let Some(ds) = dbl_star {
                    if !first {
                        self.w(", ");
                    }
                    first = false;
                    self.w("**");
                    self.expr(ds, PREC_COND);
                }
                for kw in kwargs {
                    if !first {
                        self.w(", ");
                    }
                    first = false;
                    self.w(&kw.name);
                    self.w("=");
                    self.expr(&kw.value, PREC_COND);
                }
                self.w(")");
            }
            Expr::Attr { pos, x, name } => {
                if self.plain {
                    self.expr(x, PREC_POST);
                    self.w(".");
                    self.w(name);
                    return;
                }
                self.w("_fly_attr(");
                self.expr(x, PREC_COND);
                self.w(&format!(", {}, {}, {})", py_quote(name), pos.line, pos.col));
            }
            Expr::Subscript { pos, x, index } => {
                if self.plain {
                    self.expr(x, PREC_POST);
                    self.w("[");
                    self.expr(index, PREC_LOWEST);
                    self.w("]");
                    return;
                }
                self.w("_fly_get(");
                self.expr(x, PREC_COND);
                self.w(", ");
                if let Expr::Slice { lo, hi, step, .. } = index.as_ref() {
                    self.w("slice(");
                    self.slice_part(lo.as_deref());
                    self.w(", ");
                    self.slice_part(hi.as_deref());
                    if let Some(step) = step {
                        self.w(", ");
                        self.slice_part(Some(step));
                    }
                    self.w(")");
                } else {
                    self.expr(index, PREC_COND);
                }
                self.w(&format!(", {}, {})", pos.line, pos.col));
            }
            Expr::Slice { lo, hi, step, .. } => {
                if let Some(e) = lo {
                    self.expr(e, PREC_COND);
                }
                self.w(":");
                if let Some(e) = hi {
                    self.expr(e, PREC_COND);
                }
                if let Some(e) = step {
                    self.w(":");
                    self.expr(e, PREC_COND);
                }
            }
            Expr::BinOp { pos, op, x, y } => {
                if self.plain {
                    if op == "**" {
                        self.expr(x, PREC_POWER + 1);
                        self.w("**");
                        self.expr(y, PREC_POWER);
                        return;
                    }
                    let p = bin_prec(op);
                    self.expr(x, p);
                    self.w(" ");
                    self.w(op);
                    self.w(" ");
                    self.expr(y, p + 1);
                    return;
                }
                if self.types.plain_bin_op(op, x, y, &self.cur_fn) {
                    let p = bin_prec(op);
                    self.expr(x, p);
                    self.w(" ");
                    self.w(op);
                    self.w(" ");
                    self.expr(y, p + 1);
                    return;
                }
                self.w("_fly_binop(");
                self.expr(x, PREC_COND);
                self.w(", ");
                self.expr(y, PREC_COND);
                self.w(&format!(", {:?}, {}, {})", op_name(op), pos.line, pos.col));
            }
            Expr::UnaryOp { pos, op, x } => {
                if self.plain || op == "not" {
                    if op == "not" {
                        self.w("not ");
                        self.expr(x, PREC_COMPARE);
                        return;
                    }
                    self.w(op);
                    self.w(" ");
                    self.expr(x, PREC_UNARY);
                    return;
                }
                self.w("_fly_unary(");
                self.expr(x, PREC_COND);
                self.w(&format!(", {:?}, {}, {})", unary_name(op), pos.line, pos.col));
            }
            Expr::BoolOp { op, x, y, .. } => {
                let p = if op == "and" { PREC_AND_BOOL } else { PREC_OR_BOOL };
                self.expr(x, p);
                self.w(" ");
                self.w(op);
                self.w(" ");
                self.expr(y, p);
            }
            Expr::Compare { pos, x, ops, ys } => {
                if self.plain {
                    self.expr(x, PREC_COMPARE);
                    for (i, op) in ops.iter().enumerate() {
                        self.w(" ");
                        self.w(op);
                        self.w(" ");
                        self.expr(&ys[i], PREC_COMPARE);
                    }
                    return;
                }
                for (i, op) in ops.iter().enumerate() {
                    if i > 0 {
                        self.w(" and ");
                    }
                    let left = if i > 0 { &ys[i - 1] } else { x.as_ref() };
                    if op == "==" || op == "!=" || op == "is" || op == "is not" {
                        self.expr(left, PREC_COMPARE);
                        self.w(" ");
                        self.w(op);
                        self.w(" ");
                        self.expr(&ys[i], PREC_COMPARE);
                    } else if self.types.plain_bin_op(op, left, &ys[i], &self.cur_fn) {
                        self.expr(left, PREC_COMPARE);
                        self.w(" ");
                        self.w(op);
                        self.w(" ");
                        self.expr(&ys[i], PREC_COMPARE);
                    } else if op == "in" || op == "not in" {
                        if op == "not in" {
                            self.w("not ");
                        }
                        // operator.contains(a, b) 语义为 b in a，交换参数
                        self.w("_fly_cmp(lambda: ");
                        self.expr(&ys[i], PREC_LOWEST);
                        self.w(", lambda: ");
                        self.expr(left, PREC_LOWEST);
                        self.w(&format!(", {:?}, {}, {})", "contains", pos.line, pos.col));
                    } else {
                        self.w("_fly_cmp(lambda: ");
                        self.expr(left, PREC_LOWEST);
                        self.w(", lambda: ");
                        self.expr(&ys[i], PREC_LOWEST);
                        self.w(&format!(", {:?}, {}, {})", cmp_name(op), pos.line, pos.col));
                    }
                }
            }
            Expr::Cond { cond, then, els, .. } => {
                self.expr(then, PREC_COND + 1);
                self.w(" if ");
                self.expr(cond, PREC_COND);
                self.w(" else ");
                self.expr(els, PREC_COND);
            }
            Expr::ListComp { pos, elem, clauses } => {
                self.w("[");
                self.expr(elem, PREC_COND);
                for cl in clauses {
                    self.w(" for ");
                    self.expr(&cl.target, PREC_COND);
                    if self.plain {
                        self.w(" in ");
                        self.expr(&cl.iter, PREC_COND);
                    } else {
                        self.w(" in _fly_iter(");
                        self.expr(&cl.iter, PREC_COND);
                        self.w(&format!(", {}, {})", pos.line, pos.col));
                    }
                    for f in &cl.ifs {
                        self.w(" if ");
                        self.expr(f, PREC_COND);
                    }
                }
                self.w("]");
            }
        }
    }
}

fn mods_lit(mods: &[String]) -> String {
    let q: Vec<String> = mods.iter().map(|m| format!("'{}'", m)).collect();
    format!("({})", q.join(", "))
}

// py_quote 复刻 Go strconv.Quote 语义（产物是 Python，转义序列两者通用）。
fn py_quote(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\x07' => out.push_str("\\a"),
            '\x08' => out.push_str("\\b"),
            '\x0c' => out.push_str("\\f"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            '\x0b' => out.push_str("\\v"),
            c if c >= '\u{20}' && !c.is_control() && c != '\u{7f}' && c != '\u{2028}' && c != '\u{2029}' => {
                out.push(c)
            }
            c if (c as u32) < 0x10000 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push_str(&format!("\\U{:08x}", c as u32)),
        }
    }
    out.push('"');
    out
}

// needs_runtime 判断是否需要注入运行时兜底（_fly_binop/_fly_get 等）。
fn needs_runtime(stmts: &[Stmt]) -> bool {
    fn walk_expr(e: &Expr, found: &mut bool) {
        if *found {
            return;
        }
        match e {
            Expr::Name { .. }
            | Expr::IntLit { .. }
            | Expr::FloatLit { .. }
            | Expr::StringLit { .. }
            | Expr::EllipsisLit { .. } => {}
            Expr::ListLit { elems, .. } => {
                for el in elems {
                    walk_expr(el, found);
                }
            }
            Expr::TupleLit { elems, .. } => {
                for el in elems {
                    walk_expr(el, found);
                }
            }
            Expr::DictLit { keys, vals, .. } => {
                for (i, k) in keys.iter().enumerate() {
                    walk_expr(k, found);
                    if i < vals.len() {
                        walk_expr(&vals[i], found);
                    }
                }
            }
            Expr::SetLit { elems, .. } => {
                for el in elems {
                    walk_expr(el, found);
                }
            }
            Expr::Call { func, args, star, dbl_star, kwargs, .. } => {
                if let Expr::Name { name, .. } = func.as_ref() {
                    if name == "int" || name == "float" {
                        *found = true;
                        return;
                    }
                }
                walk_expr(func, found);
                for a in args {
                    walk_expr(a, found);
                }
                if let Some(st) = star {
                    walk_expr(st, found);
                }
                if let Some(ds) = dbl_star {
                    walk_expr(ds, found);
                }
                for kw in kwargs {
                    walk_expr(&kw.value, found);
                }
            }
            Expr::Attr { .. } => *found = true,
            Expr::Subscript { .. } => *found = true,
            Expr::BinOp { .. } => *found = true,
            Expr::UnaryOp { op, .. } => {
                if op != "not" {
                    *found = true;
                }
            }
            Expr::Compare { .. } => *found = true,
            Expr::BoolOp { .. } => {}
            Expr::Cond { cond, then, els, .. } => {
                walk_expr(cond, found);
                walk_expr(then, found);
                walk_expr(els, found);
            }
            Expr::Slice { lo, hi, step, .. } => {
                if let Some(e) = lo {
                    walk_expr(e, found);
                }
                if let Some(e) = hi {
                    walk_expr(e, found);
                }
                if let Some(e) = step {
                    walk_expr(e, found);
                }
            }
            Expr::ListComp { .. } => *found = true,
        }
    }
    fn walk_stmt(s: &Stmt, found: &mut bool) {
        if *found {
            return;
        }
        match s {
            Stmt::Assign { right, .. } => walk_expr(right, found),
            Stmt::Lock { value, .. } => {
                if let Some(v) = value {
                    walk_expr(v, found);
                }
            }
            Stmt::ExprStmt { x, .. } => walk_expr(x, found),
            Stmt::FuncDef { decorators, params, body, .. } => {
                for d in decorators {
                    walk_expr(d, found);
                }
                for p in params {
                    if let Some(def) = &p.default {
                        walk_expr(def, found);
                    }
                }
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::ClassDef { body, .. } => {
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::Only { body, .. } => {
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::Trace { body, .. } => {
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::Cage { body, .. } => {
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::If { cond, then, elifs, els, .. } => {
                walk_expr(cond, found);
                for st in then {
                    walk_stmt(st, found);
                }
                for el in elifs {
                    walk_expr(&el.cond, found);
                    for st in &el.body {
                        walk_stmt(st, found);
                    }
                }
                for st in els {
                    walk_stmt(st, found);
                }
            }
            Stmt::For { iter, body, .. } => {
                *found = true;
                walk_expr(iter, found);
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::While { cond, body, .. } => {
                walk_expr(cond, found);
                for st in body {
                    walk_stmt(st, found);
                }
            }
            Stmt::Try { body, handlers, .. } => {
                for st in body {
                    walk_stmt(st, found);
                }
                for h in handlers {
                    for st in &h.body {
                        walk_stmt(st, found);
                    }
                }
            }
            Stmt::Return { value, .. } => {
                if let Some(v) = value {
                    walk_expr(v, found);
                }
            }
            Stmt::Raise { exc, .. } => {
                if let Some(e) = exc {
                    walk_expr(e, found);
                }
            }
            Stmt::Guard { ty, conds, .. } => {
                if let Some(ty) = ty {
                    walk_expr(ty, found);
                }
                for c in conds {
                    walk_expr(c, found);
                }
            }
            _ => {}
        }
    }
    let mut found = false;
    for s in stmts {
        walk_stmt(s, &mut found);
        if found {
            return true;
        }
    }
    found
}

fn needs_guard(stmts: &[Stmt]) -> bool {
    for s in stmts {
        match s {
            Stmt::Guard { .. } => return true,
            Stmt::FuncDef { body, .. } => {
                if needs_guard(body) {
                    return true;
                }
            }
            Stmt::ClassDef { body, .. } => {
                if needs_guard(body) {
                    return true;
                }
            }
            Stmt::If { then, elifs, els, .. } => {
                if needs_guard(then) || needs_guard(els) {
                    return true;
                }
                for el in elifs {
                    if needs_guard(&el.body) {
                        return true;
                    }
                }
            }
            Stmt::For { body, els, .. } => {
                if needs_guard(body) || needs_guard(els) {
                    return true;
                }
            }
            Stmt::While { body, els, .. } => {
                if needs_guard(body) || needs_guard(els) {
                    return true;
                }
            }
            Stmt::Try { body, handlers, els, finally, .. } => {
                if needs_guard(body) || needs_guard(els) || needs_guard(finally) {
                    return true;
                }
                for h in handlers {
                    if needs_guard(&h.body) {
                        return true;
                    }
                }
            }
            Stmt::Only { body, .. } => {
                if needs_only(body) {
                    return true;
                }
            }
            Stmt::Trace { body, .. } => {
                if needs_trace(body) {
                    return true;
                }
            }
            _ => {}
        }
    }
    false
}

fn needs_only(stmts: &[Stmt]) -> bool {
    for s in stmts {
        if matches!(s, Stmt::Only { .. }) {
            return true;
        }
        if need_stmt(needs_only, s) {
            return true;
        }
    }
    false
}

fn needs_trace(stmts: &[Stmt]) -> bool {
    for s in stmts {
        if matches!(s, Stmt::Trace { .. }) {
            return true;
        }
        if need_stmt(needs_trace, s) {
            return true;
        }
    }
    false
}

fn needs_cage(stmts: &[Stmt]) -> bool {
    for s in stmts {
        if matches!(s, Stmt::Cage { .. }) {
            return true;
        }
        if need_stmt(needs_cage, s) {
            return true;
        }
    }
    false
}

fn need_stmt(scan: fn(&[Stmt]) -> bool, s: &Stmt) -> bool {
    match s {
        Stmt::FuncDef { body, .. } => scan(body),
        Stmt::ClassDef { body, .. } => scan(body),
        Stmt::If { then, els, elifs, .. } => {
            if scan(then) || scan(els) {
                return true;
            }
            for el in elifs {
                if scan(&el.body) {
                    return true;
                }
            }
            false
        }
        Stmt::For { body, els, .. } => scan(body) || scan(els),
        Stmt::While { body, els, .. } => scan(body) || scan(els),
        Stmt::Try { body, handlers, els, finally, .. } => {
            if scan(body) || scan(els) || scan(finally) {
                return true;
            }
            for h in handlers {
                if scan(&h.body) {
                    return true;
                }
            }
            false
        }
        Stmt::Only { body, .. } => scan(body),
        Stmt::Trace { body, .. } => scan(body),
        Stmt::Cage { body, .. } => scan(body),
        _ => false,
    }
}

const PREC_LOWEST: i32 = -10;
const PREC_TUPLE: i32 = -4;
const PREC_COND: i32 = -3;
const PREC_OR_BOOL: i32 = -2;
const PREC_AND_BOOL: i32 = -1;
const PREC_COMPARE: i32 = 0;
const PREC_OR: i32 = 1;
const PREC_XOR: i32 = 2;
const PREC_AND: i32 = 3;
const PREC_SHIFT: i32 = 4;
const PREC_ADD: i32 = 5;
const PREC_MUL: i32 = 6;
const PREC_UNARY: i32 = 7;
const PREC_POWER: i32 = 8;
const PREC_POST: i32 = 9;
const PREC_ATOM: i32 = 10;

fn bin_prec(op: &str) -> i32 {
    match op {
        "|" => PREC_OR,
        "^" => PREC_XOR,
        "&" => PREC_AND,
        "<<" | ">>" => PREC_SHIFT,
        "+" | "-" => PREC_ADD,
        "*" | "/" | "//" | "%" => PREC_MUL,
        "**" => PREC_POWER,
        _ => PREC_ATOM,
    }
}

fn op_name(op: &str) -> &str {
    match op {
        "+" => "add",
        "-" => "sub",
        "*" => "mul",
        "/" => "truediv",
        "//" => "floordiv",
        "%" => "mod",
        "**" => "pow",
        "<<" => "lshift",
        ">>" => "rshift",
        "&" => "and_",
        "|" => "or_",
        "^" => "xor",
        "@" => "matmul",
        _ => "",
    }
}

fn unary_name(op: &str) -> &str {
    match op {
        "-" => "neg",
        "+" => "pos",
        _ => "invert",
    }
}

fn cmp_name(op: &str) -> &str {
    match op {
        "<" => "lt",
        "<=" => "le",
        ">" => "gt",
        ">=" => "ge",
        _ => "contains",
    }
}

fn prec_of(e: &Expr) -> i32 {
    match e {
        Expr::Name { .. }
        | Expr::IntLit { .. }
        | Expr::FloatLit { .. }
        | Expr::StringLit { .. }
        | Expr::EllipsisLit { .. }
        | Expr::ListLit { .. }
        | Expr::DictLit { .. }
        | Expr::SetLit { .. }
        | Expr::ListComp { .. } => PREC_ATOM,
        Expr::Call { .. } | Expr::Attr { .. } | Expr::Subscript { .. } => PREC_POST,
        Expr::BinOp { op, .. } => bin_prec(op),
        Expr::UnaryOp { op, .. } => {
            if op == "not" {
                PREC_COMPARE
            } else {
                PREC_UNARY
            }
        }
        Expr::BoolOp { op, .. } => {
            if op == "and" {
                PREC_AND_BOOL
            } else {
                PREC_OR_BOOL
            }
        }
        Expr::Compare { .. } => PREC_COMPARE,
        Expr::Cond { .. } => PREC_COND,
        Expr::TupleLit { paren, .. } => {
            if *paren {
                PREC_POST
            } else {
                PREC_TUPLE
            }
        }
        Expr::Slice { .. } => PREC_POST,
    }
}
