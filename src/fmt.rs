// fmt.rs：fly fmt 的代码格式化器——token 流级空白重排（与 Go 版 internal/format 行为一致）。
// 不改变 token 序列（语义安全），保留注释与三引号字符串原文；
// 调用方须先确保 check 通过（语法错误文件拒绝格式化）。
use crate::lexer::lexer::Lexer;
use crate::lexer::token::{Token, TokenType};

struct Line {
    num: usize,
    text: String,
    tokens: Vec<Token>,
    raw: bool,
    comment: String,
    blank: bool,
}

// Format 返回格式化后的源码。token 序列不变，仅调整空白与空行。
pub fn format(src: &str) -> String {
    let toks = lex_all(src);
    if toks.is_empty() {
        return src.trim_end_matches('\n').to_string();
    }
    let mut lines = build_lines(src, toks);
    let mut out = String::new();
    let mut prev_blank = false;
    let n = lines.len();
    for (i, ln) in lines.iter_mut().enumerate() {
        if ln.blank {
            if i == 0 || prev_blank || i == n - 1 {
                continue;
            }
            out.push('\n');
            prev_blank = true;
            continue;
        }
        prev_blank = false;
        if i > 0 {
            out.push('\n');
        }
        if ln.raw {
            out.push_str(ln.text.trim_end_matches([' ', '\t']));
        } else {
            render_line(ln, &mut out);
        }
    }
    out.trim_end_matches('\n').to_string() + "\n"
}

fn lex_all(src: &str) -> Vec<Token> {
    let mut lx = Lexer::new(src);
    let mut toks = Vec::new();
    loop {
        let t = lx.next();
        let done = t.ty == TokenType::Eof;
        toks.push(t);
        if done {
            break;
        }
    }
    toks
}

// build_lines 按行分组 token，并识别注释/跨行字符串/空行。
fn build_lines(src: &str, toks: Vec<Token>) -> Vec<Line> {
    let src_lines: Vec<&str> = src.split('\n').collect();
    let mut lines: Vec<Line> = src_lines
        .iter()
        .enumerate()
        .map(|(i, s)| Line {
            num: i + 1,
            text: s.to_string(),
            tokens: Vec::new(),
            raw: false,
            comment: String::new(),
            blank: false,
        })
        .collect();
    for t in toks {
        match t.ty {
            TokenType::Newline | TokenType::Indent | TokenType::Dedent | TokenType::Eof => continue,
            _ => {}
        }
        let idx = t.line as usize - 1;
        if idx >= lines.len() {
            continue;
        }
        match t.ty {
            TokenType::Illegal => {
                lines[idx].raw = true;
                continue;
            }
            TokenType::String => {
                let end = t.line as usize - 1 + t.lit.matches('\n').count();
                if end > t.line as usize - 1 {
                    for j in t.line as usize - 1..=end {
                        if j < lines.len() {
                            lines[j].raw = true;
                        }
                    }
                }
            }
            _ => {}
        }
        lines[idx].tokens.push(t);
    }
    // 注释与空行识别：行内 token 序列结束列之后的 # 内容为注释。
    for i in 0..lines.len() {
        if lines[i].raw {
            continue;
        }
        if lines[i].tokens.is_empty() {
            if lines[i].text.trim().is_empty() {
                lines[i].blank = true;
            } else {
                lines[i].raw = true;
            }
        }
        if !lines[i].tokens.is_empty() {
            let last = lines[i].tokens.last().unwrap().clone();
            let end_col = last.col as usize + last.lit.len();
            if end_col > 1 && end_col - 1 <= lines[i].text.len() {
                if let Some(ci) = lines[i].text[end_col - 1..].find('#') {
                    lines[i].comment = lines[i].text[end_col - 1 + ci..].to_string();
                }
            }
        } else if !lines[i].text.trim().is_empty() {
            if lines[i].text.trim_start().starts_with('#') {
                lines[i].comment = lines[i].text.trim().to_string();
            }
        }
    }
    lines
}

// render_line 重排行内 token 空白并附加注释。
fn render_line(ln: &mut Line, out: &mut String) {
    let trimmed = ln.text.trim_start_matches([' ', '\t']);
    let lead_len = ln.text.len() - trimmed.len();
    let mut lead = String::new();
    if lead_len > 0 {
        lead = ln.text[..lead_len].replace('\t', "    ");
    }
    out.push_str(&lead);
    let mut depth: i64 = 0;
    let mut bracket: i64 = 0;
    let mut prev = Token {
        ty: TokenType::Illegal,
        lit: String::new(),
        line: 0,
        col: 0,
    };
    let mut prev_unary = false;
    for (i, t) in ln.tokens.iter().cloned().enumerate() {
        let space = decide_space(&prev, prev_unary, &t, depth, bracket, i == 0);
        if space && out.len() > lead.len() {
            out.push(' ');
        }
        out.push_str(&t.lit);
        match t.ty {
            TokenType::Minus | TokenType::Plus | TokenType::Tilde | TokenType::Star | TokenType::DoubleStar => {
                prev_unary = !(is_name(prev.ty) || is_lit(prev.ty) || is_close(prev.ty) || prev.ty == TokenType::Dot);
            }
            _ => prev_unary = false,
        }
        prev = t.clone();
        match t.ty {
            TokenType::LParen | TokenType::LBracket | TokenType::LBrace => {
                depth += 1;
                if t.ty == TokenType::LBracket {
                    bracket += 1;
                }
            }
            TokenType::RParen | TokenType::RBracket | TokenType::RBrace => {
                depth -= 1;
                if t.ty == TokenType::RBracket && bracket > 0 {
                    bracket -= 1;
                }
            }
            _ => {}
        }
    }
    if !ln.comment.is_empty() {
        if !out.is_empty() {
            out.push(' ');
        }
        out.push_str(ln.comment.trim_end_matches([' ', '\t']));
    }
    while out.ends_with([' ', '\t']) {
        out.pop();
    }
}

// decide_space 决定 cur 前是否需要一个空格。
fn decide_space(
    prev: &Token,
    prev_unary: bool,
    cur: &Token,
    depth: i64,
    bracket: i64,
    at_line_start: bool,
) -> bool {
    if at_line_start {
        return false;
    }
    let pt = prev.ty;
    let ct = cur.ty;
    if is_close(ct) {
        return false;
    }
    if is_open(ct) {
        match pt {
            TokenType::Ident | TokenType::Cage | TokenType::Trace | TokenType::Only => return false,
            TokenType::RParen | TokenType::RBracket | TokenType::RBrace | TokenType::LParen | TokenType::LBracket | TokenType::LBrace => {
                return false
            }
            TokenType::Int | TokenType::Float | TokenType::String | TokenType::None | TokenType::True | TokenType::False => {
                return false
            }
            _ => {}
        }
        return true;
    }
    if is_name(ct) || is_lit(ct) {
        if is_open(pt) || pt == TokenType::Dot {
            return false;
        }
        if pt == TokenType::Colon {
            return bracket == 0;
        }
        if pt == TokenType::Assign && depth > 0 {
            return false;
        }
        if prev_unary {
            return false;
        }
        return true;
    }
    match ct {
        TokenType::Dot => return false,
        TokenType::Comma => return false,
        TokenType::Colon => return false,
        TokenType::Assign => return depth == 0,
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
        | TokenType::CaretAssign => return true,
        TokenType::Minus | TokenType::Plus | TokenType::Tilde => {
            return !(is_open(pt) || pt == TokenType::Comma || pt == TokenType::Colon)
        }
        TokenType::Star | TokenType::DoubleStar => {
            return !(is_open(pt) || pt == TokenType::Comma || pt == TokenType::Colon)
        }
        TokenType::Slash
        | TokenType::Percent
        | TokenType::FloorDiv
        | TokenType::Shl
        | TokenType::Shr
        | TokenType::Amp
        | TokenType::Pipe
        | TokenType::Caret
        | TokenType::Lt
        | TokenType::Gt
        | TokenType::Le
        | TokenType::Ge
        | TokenType::EqEq
        | TokenType::Ne
        | TokenType::And
        | TokenType::Or => return true,
        TokenType::Is => return pt != TokenType::Not,
        TokenType::In => return true,
        TokenType::Not => return true,
        _ => {}
    }
    true
}

fn is_open(t: TokenType) -> bool {
    t == TokenType::LParen || t == TokenType::LBracket || t == TokenType::LBrace
}

fn is_close(t: TokenType) -> bool {
    t == TokenType::RParen || t == TokenType::RBracket || t == TokenType::RBrace
}

fn is_name(t: TokenType) -> bool {
    t == TokenType::Ident
}

fn is_lit(t: TokenType) -> bool {
    matches!(
        t,
        TokenType::Int
            | TokenType::Float
            | TokenType::String
            | TokenType::None
            | TokenType::True
            | TokenType::False
    )
}

// CommentLines 供 analyze 复用：返回源码行注释行号集合（供注释比例统计）。
pub fn comment_lines(src: &str) -> Vec<usize> {
    let toks = lex_all(src);
    let lines = build_lines(src, toks);
    lines
        .iter()
        .filter(|ln| !ln.comment.is_empty())
        .map(|ln| ln.num)
        .collect()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn fmt_basic() {
        let src = "x=1\nif x>0:\n    print(x)\n";
        let out = format(src);
        assert_eq!(out, "x = 1\nif x > 0:\n    print(x)\n");
    }

    #[test]
    fn fmt_comment_kept() {
        let src = "x = 1  # 注释\n";
        let out = format(src);
        assert_eq!(out, "x = 1 # 注释\n");
    }

    #[test]
    fn fmt_blank_collapse() {
        let src = "x = 1\n\n\n\ny = 2\n";
        let out = format(src);
        assert_eq!(out, "x = 1\n\ny = 2\n");
    }

    #[test]
    fn fmt_not_in_keeps_space() {
        let src = "if x not in y:\n    pass\nz = 1 in (1, 2)\n";
        let out = format(src);
        assert_eq!(out, "if x not in y:\n    pass\nz = 1 in (1, 2)\n");
    }

    #[test]
    fn comment_lines_ok() {
        let src = "x = 1\n# 注释\ny = 2\n";
        let c = comment_lines(src);
        assert_eq!(c, vec![2]);
    }
}