// ast.rs：Rust 版 AST 节点定义（与 Go 版 internal/ast 对应，仅 check 管线使用）。
use crate::diagnostic::Position;

#[derive(Debug, Clone, PartialEq)]
pub struct Module {
    pub pos: Position,
    pub stmts: Vec<Stmt>,
}

impl Module {
    pub fn new(pos: Position, stmts: Vec<Stmt>) -> Self {
        Module { pos, stmts }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct ImportItem {
    pub name: String,
    pub alias: String,
}

impl ImportItem {
    // Module 返回导入的顶层模块名（import os.path → "os"）。
    pub fn module(&self) -> &str {
        match self.name.find('.') {
            Some(i) => &self.name[..i],
            None => &self.name,
        }
    }

    // TopName 返回绑定名（import os.path → "os"；import os.path as p → "p"）。
    pub fn top_name(&self) -> &str {
        let n = if self.alias.is_empty() {
            &self.name
        } else {
            &self.alias
        };
        match n.find('.') {
            Some(i) => &n[..i],
            None => n,
        }
    }
}

#[derive(Debug, Clone, PartialEq)]
pub struct Param {
    pub name: String,
    pub anno: Option<Box<Expr>>,
    pub default: Option<Box<Expr>>,
    pub star: bool,
    pub dbl_star: bool,
}

#[derive(Debug, Clone, PartialEq)]
pub struct ElifClause {
    pub pos: Position,
    pub cond: Box<Expr>,
    pub body: Vec<Stmt>,
}

#[derive(Debug, Clone, PartialEq)]
pub struct ExceptClause {
    pub pos: Position,
    pub ty: Option<Box<Expr>>,
    pub name: String,
    pub body: Vec<Stmt>,
}

#[derive(Debug, Clone, PartialEq)]
pub struct KwArg {
    pub pos: Position,
    pub name: String,
    pub value: Box<Expr>,
}

#[derive(Debug, Clone, PartialEq)]
pub struct CompClause {
    pub target: Box<Expr>,
    pub iter: Box<Expr>,
    pub ifs: Vec<Expr>,
}

#[derive(Debug, Clone, PartialEq)]
pub enum Stmt {
    Import {
        pos: Position,
        items: Vec<ImportItem>,
    },
    FromImport {
        pos: Position,
        module: String,
        items: Vec<ImportItem>,
    },
    Lock {
        pos: Position,
        name: String,
        value: Option<Box<Expr>>,
    },
    Safe {
        pos: Position,
        names: Vec<String>,
    },
    Mask {
        pos: Position,
        names: Vec<String>,
    },
    Guard {
        pos: Position,
        name: String,
        ty: Option<Box<Expr>>,
        conds: Vec<Expr>,
    },
    Assign {
        pos: Position,
        left: Vec<Expr>,
        op: String,
        right: Box<Expr>,
    },
    ExprStmt {
        pos: Position,
        x: Box<Expr>,
    },
    FuncDef {
        pos: Position,
        name: String,
        params: Vec<Param>,
        return_type: Option<Box<Expr>>,
        body: Vec<Stmt>,
        decorators: Vec<Expr>,
    },
    ClassDef {
        pos: Position,
        name: String,
        bases: Vec<Expr>,
        body: Vec<Stmt>,
        decorators: Vec<Expr>,
        seal: bool,
    },
    Only {
        pos: Position,
        modules: Vec<String>,
        body: Vec<Stmt>,
    },
    Trace {
        pos: Position,
        level: String,
        args: bool,
        ret: bool,
        body: Vec<Stmt>,
    },
    Cage {
        pos: Position,
        has_time: bool,
        max_time: f64,
        has_mem: bool,
        max_memory: i64,
        body: Vec<Stmt>,
    },
    If {
        pos: Position,
        cond: Box<Expr>,
        then: Vec<Stmt>,
        elifs: Vec<ElifClause>,
        els: Vec<Stmt>,
    },
    For {
        pos: Position,
        target: Box<Expr>,
        iter: Box<Expr>,
        body: Vec<Stmt>,
        els: Vec<Stmt>,
    },
    While {
        pos: Position,
        cond: Box<Expr>,
        body: Vec<Stmt>,
        els: Vec<Stmt>,
    },
    Return {
        pos: Position,
        value: Option<Box<Expr>>,
    },
    Raise {
        pos: Position,
        exc: Option<Box<Expr>>,
        from: Option<Box<Expr>>,
    },
    Try {
        pos: Position,
        body: Vec<Stmt>,
        handlers: Vec<ExceptClause>,
        els: Vec<Stmt>,
        finally: Vec<Stmt>,
    },
    Pass {
        pos: Position,
    },
    Break {
        pos: Position,
    },
    Continue {
        pos: Position,
    },
    Delete {
        pos: Position,
        targets: Vec<Expr>,
    },
}

#[derive(Debug, Clone, PartialEq)]
pub enum Expr {
    Name {
        pos: Position,
        name: String,
    },
    IntLit {
        pos: Position,
        value: String,
    },
    FloatLit {
        pos: Position,
        value: String,
    },
    StringLit {
        pos: Position,
        value: String,
    },
    EllipsisLit {
        pos: Position,
    },
    ListLit {
        pos: Position,
        elems: Vec<Expr>,
    },
    TupleLit {
        pos: Position,
        elems: Vec<Expr>,
        paren: bool,
    },
    DictLit {
        pos: Position,
        keys: Vec<Expr>,
        vals: Vec<Expr>,
    },
    SetLit {
        pos: Position,
        elems: Vec<Expr>,
    },
    Call {
        pos: Position,
        func: Box<Expr>,
        args: Vec<Expr>,
        kwargs: Vec<KwArg>,
        star: Option<Box<Expr>>,
        dbl_star: Option<Box<Expr>>,
    },
    Attr {
        pos: Position,
        x: Box<Expr>,
        name: String,
    },
    Slice {
        pos: Position,
        lo: Option<Box<Expr>>,
        hi: Option<Box<Expr>>,
        step: Option<Box<Expr>>,
    },
    Subscript {
        pos: Position,
        x: Box<Expr>,
        index: Box<Expr>,
    },
    BinOp {
        pos: Position,
        op: String,
        x: Box<Expr>,
        y: Box<Expr>,
    },
    UnaryOp {
        pos: Position,
        op: String,
        x: Box<Expr>,
    },
    BoolOp {
        pos: Position,
        op: String,
        x: Box<Expr>,
        y: Box<Expr>,
    },
    Compare {
        pos: Position,
        x: Box<Expr>,
        ops: Vec<String>,
        ys: Vec<Expr>,
    },
    Cond {
        pos: Position,
        cond: Box<Expr>,
        then: Box<Expr>,
        els: Box<Expr>,
    },
    ListComp {
        pos: Position,
        elem: Box<Expr>,
        clauses: Vec<CompClause>,
    },
}

impl Expr {
    pub fn pos(&self) -> Position {
        match self {
            Expr::Name { pos, .. }
            | Expr::IntLit { pos, .. }
            | Expr::FloatLit { pos, .. }
            | Expr::StringLit { pos, .. }
            | Expr::EllipsisLit { pos }
            | Expr::ListLit { pos, .. }
            | Expr::TupleLit { pos, .. }
            | Expr::DictLit { pos, .. }
            | Expr::SetLit { pos, .. }
            | Expr::Call { pos, .. }
            | Expr::Attr { pos, .. }
            | Expr::Slice { pos, .. }
            | Expr::Subscript { pos, .. }
            | Expr::BinOp { pos, .. }
            | Expr::UnaryOp { pos, .. }
            | Expr::BoolOp { pos, .. }
            | Expr::Compare { pos, .. }
            | Expr::Cond { pos, .. }
            | Expr::ListComp { pos, .. } => *pos,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn import_item_module() {
        let it = ImportItem {
            name: "os.path".into(),
            alias: String::new(),
        };
        assert_eq!(it.module(), "os");
        assert_eq!(it.top_name(), "os");
        let it = ImportItem {
            name: "os.path".into(),
            alias: "p".into(),
        };
        assert_eq!(it.top_name(), "p");
        let it = ImportItem {
            name: "os".into(),
            alias: String::new(),
        };
        assert_eq!(it.module(), "os");
        assert_eq!(it.top_name(), "os");
    }
}
