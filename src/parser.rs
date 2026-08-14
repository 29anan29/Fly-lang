// parser.rs：Rust 版递归下降解析器（与 Go 版 internal/parser 行为一致）。
use crate::ast::{CompClause, ElifClause, ExceptClause, Expr, ImportItem, KwArg, Module, Param, Stmt};
use crate::diagnostic::{error_code, Diagnostic, Position};
use crate::lexer::lexer::{format_msg, Lexer};
use crate::lexer::token::{name, Token, TokenType};

pub struct Parser {
    lex: Lexer,
    tok: Token,
    peeked: Option<Token>,
    err: Option<Diagnostic>,
}

pub fn new(src: &str) -> Parser {
    let mut p = Parser {
        lex: Lexer::new(src),
        tok: Token {
            ty: TokenType::Illegal,
            lit: String::new(),
            line: 0,
            col: 0,
        },
        peeked: None,
        err: None,
    };
    p.next();
    p
}

impl Parser {
    fn next(&mut self) {
        if let Some(t) = self.peeked.take() {
            self.tok = t;
            return;
        }
        self.tok = self.lex.next();
    }

    fn peek(&mut self) -> Token {
        if let Some(t) = &self.peeked {
            return t.clone();
        }
        let t = self.lex.next();
        self.peeked = Some(t.clone());
        t
    }

    fn errorf(&mut self, pos: Position, format: &str, args: &[String]) {
        if self.err.is_none() {
            self.err = Some(Diagnostic {
                pos,
                code: error_code(format).to_string(),
                msg: format_msg(format, args),
            });
        }
    }

    fn err(&mut self) -> Option<Diagnostic> {
        self.err.clone().or_else(|| self.lex.err().cloned())
    }

    fn expect(&mut self, tt: TokenType) -> Token {
        if self.tok.ty != tt {
            let lit = self.tok.lit.clone();
            let line = self.tok.line;
            let col = self.tok.col;
            let msg = format!("期望 {}，实际为 {}", name(tt), lit);
            let t = Token {
                ty: TokenType::Illegal,
                lit: String::new(),
                line,
                col,
            };
            if self.err.is_none() {
                self.err = Some(Diagnostic {
                    pos: self.tok.pos(),
                    code: error_code("期望 %s，实际为 %s").to_string(),
                    msg,
                });
            }
            return t;
        }
        let t = self.tok.clone();
        self.next();
        t
    }

    fn parse_module(&mut self) -> Option<Module> {
        if self.lex.err().is_some() {
            return None;
        }
        let mut stmts = Vec::new();
        while self.tok.ty != TokenType::Eof && self.err.is_none() {
            if let Some(s) = self.statement() {
                stmts.push(s);
            }
        }
        Some(Module::new(self.tok.pos(), stmts))
    }

    fn statement(&mut self) -> Option<Stmt> {
        match self.tok.ty {
            TokenType::Newline => {
                self.next();
                None
            }
            TokenType::Semicolon => {
                while self.tok.ty == TokenType::Semicolon {
                    self.next();
                }
                None
            }
            TokenType::Def => self.func_def(None),
            TokenType::Class => self.class_def(None, false),
            TokenType::If => self.if_stmt(),
            TokenType::For => self.for_stmt(),
            TokenType::While => self.while_stmt(),
            TokenType::Return => {
                let pos = self.tok.pos();
                self.next();
                let value = if !self.at_end() {
                    Some(Box::new(self.parse_test_list()))
                } else {
                    None
                };
                self.stmt_end();
                Some(Stmt::Return { pos, value })
            }
            TokenType::Raise => {
                let pos = self.tok.pos();
                self.next();
                let mut exc = if !self.at_end() {
                    Some(Box::new(self.parse_test_list()))
                } else {
                    None
                };
                let from = if self.tok.ty == TokenType::From {
                    self.next();
                    Some(Box::new(self.parse_test_list()))
                } else {
                    None
                };
                if exc.is_none() {
                    exc = None;
                }
                self.stmt_end();
                Some(Stmt::Raise { pos, exc, from })
            }
            TokenType::Try => self.try_stmt(),
            TokenType::Pass => {
                let pos = self.tok.pos();
                self.next();
                self.stmt_end();
                Some(Stmt::Pass { pos })
            }
            TokenType::Break => {
                let pos = self.tok.pos();
                self.next();
                self.stmt_end();
                Some(Stmt::Break { pos })
            }
            TokenType::Continue => {
                let pos = self.tok.pos();
                self.next();
                self.stmt_end();
                Some(Stmt::Continue { pos })
            }
            TokenType::Del => {
                let pos = self.tok.pos();
                self.next();
                let mut targets = Vec::new();
                loop {
                    targets.push(self.parse_test_list());
                    if self.tok.ty != TokenType::Comma {
                        break;
                    }
                    self.next();
                }
                self.stmt_end();
                Some(Stmt::Delete { pos, targets })
            }
            TokenType::Import => self.import_stmt(),
            TokenType::From => self.from_import_stmt(),
            TokenType::At => self.decorated_stmt(),
            TokenType::Assert
            | TokenType::With
            | TokenType::Yield
            | TokenType::Global
            | TokenType::Nonlocal
            | TokenType::Lambda
            | TokenType::Async
            | TokenType::Await => {
                let lit = self.tok.lit.clone();
                self.errorf(self.tok.pos(), "关键字 %s 暂不支持", &[lit]);
                None
            }
            TokenType::Lock => self.lock_stmt(),
            TokenType::Guard => self.guard_stmt(),
            TokenType::Safe | TokenType::Mask => {
                self.taint_decl_stmt(self.tok.ty == TokenType::Safe)
            }
            TokenType::Only => self.only_stmt(),
            TokenType::Seal => self.seal_stmt(),
            TokenType::Trace => self.trace_stmt(),
            TokenType::Cage => self.cage_stmt(),
            _ => self.simple_stmt(),
        }
    }

    fn at_end(&self) -> bool {
        self.tok.ty == TokenType::Newline || self.tok.ty == TokenType::Eof || self.tok.ty == TokenType::Semicolon
    }

    fn stmt_end(&mut self) {
        while self.tok.ty == TokenType::Semicolon {
            self.next();
        }
        if self.tok.ty != TokenType::Newline && self.tok.ty != TokenType::Eof {
            let lit = self.tok.lit.clone();
            self.errorf(self.tok.pos(), "期望语句结束，实际为 %q", &[lit]);
        }
        if self.tok.ty == TokenType::Newline {
            self.next();
        }
    }

    fn simple_stmt(&mut self) -> Option<Stmt> {
        let mut first: Option<Stmt> = None;
        loop {
            let exprs = self.parse_test_list();
            if matches!(&exprs, Expr::Name { name, .. } if name.is_empty()) {
                return None;
            }
            let s = if self.tok.ty == TokenType::Assign || self.is_aug_assign() {
                let mut left = vec![exprs];
                let mut op = self.tok.lit.clone();
                self.next();
                let mut right = self.parse_test_list();
                if self.tok.ty == TokenType::Assign {
                    loop {
                        if !self.check_target(&right) {
                            let pos = right.pos();
                            self.errorf(pos, "非法赋值目标", &[]);
                        }
                        left.push(right);
                        op = "=".to_string();
                        self.next();
                        right = self.parse_test_list();
                        if self.tok.ty != TokenType::Assign {
                            break;
                        }
                    }
                }
                if !self.check_target(&left[0]) {
                    let pos = left[0].pos();
                    self.errorf(pos, "非法赋值目标", &[]);
                }
                let pos = left[0].pos();
                Stmt::Assign {
                    pos,
                    left,
                    op,
                    right: Box::new(right),
                }
            } else {
                let pos = exprs.pos();
                Stmt::ExprStmt {
                    pos,
                    x: Box::new(exprs),
                }
            };
            if first.is_none() {
                first = Some(s);
            }
            if self.tok.ty != TokenType::Semicolon {
                break;
            }
            self.next();
            if self.at_end() {
                break;
            }
        }
        self.stmt_end();
        first
    }

    fn is_aug_assign(&self) -> bool {
        matches!(
            self.tok.ty,
            TokenType::PlusAssign
                | TokenType::MinusAssign
                | TokenType::StarAssign
                | TokenType::SlashAssign
                | TokenType::PercentAssign
                | TokenType::DoubleStarAssign
                | TokenType::FloorDivAssign
                | TokenType::ShlAssign
                | TokenType::ShrAssign
                | TokenType::AmpAssign
                | TokenType::PipeAssign
                | TokenType::CaretAssign
        )
    }

    fn check_target(&self, e: &Expr) -> bool {
        match e {
            Expr::Name { .. } | Expr::Attr { .. } | Expr::Subscript { .. } => true,
            Expr::TupleLit { elems, .. } | Expr::ListLit { elems, .. } => elems.iter().all(|el| self.check_target(el)),
            Expr::UnaryOp { op, x, .. } => {
                if op == "*" {
                    matches!(&**x, Expr::Name { .. })
                } else {
                    false
                }
            }
            _ => false,
        }
    }

    fn lock_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let mut name = String::new();
        if self.tok.ty == TokenType::Ident {
            name = self.tok.lit.clone();
            self.next();
        }
        let value = if self.tok.ty == TokenType::Assign {
            self.next();
            let v = self.parse_test_list();
            if name.is_empty() {
                let p = pos;
                self.errorf(p, "lock 需要一个变量名", &[]);
            }
            Some(Box::new(v))
        } else {
            if name.is_empty() {
                let p = pos;
                self.errorf(p, "lock 需要一个变量名", &[]);
            }
            None
        };
        self.stmt_end();
        Some(Stmt::Lock { pos, name, value })
    }

    fn taint_decl_stmt(&mut self, safe: bool) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let mut names = Vec::new();
        loop {
            let t = self.expect(TokenType::Ident);
            names.push(t.lit);
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
        }
        self.stmt_end();
        if safe {
            Some(Stmt::Safe { pos, names })
        } else {
            Some(Stmt::Mask { pos, names })
        }
    }

    fn guard_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        if self.tok.ty == TokenType::Ident && self.peek().ty == TokenType::Colon {
            let name = self.tok.lit.clone();
            self.next();
            self.next();
            let ty = self.parse_test();
            let mut conds = Vec::new();
            while self.tok.ty == TokenType::Comma {
                self.next();
                conds.push(self.parse_test());
            }
            self.stmt_end();
            return Some(Stmt::Guard {
                pos,
                name,
                ty: Some(Box::new(ty)),
                conds,
            });
        }
        let exprs = self.parse_test_list();
        let conds = match exprs {
            Expr::TupleLit { elems, .. } => elems,
            other => vec![other],
        };
        self.stmt_end();
        Some(Stmt::Guard {
            pos,
            name: String::new(),
            ty: None,
            conds,
        })
    }

    fn func_def(&mut self, decorators: Option<Vec<Expr>>) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let name = self.expect(TokenType::Ident).lit;
        self.expect(TokenType::LParen);
        let params = self.params();
        self.expect(TokenType::RParen);
        let return_type = if self.tok.ty == TokenType::Arrow {
            self.next();
            Some(Box::new(self.parse_test()))
        } else {
            None
        };
        let body = self.suite();
        Some(Stmt::FuncDef {
            pos,
            name,
            params,
            return_type,
            body,
            decorators: decorators.unwrap_or_default(),
        })
    }

    fn params(&mut self) -> Vec<Param> {
        let mut params = Vec::new();
        if self.tok.ty == TokenType::RParen {
            return params;
        }
        loop {
            let mut prm = Param {
                name: String::new(),
                anno: None,
                default: None,
                star: false,
                dbl_star: false,
            };
            if self.tok.ty == TokenType::Star {
                self.next();
                if self.tok.ty == TokenType::Ident {
                    prm.star = true;
                    prm.name = self.expect(TokenType::Ident).lit;
                } else {
                    prm.star = true;
                }
            } else if self.tok.ty == TokenType::DoubleStar {
                self.next();
                prm.dbl_star = true;
                prm.name = self.expect(TokenType::Ident).lit;
            } else {
                prm.name = self.expect(TokenType::Ident).lit;
            }
            if self.tok.ty == TokenType::Colon {
                self.next();
                prm.anno = Some(Box::new(self.parse_test()));
            }
            if self.tok.ty == TokenType::Assign {
                self.next();
                prm.default = Some(Box::new(self.parse_test()));
            }
            params.push(prm);
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
            if self.tok.ty == TokenType::RParen {
                break;
            }
        }
        params
    }

    fn only_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        self.expect(TokenType::LParen);
        let mut modules = Vec::new();
        while self.tok.ty != TokenType::RParen {
            let t = self.expect(TokenType::Ident);
            modules.push(t.lit);
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
        }
        self.expect(TokenType::RParen);
        let body = self.suite();
        Some(Stmt::Only {
            pos,
            modules,
            body,
        })
    }

    fn seal_stmt(&mut self) -> Option<Stmt> {
        self.next();
        if self.tok.ty != TokenType::Class {
            self.errorf(self.tok.pos(), "seal 后必须跟随 class 定义", &[]);
            return None;
        }
        self.class_def(None, true)
    }

    fn trace_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let mut level = "WARNING".to_string();
        let mut args = true;
        let mut ret = true;
        self.expect(TokenType::LParen);
        while self.tok.ty != TokenType::RParen {
            let name = self.expect(TokenType::Ident).lit;
            self.expect(TokenType::Assign);
            match name.as_str() {
                "level" => {
                    if self.tok.ty != TokenType::String {
                        self.errorf(self.tok.pos(), "level 必须是字符串（如 \"WARN\"）", &[]);
                        return None;
                    }
                    level = unquote_str(&self.tok.lit);
                    self.next();
                }
                "args" => {
                    if self.tok.ty != TokenType::True && self.tok.ty != TokenType::False {
                        self.errorf(self.tok.pos(), "args 必须是 True 或 False", &[]);
                        return None;
                    }
                    args = self.tok.ty == TokenType::True;
                    self.next();
                }
                "ret" => {
                    if self.tok.ty != TokenType::True && self.tok.ty != TokenType::False {
                        self.errorf(self.tok.pos(), "ret 必须是 True 或 False", &[]);
                        return None;
                    }
                    ret = self.tok.ty == TokenType::True;
                    self.next();
                }
                _ => {
                    self.errorf(self.tok.pos(), "trace 参数 %s 未知（支持 level/args/ret）", &[name]);
                    return None;
                }
            }
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
        }
        self.expect(TokenType::RParen);
        let body = self.suite();
        Some(Stmt::Trace {
            pos,
            level,
            args,
            ret,
            body,
        })
    }

    fn cage_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let mut has_time = false;
        let mut max_time = 0.0;
        let mut has_mem = false;
        let mut max_memory: i64 = 0;
        self.expect(TokenType::LParen);
        while self.tok.ty != TokenType::RParen {
            let name = self.expect(TokenType::Ident).lit;
            self.expect(TokenType::Assign);
            if self.tok.ty != TokenType::String {
                self.errorf(self.tok.pos(), "cage 参数 %s 必须是字符串（如 max_time=\"5s\"）", &[name]);
                return None;
            }
            let val = unquote_str(&self.tok.lit);
            self.next();
            match name.as_str() {
                "max_time" => {
                    match parse_time_spec(&val) {
                        Some(secs) => {
                            has_time = true;
                            max_time = secs;
                        }
                        None => {
                            self.errorf(self.tok.pos(), "max_time 格式非法：%q（支持 500ms/5s/2m/1h）", &[val]);
                            return None;
                        }
                    }
                }
                "max_memory" => {
                    match parse_mem_spec(&val) {
                        Some(bytes) => {
                            has_mem = true;
                            max_memory = bytes;
                        }
                        None => {
                            self.errorf(self.tok.pos(), "max_memory 格式非法：%q（支持 64KB/100MB/2GB）", &[val]);
                            return None;
                        }
                    }
                }
                _ => {
                    self.errorf(self.tok.pos(), "cage 参数 %s 未知（支持 max_time/max_memory）", &[name]);
                    return None;
                }
            }
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
        }
        self.expect(TokenType::RParen);
        if !has_time && !has_mem {
            self.errorf(pos, "cage 需至少指定 max_time 或 max_memory 之一", &[]);
            return None;
        }
        let body = self.suite();
        Some(Stmt::Cage {
            pos,
            has_time,
            max_time,
            has_mem,
            max_memory,
            body,
        })
    }

    fn class_def(&mut self, decorators: Option<Vec<Expr>>, seal: bool) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let name = self.expect(TokenType::Ident).lit;
        let mut bases = Vec::new();
        if self.tok.ty == TokenType::LParen {
            self.next();
            if self.tok.ty != TokenType::RParen {
                loop {
                    bases.push(self.parse_test());
                    if self.tok.ty != TokenType::Comma {
                        break;
                    }
                    self.next();
                }
            }
            self.expect(TokenType::RParen);
        }
        let body = self.suite();
        Some(Stmt::ClassDef {
            pos,
            name,
            bases,
            body,
            decorators: decorators.unwrap_or_default(),
            seal,
        })
    }

    fn if_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let cond = self.parse_test_list();
        let then = self.suite();
        let mut elifs = Vec::new();
        while self.tok.ty == TokenType::Elif {
            let pos = self.tok.pos();
            self.next();
            let cond = self.parse_test_list();
            let body = self.suite();
            elifs.push(ElifClause {
                pos,
                cond: Box::new(cond),
                body,
            });
        }
        let els = if self.tok.ty == TokenType::Else {
            self.next();
            self.suite()
        } else {
            Vec::new()
        };
        Some(Stmt::If {
            pos,
            cond: Box::new(cond),
            then,
            elifs,
            els,
        })
    }

    fn for_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let target = self.parse_expr_list();
        self.expect(TokenType::In);
        let iter = self.parse_test_list();
        let body = self.suite();
        let els = if self.tok.ty == TokenType::Else {
            self.next();
            self.suite()
        } else {
            Vec::new()
        };
        Some(Stmt::For {
            pos,
            target: Box::new(target),
            iter: Box::new(iter),
            body,
            els,
        })
    }

    fn while_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let cond = self.parse_test_list();
        let body = self.suite();
        let els = if self.tok.ty == TokenType::Else {
            self.next();
            self.suite()
        } else {
            Vec::new()
        };
        Some(Stmt::While {
            pos,
            cond: Box::new(cond),
            body,
            els,
        })
    }

    fn try_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let body = self.suite();
        let mut handlers = Vec::new();
        while self.tok.ty == TokenType::Except {
            let pos = self.tok.pos();
            self.next();
            let mut ty = None;
            let mut name = String::new();
            if self.tok.ty != TokenType::Colon {
                ty = Some(Box::new(self.parse_test()));
                if self.tok.ty == TokenType::As {
                    self.next();
                    name = self.expect(TokenType::Ident).lit;
                }
            }
            let body = self.suite();
            handlers.push(ExceptClause {
                pos,
                ty,
                name,
                body,
            });
        }
        let els = if self.tok.ty == TokenType::Else {
            self.next();
            self.suite()
        } else {
            Vec::new()
        };
        let finally = if self.tok.ty == TokenType::Finally {
            self.next();
            self.suite()
        } else {
            Vec::new()
        };
        Some(Stmt::Try {
            pos,
            body,
            handlers,
            els,
            finally,
        })
    }

    fn import_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let mut items = Vec::new();
        loop {
            let mut item = ImportItem {
                name: self.dotted_name(),
                alias: String::new(),
            };
            if self.tok.ty == TokenType::As {
                self.next();
                item.alias = self.expect(TokenType::Ident).lit;
            }
            items.push(item);
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
        }
        self.stmt_end();
        Some(Stmt::Import { pos, items })
    }

    fn from_import_stmt(&mut self) -> Option<Stmt> {
        let pos = self.tok.pos();
        self.next();
        let mut module = String::new();
        while self.tok.ty == TokenType::Dot {
            module.push('.');
            self.next();
        }
        if self.tok.ty == TokenType::Ident {
            module.push_str(&self.dotted_name());
        }
        self.expect(TokenType::Import);
        let mut items = Vec::new();
        if self.tok.ty == TokenType::Star {
            self.next();
            items.push(ImportItem {
                name: "*".to_string(),
                alias: String::new(),
            });
        } else {
            loop {
                if self.tok.ty == TokenType::Star {
                    self.next();
                    items.push(ImportItem {
                        name: "*".to_string(),
                        alias: String::new(),
                    });
                    break;
                }
                let mut item = ImportItem {
                    name: self.expect(TokenType::Ident).lit,
                    alias: String::new(),
                };
                if self.tok.ty == TokenType::As {
                    self.next();
                    item.alias = self.expect(TokenType::Ident).lit;
                }
                items.push(item);
                if self.tok.ty != TokenType::Comma {
                    break;
                }
                self.next();
            }
        }
        self.stmt_end();
        Some(Stmt::FromImport {
            pos,
            module,
            items,
        })
    }

    fn dotted_name(&mut self) -> String {
        let mut parts = vec![self.expect(TokenType::Ident).lit];
        while self.tok.ty == TokenType::Dot {
            self.next();
            parts.push(self.expect(TokenType::Ident).lit);
        }
        parts.join(".")
    }

    fn decorated_stmt(&mut self) -> Option<Stmt> {
        let mut decs = Vec::new();
        while self.tok.ty == TokenType::At {
            self.next();
            decs.push(self.parse_test_list());
            if self.tok.ty == TokenType::Newline {
                self.next();
                continue;
            }
            self.stmt_end();
            break;
        }
        match self.tok.ty {
            TokenType::Def => self.func_def(Some(decs)),
            TokenType::Class => self.class_def(Some(decs), false),
            _ => {
                self.errorf(self.tok.pos(), "装饰器后必须跟随 def 或 class", &[]);
                None
            }
        }
    }

    fn suite(&mut self) -> Vec<Stmt> {
        self.expect(TokenType::Colon);
        if self.tok.ty == TokenType::Newline {
            self.next();
            self.expect(TokenType::Indent);
            let mut stmts = Vec::new();
            while self.tok.ty != TokenType::Dedent
                && self.tok.ty != TokenType::Eof
                && self.err.is_none()
            {
                if let Some(s) = self.statement() {
                    stmts.push(s);
                }
            }
            self.expect(TokenType::Dedent);
            return stmts;
        }
        let mut stmts = Vec::new();
        loop {
            if let Some(s) = self.simple_stmt() {
                stmts.push(s);
            }
            if self.tok.ty != TokenType::Semicolon {
                break;
            }
            self.next();
            if self.at_end() {
                break;
            }
        }
        stmts
    }

    fn parse_test_list(&mut self) -> Expr {
        let pos = self.tok.pos();
        let first = self.test_item();
        if self.tok.ty != TokenType::Comma {
            return first;
        }
        let mut elems = vec![first];
        while self.tok.ty == TokenType::Comma {
            self.next();
            if self.at_end()
                || self.tok.ty == TokenType::Colon
                || self.tok.ty == TokenType::RParen
                || self.tok.ty == TokenType::RBracket
                || self.tok.ty == TokenType::RBrace
            {
                break;
            }
            elems.push(self.test_item());
        }
        Expr::TupleLit {
            pos,
            elems,
            paren: false,
        }
    }

    fn test_item(&mut self) -> Expr {
        if self.tok.ty == TokenType::Star {
            let pos = self.tok.pos();
            self.next();
            let x = self.parse_test();
            return Expr::UnaryOp {
                pos,
                op: "*".to_string(),
                x: Box::new(x),
            };
        }
        self.parse_test()
    }

    fn parse_expr_list(&mut self) -> Expr {
        let pos = self.tok.pos();
        let first = self.expr_item();
        if self.tok.ty != TokenType::Comma {
            return first;
        }
        let mut elems = vec![first];
        while self.tok.ty == TokenType::Comma {
            self.next();
            if self.at_end() || self.tok.ty == TokenType::Colon {
                break;
            }
            elems.push(self.expr_item());
        }
        Expr::TupleLit {
            pos,
            elems,
            paren: false,
        }
    }

    fn expr_item(&mut self) -> Expr {
        if self.tok.ty == TokenType::Star {
            let pos = self.tok.pos();
            self.next();
            let x = self.parse_expr();
            return Expr::UnaryOp {
                pos,
                op: "*".to_string(),
                x: Box::new(x),
            };
        }
        self.parse_expr()
    }

    fn parse_test(&mut self) -> Expr {
        let x = self.parse_or_test();
        if self.tok.ty == TokenType::If {
            let pos = x.pos();
            self.next();
            let cond = self.parse_or_test();
            self.expect(TokenType::Else);
            let els = self.parse_test();
            return Expr::Cond {
                pos,
                cond: Box::new(cond),
                then: Box::new(x),
                els: Box::new(els),
            };
        }
        x
    }

    fn parse_expr(&mut self) -> Expr {
        self.parse_bit_or()
    }

    fn parse_or_test(&mut self) -> Expr {
        let mut x = self.parse_and_test();
        while self.tok.ty == TokenType::Or {
            let pos = self.tok.pos();
            let op = self.tok.lit.clone();
            self.next();
            let y = self.parse_and_test();
            x = Expr::BoolOp {
                pos,
                op,
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_and_test(&mut self) -> Expr {
        let mut x = self.parse_not_test();
        while self.tok.ty == TokenType::And {
            let pos = self.tok.pos();
            let op = self.tok.lit.clone();
            self.next();
            let y = self.parse_not_test();
            x = Expr::BoolOp {
                pos,
                op,
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_not_test(&mut self) -> Expr {
        if self.tok.ty == TokenType::Not {
            let pos = self.tok.pos();
            self.next();
            let x = self.parse_not_test();
            return Expr::UnaryOp {
                pos,
                op: "not".to_string(),
                x: Box::new(x),
            };
        }
        self.parse_comparison()
    }

    fn parse_comparison(&mut self) -> Expr {
        let x = self.parse_bit_or();
        if !self.is_comp_op() {
            return x;
        }
        let pos = x.pos();
        let mut ops = Vec::new();
        let mut ys = Vec::new();
        while self.is_comp_op() {
            ops.push(self.comp_op());
            ys.push(self.parse_bit_or());
        }
        Expr::Compare {
            pos,
            x: Box::new(x),
            ops,
            ys,
        }
    }

    fn is_comp_op(&self) -> bool {
        matches!(
            self.tok.ty,
            TokenType::Lt
                | TokenType::Gt
                | TokenType::Le
                | TokenType::Ge
                | TokenType::EqEq
                | TokenType::Ne
                | TokenType::In
                | TokenType::Is
                | TokenType::Not
        )
    }

    fn comp_op(&mut self) -> String {
        match self.tok.ty {
            TokenType::Lt => {
                self.next();
                "<".to_string()
            }
            TokenType::Gt => {
                self.next();
                ">".to_string()
            }
            TokenType::Le => {
                self.next();
                "<=".to_string()
            }
            TokenType::Ge => {
                self.next();
                ">=".to_string()
            }
            TokenType::EqEq => {
                self.next();
                "==".to_string()
            }
            TokenType::Ne => {
                self.next();
                "!=".to_string()
            }
            TokenType::In => {
                self.next();
                "in".to_string()
            }
            TokenType::Is => {
                self.next();
                if self.tok.ty == TokenType::Not {
                    self.next();
                    "is not".to_string()
                } else {
                    "is".to_string()
                }
            }
            TokenType::Not => {
                self.next();
                self.expect(TokenType::In);
                "not in".to_string()
            }
            _ => String::new(),
        }
    }

    fn parse_bit_or(&mut self) -> Expr {
        let mut x = self.parse_bit_xor();
        while self.tok.ty == TokenType::Pipe {
            let pos = self.tok.pos();
            self.next();
            let y = self.parse_bit_xor();
            x = Expr::BinOp {
                pos,
                op: "|".to_string(),
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_bit_xor(&mut self) -> Expr {
        let mut x = self.parse_bit_and();
        while self.tok.ty == TokenType::Caret {
            let pos = self.tok.pos();
            self.next();
            let y = self.parse_bit_and();
            x = Expr::BinOp {
                pos,
                op: "^".to_string(),
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_bit_and(&mut self) -> Expr {
        let mut x = self.parse_shift();
        while self.tok.ty == TokenType::Amp {
            let pos = self.tok.pos();
            self.next();
            let y = self.parse_shift();
            x = Expr::BinOp {
                pos,
                op: "&".to_string(),
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_shift(&mut self) -> Expr {
        let mut x = self.parse_arith();
        while self.tok.ty == TokenType::Shl || self.tok.ty == TokenType::Shr {
            let pos = self.tok.pos();
            let op = self.tok.lit.clone();
            self.next();
            let y = self.parse_arith();
            x = Expr::BinOp {
                pos,
                op,
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_arith(&mut self) -> Expr {
        let mut x = self.parse_term();
        while self.tok.ty == TokenType::Plus || self.tok.ty == TokenType::Minus {
            let pos = self.tok.pos();
            let op = self.tok.lit.clone();
            self.next();
            let y = self.parse_term();
            x = Expr::BinOp {
                pos,
                op,
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_term(&mut self) -> Expr {
        let mut x = self.parse_factor();
        while matches!(
            self.tok.ty,
            TokenType::Star
                | TokenType::Slash
                | TokenType::FloorDiv
                | TokenType::Percent
        ) {
            let pos = self.tok.pos();
            let op = self.tok.lit.clone();
            self.next();
            let y = self.parse_factor();
            x = Expr::BinOp {
                pos,
                op,
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_factor(&mut self) -> Expr {
        if matches!(
            self.tok.ty,
            TokenType::Plus | TokenType::Minus | TokenType::Tilde
        ) {
            let pos = self.tok.pos();
            let op = self.tok.lit.clone();
            self.next();
            let x = self.parse_factor();
            return Expr::UnaryOp {
                pos,
                op,
                x: Box::new(x),
            };
        }
        self.parse_power()
    }

    fn parse_power(&mut self) -> Expr {
        let x = self.parse_atom_expr();
        if self.tok.ty == TokenType::DoubleStar {
            let pos = self.tok.pos();
            self.next();
            let y = self.parse_factor();
            return Expr::BinOp {
                pos,
                op: "**".to_string(),
                x: Box::new(x),
                y: Box::new(y),
            };
        }
        x
    }

    fn parse_atom_expr(&mut self) -> Expr {
        let mut x = self.parse_atom();
        loop {
            match self.tok.ty {
                TokenType::LParen => {
                    x = self.call_args(x);
                }
                TokenType::LBracket => {
                    x = self.subscript(x);
                }
                TokenType::Dot => {
                    self.next();
                    let name = self.expect(TokenType::Ident);
                    let pos = name.pos();
                    x = Expr::Attr {
                        pos,
                        x: Box::new(x),
                        name: name.lit,
                    };
                }
                _ => return x,
            }
        }
    }

    fn call_args(&mut self, f: Expr) -> Expr {
        let pos = self.tok.pos();
        self.next();
        let mut args = Vec::new();
        let mut kwargs = Vec::new();
        let mut star = None;
        let mut dbl_star = None;
        while self.tok.ty != TokenType::RParen {
            if self.tok.ty == TokenType::Star {
                self.next();
                star = Some(Box::new(self.parse_test()));
            } else if self.tok.ty == TokenType::DoubleStar {
                self.next();
                dbl_star = Some(Box::new(self.parse_test()));
            } else {
                let e = self.parse_test();
                if self.tok.ty == TokenType::Assign {
                    let (kpos, kname) = match &e {
                        Expr::Name { pos, name } => (*pos, name.clone()),
                        _ => {
                            let p = e.pos();
                            self.errorf(p, "关键字参数名必须是标识符", &[]);
                            self.next();
                            continue;
                        }
                    };
                    self.next();
                    kwargs.push(KwArg {
                        pos: kpos,
                        name: kname,
                        value: Box::new(self.parse_test()),
                    });
                } else {
                    args.push(e);
                }
            }
            if self.tok.ty != TokenType::Comma {
                break;
            }
            self.next();
        }
        self.expect(TokenType::RParen);
        Expr::Call {
            pos,
            func: Box::new(f),
            args,
            kwargs,
            star,
            dbl_star,
        }
    }

    fn subscript(&mut self, x: Expr) -> Expr {
        let pos = self.tok.pos();
        self.next();
        let index = self.parse_subscript_index();
        self.expect(TokenType::RBracket);
        Expr::Subscript {
            pos,
            x: Box::new(x),
            index: Box::new(index),
        }
    }

    fn parse_subscript_index(&mut self) -> Expr {
        let pos = self.tok.pos();
        let mut lo: Option<Expr> = None;
        if !matches!(
            self.tok.ty,
            TokenType::Colon | TokenType::RBracket | TokenType::Comma
        ) {
            lo = Some(self.parse_test());
        }
        if self.tok.ty == TokenType::Colon {
            self.next();
            let mut hi = None;
            if !matches!(
                self.tok.ty,
                TokenType::Colon | TokenType::RBracket | TokenType::Comma
            ) {
                hi = Some(self.parse_test());
            }
            let mut step = None;
            if self.tok.ty == TokenType::Colon {
                self.next();
                if !matches!(self.tok.ty, TokenType::RBracket | TokenType::Comma) {
                    step = Some(self.parse_test());
                }
            }
            return Expr::Slice {
                pos,
                lo: lo.map(Box::new),
                hi: hi.map(Box::new),
                step: step.map(Box::new),
            };
        }
        if self.tok.ty == TokenType::Comma {
            let mut elems = Vec::new();
            if let Some(l) = lo {
                elems.push(l);
            }
            while self.tok.ty == TokenType::Comma {
                self.next();
                if self.tok.ty == TokenType::RBracket {
                    break;
                }
                elems.push(self.parse_test());
            }
            return Expr::TupleLit {
                pos,
                elems,
                paren: false,
            };
        }
        lo.unwrap_or_else(|| {
            Expr::Name {
                pos: self.tok.pos(),
                name: String::new(),
            }
        })
    }

    fn parse_atom(&mut self) -> Expr {
        let t = self.tok.clone();
        match t.ty {
            TokenType::Ident => {
                self.next();
                Expr::Name {
                    pos: t.pos(),
                    name: t.lit,
                }
            }
            TokenType::Int => {
                self.next();
                Expr::IntLit {
                    pos: t.pos(),
                    value: t.lit,
                }
            }
            TokenType::Float => {
                self.next();
                Expr::FloatLit {
                    pos: t.pos(),
                    value: t.lit,
                }
            }
            TokenType::String => {
                let mut parts = Vec::new();
                while self.tok.ty == TokenType::String {
                    parts.push(self.tok.lit.clone());
                    self.next();
                }
                Expr::StringLit {
                    pos: t.pos(),
                    value: parts.join(" "),
                }
            }
            TokenType::None => {
                self.next();
                Expr::Name {
                    pos: t.pos(),
                    name: "None".to_string(),
                }
            }
            TokenType::True => {
                self.next();
                Expr::Name {
                    pos: t.pos(),
                    name: "True".to_string(),
                }
            }
            TokenType::False => {
                self.next();
                Expr::Name {
                    pos: t.pos(),
                    name: "False".to_string(),
                }
            }
            TokenType::Ellipsis => {
                self.next();
                Expr::EllipsisLit { pos: t.pos() }
            }
            TokenType::LParen => {
                self.next();
                let e = if self.tok.ty == TokenType::RParen {
                    Expr::TupleLit {
                        pos: t.pos(),
                        elems: Vec::new(),
                        paren: true,
                    }
                } else {
                    match self.parse_test_list() {
                        Expr::TupleLit { pos, elems, .. } => Expr::TupleLit {
                            pos,
                            elems,
                            paren: true,
                        },
                        other => other,
                    }
                };
                self.expect(TokenType::RParen);
                e
            }
            TokenType::LBracket => self.list_atom(t.pos()),
            TokenType::LBrace => self.dict_set_atom(t.pos()),
            _ => {
                let lit = t.lit.clone();
                self.errorf(t.pos(), "期望表达式，实际为 %q", &[lit]);
                Expr::Name {
                    pos: t.pos(),
                    name: String::new(),
                }
            }
        }
    }

    fn list_atom(&mut self, pos: Position) -> Expr {
        self.next();
        if self.tok.ty == TokenType::RBracket {
            self.next();
            return Expr::ListLit {
                pos,
                elems: Vec::new(),
            };
        }
        let first = self.parse_test();
        if self.tok.ty == TokenType::For {
            let mut clauses = Vec::new();
            while self.tok.ty == TokenType::For {
                self.next();
                let target = self.parse_expr_list();
                self.expect(TokenType::In);
                let iter = self.parse_or_test();
                let mut ifs = Vec::new();
                while self.tok.ty == TokenType::If {
                    self.next();
                    ifs.push(self.parse_or_test());
                }
                clauses.push(CompClause {
                    target: Box::new(target),
                    iter: Box::new(iter),
                    ifs,
                });
            }
            self.expect(TokenType::RBracket);
            return Expr::ListComp {
                pos,
                elem: Box::new(first),
                clauses,
            };
        }
        let mut elems = vec![first];
        while self.tok.ty == TokenType::Comma {
            self.next();
            if self.tok.ty == TokenType::RBracket {
                break;
            }
            elems.push(self.parse_test());
        }
        self.expect(TokenType::RBracket);
        Expr::ListLit { pos, elems }
    }

    fn dict_set_atom(&mut self, pos: Position) -> Expr {
        self.next();
        if self.tok.ty == TokenType::RBrace {
            self.next();
            return Expr::DictLit {
                pos,
                keys: Vec::new(),
                vals: Vec::new(),
            };
        }
        if self.tok.ty == TokenType::DoubleStar {
            self.errorf(self.tok.pos(), "字典解包 {} 暂不支持", &[]);
            self.next();
            self.parse_test();
            self.expect(TokenType::RBrace);
            return Expr::DictLit {
                pos,
                keys: Vec::new(),
                vals: Vec::new(),
            };
        }
        let first = self.parse_test();
        if self.tok.ty == TokenType::For {
            self.errorf(self.tok.pos(), "字典/集合推导式暂不支持", &[]);
            while self.tok.ty != TokenType::RBrace && self.tok.ty != TokenType::Eof {
                self.next();
            }
            self.expect(TokenType::RBrace);
            return Expr::SetLit {
                pos,
                elems: vec![first],
            };
        }
        if self.tok.ty == TokenType::Colon {
            self.next();
            let mut keys = vec![first];
            let mut vals = vec![self.parse_test()];
            while self.tok.ty == TokenType::Comma {
                self.next();
                if self.tok.ty == TokenType::RBrace {
                    break;
                }
                keys.push(self.parse_test());
                self.expect(TokenType::Colon);
                vals.push(self.parse_test());
            }
            self.expect(TokenType::RBrace);
            return Expr::DictLit { pos, keys, vals };
        }
        let mut elems = vec![first];
        while self.tok.ty == TokenType::Comma {
            self.next();
            if self.tok.ty == TokenType::RBrace {
                break;
            }
            elems.push(self.parse_test());
        }
        self.expect(TokenType::RBrace);
        Expr::SetLit { pos, elems }
    }
}

fn unquote_str(s: &str) -> String {
    if s.len() >= 2 && (s.as_bytes()[0] == b'"' || s.as_bytes()[0] == b'\'')
        && s.as_bytes()[s.len() - 1] == s.as_bytes()[0]
    {
        return s[1..s.len() - 1].to_string();
    }
    s.to_string()
}

fn parse_time_spec(v: &str) -> Option<f64> {
    let mut i = 0;
    let bytes = v.as_bytes();
    while i < bytes.len() && (bytes[i].is_ascii_digit() || bytes[i] == b'.') {
        i += 1;
    }
    if i == 0 {
        return None;
    }
    let num: f64 = v[..i].parse().ok()?;
    let mult: f64 = match &v[i..] {
        "" | "s" => 1.0,
        "ms" => 0.001,
        "m" => 60.0,
        "h" => 3600.0,
        _ => return None,
    };
    let secs = num * mult;
    if secs <= 0.0 {
        return None;
    }
    Some(secs)
}

fn parse_mem_spec(v: &str) -> Option<i64> {
    let mut i = 0;
    let bytes = v.as_bytes();
    while i < bytes.len() && bytes[i].is_ascii_digit() {
        i += 1;
    }
    if i == 0 {
        return None;
    }
    let num: i64 = v[..i].parse().ok()?;
    if num <= 0 {
        return None;
    }
    let mult: i64 = match &v[i..] {
        "" | "B" => 1,
        "KB" | "KiB" => 1 << 10,
        "MB" | "MiB" => 1 << 20,
        "GB" | "GiB" => 1 << 30,
        _ => return None,
    };
    Some(num * mult)
}

pub fn parse(src: &str) -> (Option<Module>, Option<Diagnostic>) {
    let mut p = new(src);
    let m = p.parse_module();
    (m, p.err())
}

#[cfg(test)]
mod tests {
    use super::*;

    fn parse_ok(src: &str) -> Module {
        let (m, err) = parse(src);
        assert!(err.is_none(), "不应报错: {:?}", err);
        m.expect("应返回模块")
    }

    #[test]
    fn simple_assign() {
        let m = parse_ok("x = 1\n");
        assert_eq!(m.stmts.len(), 1);
        match &m.stmts[0] {
            Stmt::Assign { op, right, .. } => {
                assert_eq!(op, "=");
                assert!(matches!(&**right, Expr::IntLit { value, .. } if value == "1"));
            }
            other => panic!("期望 Assign，得到 {:?}", other),
        }
    }

    #[test]
    fn if_elif_else() {
        let m = parse_ok("if a:\n    pass\nelif b:\n    pass\nelse:\n    pass\n");
        assert_eq!(m.stmts.len(), 1);
        match &m.stmts[0] {
            Stmt::If { elifs, els, .. } => {
                assert_eq!(elifs.len(), 1);
                assert_eq!(els.len(), 1);
            }
            other => panic!("期望 If，得到 {:?}", other),
        }
    }

    #[test]
    fn func_def_params() {
        let m = parse_ok("def f(a, b: int = 1, *args, **kw):\n    return a\n");
        match &m.stmts[0] {
            Stmt::FuncDef { params, .. } => {
                assert_eq!(params.len(), 4);
                assert!(params[1].anno.is_some());
                assert!(params[1].default.is_some());
                assert!(params[2].star);
                assert!(params[3].dbl_star);
            }
            other => panic!("期望 FuncDef，得到 {:?}", other),
        }
    }

    #[test]
    fn call_kwargs_and_star() {
        let m = parse_ok("f(a, x=1, *xs, **kw)\n");
        match &m.stmts[0] {
            Stmt::ExprStmt { x, .. } => match &**x {
                Expr::Call {
                    args, kwargs, star, dbl_star, ..
                } => {
                    assert_eq!(args.len(), 1);
                    assert_eq!(kwargs.len(), 1);
                    assert_eq!(kwargs[0].name, "x");
                    assert!(star.is_some());
                    assert!(dbl_star.is_some());
                }
                other => panic!("期望 Call，得到 {:?}", other),
            },
            other => panic!("期望 ExprStmt，得到 {:?}", other),
        }
    }

    #[test]
    fn subscript_slice_and_tuple() {
        let m = parse_ok("a[1:2], b[1, 2], c[:]\n");
        assert_eq!(m.stmts.len(), 1);
    }

    #[test]
    fn list_comp() {
        let m = parse_ok("xs = [x for x in range(10) if x > 2]\n");
        match &m.stmts[0] {
            Stmt::Assign { right, .. } => match &**right {
                Expr::ListComp { clauses, .. } => {
                    assert_eq!(clauses.len(), 1);
                    assert_eq!(clauses[0].ifs.len(), 1);
                }
                other => panic!("期望 ListComp，得到 {:?}", other),
            },
            other => panic!("期望 Assign，得到 {:?}", other),
        }
    }

    #[test]
    fn error_expected_expr() {
        let (_m, err) = parse("x = (1 +\n");
        assert!(err.is_some());
        let d = err.unwrap();
        assert_eq!(d.code, "E0002");
    }

    #[test]
    fn trace_and_cage() {
        let m = parse_ok("trace(level=\"INFO\", args=False):\n    pass\n");
        assert!(matches!(&m.stmts[0], Stmt::Trace { .. }));
        let m = parse_ok("cage(max_time=\"5s\", max_memory=\"100MB\"):\n    pass\n");
        match &m.stmts[0] {
            Stmt::Cage {
                has_time,
                max_time,
                has_mem,
                max_memory,
                ..
            } => {
                assert!(*has_time);
                assert!(*has_mem);
                assert!((*max_time - 5.0).abs() < 1e-9);
                assert_eq!(*max_memory, 100 << 20);
            }
            other => panic!("期望 Cage，得到 {:?}", other),
        }
    }

    #[test]
    fn time_mem_specs() {
        assert_eq!(parse_time_spec("500ms"), Some(0.5));
        assert_eq!(parse_time_spec("2m"), Some(120.0));
        assert_eq!(parse_time_spec("1h"), Some(3600.0));
        assert_eq!(parse_mem_spec("64KB"), Some(64 << 10));
        assert_eq!(parse_mem_spec("2GB"), Some(2 << 30));
        assert!(parse_time_spec("abc").is_none());
        assert!(parse_mem_spec("xyz").is_none());
    }

    #[test]
    fn fly_keywords() {
        let m = parse_ok("lock x\nsafe a, b\nmask c\nguard x: int\nonly (math):\n    pass\n");
        assert_eq!(m.stmts.len(), 5);
        assert!(matches!(&m.stmts[0], Stmt::Lock { .. }));
        assert!(matches!(&m.stmts[1], Stmt::Safe { .. }));
        assert!(matches!(&m.stmts[2], Stmt::Mask { .. }));
        assert!(matches!(&m.stmts[3], Stmt::Guard { .. }));
        assert!(matches!(&m.stmts[4], Stmt::Only { .. }));
    }

    #[test]
    fn unsupported_keywords() {
        let (_m, err) = parse("assert True\n");
        assert!(err.is_some());
        let d = err.unwrap();
        assert_eq!(d.code, "E0004");
        assert!(d.msg.contains("assert"));
    }
}
