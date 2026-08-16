// lib.rs：Rust CLI 库入口——模块声明（lexer/ast/parser/checkd/format/errorcode/errorinfo）。
pub mod ast;
pub mod checkd;
pub mod diagnostic;
pub mod errorcode;
pub mod errorinfo;
pub mod format;
pub mod lexer;
pub mod parser;
pub mod gen;
pub mod typeinfer;
pub mod version;
