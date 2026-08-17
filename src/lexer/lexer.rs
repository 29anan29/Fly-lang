// lexer.rs：Rust 版词法分析主实现（与 Go 版 internal/lexer 行为一致，CLI 对照验收）。
use crate::diagnostic::{error_code, Diagnostic, Position};

use super::token::{keyword, Token, TokenType};

pub struct Lexer {
    src: Vec<u8>,
    off: usize,
    line: u32,
    col: u32,
    depth: i32,
    indents: Vec<usize>,
    at_line_start: bool,
    line_tok: bool,
    pending: Vec<Token>,
    err: Option<Diagnostic>,
}

impl Lexer {
    pub fn new(src: &str) -> Self {
        Lexer {
            src: src.as_bytes().to_vec(),
            off: 0,
            line: 1,
            col: 1,
            depth: 0,
            indents: vec![0],
            at_line_start: true,
            line_tok: false,
            pending: Vec::new(),
            err: None,
        }
    }

    pub fn err(&self) -> Option<&Diagnostic> {
        self.err.as_ref()
    }

    fn pos(&self) -> Position {
        Position {
            line: self.line,
            col: self.col,
        }
    }

    fn errorf(&mut self, p: Position, format: &str, args: &[String]) {
        if self.err.is_none() {
            let msg = format_msg(format, args);
            self.err = Some(Diagnostic {
                pos: p,
                code: error_code(format).to_string(),
                msg,
            });
        }
    }

    fn advance(&mut self) -> u8 {
        let c = self.src[self.off];
        self.off += 1;
        if c == b'\n' {
            self.line += 1;
            self.col = 1;
        } else {
            self.col += 1;
        }
        c
    }

    fn peek(&self, off: usize) -> u8 {
        if self.off + off < self.src.len() {
            self.src[self.off + off]
        } else {
            0
        }
    }

    fn skip_line(&mut self) {
        while self.off < self.src.len() && self.src[self.off] != b'\n' {
            self.advance();
        }
    }

    pub fn next(&mut self) -> Token {
        if self.err.is_some() {
            return Token {
                ty: TokenType::Eof,
                lit: String::new(),
                line: self.line,
                col: self.col,
            };
        }
        if !self.pending.is_empty() {
            return self.pending.remove(0);
        }
        loop {
            if self.at_line_start {
                if let Some(t) = self.line_start() {
                    return t;
                }
            }
            if self.off >= self.src.len() {
                if self.line_tok {
                    self.line_tok = false;
                    return Token {
                        ty: TokenType::Newline,
                        lit: String::new(),
                        line: self.line,
                        col: self.col,
                    };
                }
                if self.depth > 0 {
                    self.errorf(self.pos(), "未闭合的括号", &[]);
                }
                while self.indents.len() > 1 {
                    self.indents.pop();
                    self.push_pending(TokenType::Dedent);
                }
                if !self.pending.is_empty() {
                    return self.pending.remove(0);
                }
                return Token {
                    ty: TokenType::Eof,
                    lit: String::new(),
                    line: self.line,
                    col: self.col,
                };
            }
            let c = self.src[self.off];
            match c {
                b' ' | b'\r' | b'\t' => {
                    self.advance();
                }
                b'\n' => {
                    if self.depth > 0 {
                        self.advance();
                    } else {
                        let pos = self.pos();
                        self.advance();
                        self.at_line_start = true;
                        if self.line_tok {
                            self.line_tok = false;
                            return Token {
                                ty: TokenType::Newline,
                                lit: String::new(),
                                line: pos.line,
                                col: pos.col,
                            };
                        }
                    }
                }
                b'#' => self.skip_line(),
                b'\\' if self.peek(1) == b'\n' => {
                    self.advance();
                    self.advance();
                }
                _ => {
                    let t = self.lex_token();
                    if t.ty != TokenType::Illegal {
                        self.line_tok = true;
                    }
                    return t;
                }
            }
        }
    }

    fn push_pending(&mut self, ty: TokenType) {
        let p = self.pos();
        self.pending.push(Token {
            ty,
            lit: String::new(),
            line: p.line,
            col: p.col,
        });
    }

    fn line_start(&mut self) -> Option<Token> {
        loop {
            let mut n = 0;
            while self.off < self.src.len() {
                let c = self.src[self.off];
                if c == b' ' {
                    n += 1;
                    self.advance();
                    continue;
                }
                if c == b'\t' {
                    self.errorf(self.pos(), "行首不支持制表符缩进", &[]);
                    self.at_line_start = false;
                    return Some(Token {
                        ty: TokenType::Illegal,
                        lit: String::new(),
                        line: self.line,
                        col: self.col,
                    });
                }
                break;
            }
            if self.off >= self.src.len() {
                self.at_line_start = false;
                while self.indents.len() > 1 {
                    self.indents.pop();
                    self.push_pending(TokenType::Dedent);
                }
                if !self.pending.is_empty() {
                    return Some(self.pending.remove(0));
                }
                return Some(Token {
                    ty: TokenType::Eof,
                    lit: String::new(),
                    line: self.line,
                    col: self.col,
                });
            }
            let c = self.src[self.off];
            match c {
                b'\n' => {
                    self.advance();
                }
                b'#' => self.skip_line(),
                b'\\' if self.peek(1) == b'\n' => {
                    self.advance();
                    self.advance();
                }
                _ => {
                    let top = *self.indents.last().unwrap();
                    if n > top {
                        self.indents.push(n);
                        self.push_pending(TokenType::Indent);
                    } else if n < top {
                        while !self.indents.is_empty() && *self.indents.last().unwrap() > n {
                            self.indents.pop();
                            self.push_pending(TokenType::Dedent);
                        }
                        if *self.indents.last().unwrap() != n {
                            self.errorf(self.pos(), "缩进级别与上层不一致", &[]);
                            self.at_line_start = false;
                            return Some(Token {
                                ty: TokenType::Illegal,
                                lit: String::new(),
                                line: self.line,
                                col: self.col,
                            });
                        }
                    }
                    self.at_line_start = false;
                    if !self.pending.is_empty() {
                        return Some(self.pending.remove(0));
                    }
                    return None;
                }
            }
        }
    }

    fn lex_token(&mut self) -> Token {
        let c = self.src[self.off];
        if (c == b'f'
            || c == b'F'
            || c == b'r'
            || c == b'R'
            || c == b'b'
            || c == b'B'
            || c == b'u'
            || c == b'U')
            && self.is_string_prefix()
        {
            return self.scan_string_prefix();
        }
        if is_ident_start(c) {
            let start = self.pos();
            let off = self.off;
            while self.off < self.src.len() && is_ident_part(self.src[self.off]) {
                self.advance();
            }
            let name: String = String::from_utf8_lossy(&self.src[off..self.off]).to_string();
            if let Some(tt) = keyword(&name) {
                return Token {
                    ty: tt,
                    lit: name,
                    line: start.line,
                    col: start.col,
                };
            }
            return Token {
                ty: TokenType::Ident,
                lit: name,
                line: start.line,
                col: start.col,
            };
        }
        if c >= b'0' && c <= b'9' {
            return self.scan_number();
        }
        if c == b'\'' || c == b'"' {
            return self.scan_string("");
        }
        if c == b'.' && self.peek(1) == b'.' && self.peek(2) == b'.' {
            let start = self.pos();
            self.advance();
            self.advance();
            self.advance();
            return Token {
                ty: TokenType::Ellipsis,
                lit: "...".to_string(),
                line: start.line,
                col: start.col,
            };
        }
        if c == b'.' && self.peek(1) >= b'0' && self.peek(1) <= b'9' {
            return self.scan_number();
        }
        self.scan_op()
    }

    fn is_string_prefix(&self) -> bool {
        let mut i = self.off;
        let mut seen = [false; 4];
        let idx = |ch: u8| -> Option<usize> {
            match ch {
                b'f' => Some(0),
                b'r' => Some(1),
                b'b' => Some(2),
                b'u' => Some(3),
                _ => None,
            }
        };
        while i < self.src.len() {
            let ch = self.src[i];
            if ch == b'\'' || ch == b'"' {
                return true;
            }
            if i - self.off >= 2 {
                return false;
            }
            let low = if ch >= b'A' && ch <= b'Z' {
                ch + 32
            } else {
                ch
            };
            let j = match idx(low) {
                Some(j) => j,
                None => return false,
            };
            if seen[j] {
                return false;
            }
            seen[j] = true;
            i += 1;
        }
        false
    }

    fn scan_string_prefix(&mut self) -> Token {
        let mut prefix = String::new();
        while self.off < self.src.len() && is_ident_part(self.src[self.off]) {
            prefix.push(self.advance() as char);
        }
        self.scan_string(&prefix.to_lowercase())
    }

    fn scan_string(&mut self, prefix: &str) -> Token {
        let start = self.pos();
        let q = self.advance();
        let triple = self.peek(0) == q && self.peek(1) == q;
        if triple {
            self.advance();
            self.advance();
        }
        let begin = self.off;
        while self.off < self.src.len() {
            let c = self.src[self.off];
            if c == b'\\' {
                self.advance();
                if self.off < self.src.len() {
                    self.advance();
                }
                continue;
            }
            if c == q {
                if triple {
                    if self.peek(1) == q && self.peek(2) == q {
                        let lit = String::from_utf8_lossy(&self.src[begin..self.off]).to_string();
                        self.advance();
                        self.advance();
                        self.advance();
                        let qq = q as char;
                        return Token {
                            ty: TokenType::String,
                            lit: format!("{}{}{}{}{}{}{}{}", prefix, qq, qq, qq, lit, qq, qq, qq),
                            line: start.line,
                            col: start.col,
                        };
                    }
                    self.advance();
                    continue;
                }
                let lit = String::from_utf8_lossy(&self.src[begin..self.off]).to_string();
                self.advance();
                return Token {
                    ty: TokenType::String,
                    lit: format!("{}{}{}{}", prefix, q as char, lit, q as char),
                    line: start.line,
                    col: start.col,
                };
            }
            if c == b'\n' && !triple {
                self.errorf(start, "字符串未闭合", &[]);
                return Token {
                    ty: TokenType::Illegal,
                    lit: String::new(),
                    line: start.line,
                    col: start.col,
                };
            }
            self.advance();
        }
        self.errorf(start, "字符串未闭合", &[]);
        Token {
            ty: TokenType::Illegal,
            lit: String::new(),
            line: start.line,
            col: start.col,
        }
    }

    fn scan_number(&mut self) -> Token {
        let start = self.pos();
        let off = self.off;
        let mut is_float = false;
        let p0 = self.peek(0);
        let p1 = self.peek(1);
        if p0 == b'0'
            && (p1 == b'x' || p1 == b'X' || p1 == b'o' || p1 == b'O' || p1 == b'b' || p1 == b'B')
        {
            self.advance();
            self.advance();
            while self.off < self.src.len() {
                let c = self.src[self.off];
                if (c >= b'0' && c <= b'9')
                    || (c >= b'a' && c <= b'f')
                    || (c >= b'A' && c <= b'F')
                    || c == b'_'
                {
                    self.advance();
                    continue;
                }
                break;
            }
        } else {
            while self.off < self.src.len() {
                let c = self.src[self.off];
                if (c >= b'0' && c <= b'9') || c == b'_' {
                    self.advance();
                    continue;
                }
                break;
            }
            if self.peek(0) == b'.' {
                is_float = true;
                self.advance();
                while self.off < self.src.len() {
                    let c = self.src[self.off];
                    if (c >= b'0' && c <= b'9') || c == b'_' {
                        self.advance();
                        continue;
                    }
                    break;
                }
            }
            let pe = self.peek(0);
            if pe == b'e' || pe == b'E' {
                let seen = self.advance();
                is_float = true;
                if self.peek(0) == b'+' || self.peek(0) == b'-' {
                    self.advance();
                }
                let mut digits = 0;
                while self.off < self.src.len() {
                    let c = self.src[self.off];
                    if (c >= b'0' && c <= b'9') || c == b'_' {
                        self.advance();
                        digits += 1;
                        continue;
                    }
                    break;
                }
                if digits == 0 {
                    self.errorf(
                        start,
                        "指数部分缺少数字: %s",
                        &[format!("{}", seen as char)],
                    );
                    return Token {
                        ty: TokenType::Illegal,
                        lit: String::new(),
                        line: start.line,
                        col: start.col,
                    };
                }
            }
        }
        let lit = String::from_utf8_lossy(&self.src[off..self.off]).to_string();
        if self.off < self.src.len() && is_ident_part(self.src[self.off]) {
            self.errorf(start, "非法数字字面量 %s", &[lit.clone()]);
            return Token {
                ty: TokenType::Illegal,
                lit: String::new(),
                line: start.line,
                col: start.col,
            };
        }
        if is_float {
            return Token {
                ty: TokenType::Float,
                lit,
                line: start.line,
                col: start.col,
            };
        }
        Token {
            ty: TokenType::Int,
            lit,
            line: start.line,
            col: start.col,
        }
    }

    fn scan_op(&mut self) -> Token {
        let start = self.pos();
        let advance = |l: &mut Lexer, n: usize| -> String {
            let s = String::from_utf8_lossy(&l.src[l.off..l.off + n]).to_string();
            for _ in 0..n {
                l.advance();
            }
            s
        };
        let two = |ty: TokenType, lit: String| Token {
            ty,
            lit,
            line: start.line,
            col: start.col,
        };
        let c = self.src[self.off];
        match c {
            b'+' => {
                if self.peek(1) == b'=' {
                    two(TokenType::PlusAssign, advance(self, 2))
                } else {
                    two(TokenType::Plus, advance(self, 1))
                }
            }
            b'-' => {
                if self.peek(1) == b'=' {
                    two(TokenType::MinusAssign, advance(self, 2))
                } else if self.peek(1) == b'>' {
                    two(TokenType::Arrow, advance(self, 2))
                } else {
                    two(TokenType::Minus, advance(self, 1))
                }
            }
            b'*' => {
                if self.peek(1) == b'*' && self.peek(2) == b'=' {
                    two(TokenType::DoubleStarAssign, advance(self, 3))
                } else if self.peek(1) == b'*' {
                    two(TokenType::DoubleStar, advance(self, 2))
                } else if self.peek(1) == b'=' {
                    two(TokenType::StarAssign, advance(self, 2))
                } else {
                    two(TokenType::Star, advance(self, 1))
                }
            }
            b'/' => {
                if self.peek(1) == b'/' && self.peek(2) == b'=' {
                    two(TokenType::FloorDivAssign, advance(self, 3))
                } else if self.peek(1) == b'/' {
                    two(TokenType::FloorDiv, advance(self, 2))
                } else if self.peek(1) == b'=' {
                    two(TokenType::SlashAssign, advance(self, 2))
                } else {
                    two(TokenType::Slash, advance(self, 1))
                }
            }
            b'%' => {
                if self.peek(1) == b'=' {
                    two(TokenType::PercentAssign, advance(self, 2))
                } else {
                    two(TokenType::Percent, advance(self, 1))
                }
            }
            b'<' => {
                if self.peek(1) == b'<' && self.peek(2) == b'=' {
                    two(TokenType::ShlAssign, advance(self, 3))
                } else if self.peek(1) == b'<' {
                    two(TokenType::Shl, advance(self, 2))
                } else if self.peek(1) == b'=' {
                    two(TokenType::Le, advance(self, 2))
                } else {
                    two(TokenType::Lt, advance(self, 1))
                }
            }
            b'>' => {
                if self.peek(1) == b'>' && self.peek(2) == b'=' {
                    two(TokenType::ShrAssign, advance(self, 3))
                } else if self.peek(1) == b'>' {
                    two(TokenType::Shr, advance(self, 2))
                } else if self.peek(1) == b'=' {
                    two(TokenType::Ge, advance(self, 2))
                } else {
                    two(TokenType::Gt, advance(self, 1))
                }
            }
            b'!' => {
                if self.peek(1) == b'=' {
                    two(TokenType::Ne, advance(self, 2))
                } else {
                    self.errorf(start, "意外的字符 %q", &[format!("!")]);
                    Token {
                        ty: TokenType::Illegal,
                        lit: String::new(),
                        line: start.line,
                        col: start.col,
                    }
                }
            }
            b'&' => {
                if self.peek(1) == b'=' {
                    two(TokenType::AmpAssign, advance(self, 2))
                } else {
                    two(TokenType::Amp, advance(self, 1))
                }
            }
            b'|' => {
                if self.peek(1) == b'=' {
                    two(TokenType::PipeAssign, advance(self, 2))
                } else {
                    two(TokenType::Pipe, advance(self, 1))
                }
            }
            b'^' => {
                if self.peek(1) == b'=' {
                    two(TokenType::CaretAssign, advance(self, 2))
                } else {
                    two(TokenType::Caret, advance(self, 1))
                }
            }
            b'~' => two(TokenType::Tilde, advance(self, 1)),
            b'=' => {
                if self.peek(1) == b'=' {
                    two(TokenType::EqEq, advance(self, 2))
                } else {
                    two(TokenType::Assign, advance(self, 1))
                }
            }
            b':' => two(TokenType::Colon, advance(self, 1)),
            b',' => two(TokenType::Comma, advance(self, 1)),
            b'.' => two(TokenType::Dot, advance(self, 1)),
            b';' => two(TokenType::Semicolon, advance(self, 1)),
            b'@' => two(TokenType::At, advance(self, 1)),
            b'(' => {
                self.depth += 1;
                two(TokenType::LParen, advance(self, 1))
            }
            b')' => {
                self.depth -= 1;
                if self.depth < 0 {
                    self.errorf(start, "多余的右括号 ')'", &[]);
                }
                two(TokenType::RParen, advance(self, 1))
            }
            b'[' => {
                self.depth += 1;
                two(TokenType::LBracket, advance(self, 1))
            }
            b']' => {
                self.depth -= 1;
                if self.depth < 0 {
                    self.errorf(start, "多余的右括号 ']'", &[]);
                }
                two(TokenType::RBracket, advance(self, 1))
            }
            b'{' => {
                self.depth += 1;
                two(TokenType::LBrace, advance(self, 1))
            }
            b'}' => {
                self.depth -= 1;
                if self.depth < 0 {
                    self.errorf(start, "多余的右括号 '}'", &[]);
                }
                two(TokenType::RBrace, advance(self, 1))
            }
            _ => {
                self.errorf(start, "意外的字符 %q", &[format!("{}", c as char)]);
                Token {
                    ty: TokenType::Illegal,
                    lit: String::new(),
                    line: start.line,
                    col: start.col,
                }
            }
        }
    }
}

fn is_ident_start(c: u8) -> bool {
    c == b'_' || (c >= b'a' && c <= b'z') || (c >= b'A' && c <= b'Z')
}

fn is_ident_part(c: u8) -> bool {
    is_ident_start(c) || (c >= b'0' && c <= b'9')
}

pub(crate) fn format_msg(format: &str, args: &[String]) -> String {
    if args.is_empty() {
        return format.to_string();
    }
    let mut out = String::new();
    let mut rest = format;
    let mut i = 0;
    while i < args.len() {
        if let Some(p) = rest.find("%s") {
            out.push_str(&rest[..p]);
            out.push_str(&args[i]);
            rest = &rest[p + 2..];
        } else if let Some(p) = rest.find("%q") {
            out.push_str(&rest[..p]);
            out.push_str(&format!("'{}'", args[i]));
            rest = &rest[p + 2..];
        } else {
            break;
        }
        i += 1;
    }
    out.push_str(rest);
    out
}
