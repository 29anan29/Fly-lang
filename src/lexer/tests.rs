// tests.rs：词法单测（token 序列断言，与 Go 版 lexer_test 对应）。
use crate::lexer::lexer::Lexer;
use crate::lexer::token::{Token, TokenType};

fn tokenize(src: &str) -> Vec<Token> {
    let mut l = Lexer::new(src);
    let mut toks = Vec::new();
    loop {
        let t = l.next();
        let done = t.ty == TokenType::Eof;
        toks.push(t);
        if done {
            break;
        }
    }
    toks
}

fn types(src: &str) -> Vec<TokenType> {
    tokenize(src).into_iter().map(|t| t.ty).collect()
}

fn assert_types(src: &str, want: &[TokenType]) {
    let got = types(src);
    assert_eq!(got.len(), want.len(), "token 数量不匹配: src={:?}", src);
    for (i, w) in want.iter().enumerate() {
        assert_eq!(got[i], *w, "token {} 类型不匹配: src={:?}", i, src);
    }
}

#[test]
fn keywords() {
    assert_types(
        "safe only lock mask cage guard seal trace",
        &[
            TokenType::Safe,
            TokenType::Only,
            TokenType::Lock,
            TokenType::Mask,
            TokenType::Cage,
            TokenType::Guard,
            TokenType::Seal,
            TokenType::Trace,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn indent_dedent() {
    assert_types(
        "if x:\n    y = 1\n    z = 2\nw = 3\n",
        &[
            TokenType::If,
            TokenType::Ident,
            TokenType::Colon,
            TokenType::Newline,
            TokenType::Indent,
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Dedent,
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn blank_and_comment_lines() {
    assert_types(
        "a = 1\n\n# comment\n\nb = 2\n",
        &[
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn strings() {
    let toks = tokenize("a = f\"x {y}\" 's' r\"r\" b\"b\" \"q\" 'q'\n");
    let got: Vec<TokenType> = toks.iter().map(|t| t.ty).collect();
    let want = [
        TokenType::Ident,
        TokenType::Assign,
        TokenType::String,
        TokenType::String,
        TokenType::String,
        TokenType::String,
        TokenType::String,
        TokenType::String,
        TokenType::Newline,
        TokenType::Eof,
    ];
    assert_eq!(got, want);
    assert_eq!(toks[2].lit, "f\"x {y}\"");
}

#[test]
fn triple_quoted() {
    let toks = tokenize("s = \"\"\"a\nb\"\"\"\n");
    assert_eq!(toks[2].ty, TokenType::String);
    assert_eq!(toks[2].lit, "\"\"\"a\nb\"\"\"");
}

#[test]
fn numbers() {
    assert_types(
        "a = 0xFF + 0b10 - 0o17 * 1_000 // 2 % 1.5e3 ** .5",
        &[
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Plus,
            TokenType::Int,
            TokenType::Minus,
            TokenType::Int,
            TokenType::Star,
            TokenType::Int,
            TokenType::FloorDiv,
            TokenType::Int,
            TokenType::Percent,
            TokenType::Float,
            TokenType::DoubleStar,
            TokenType::Float,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn operators() {
    assert_types(
        "a <= b >= c != d == e < f > g and x or y",
        &[
            TokenType::Ident,
            TokenType::Le,
            TokenType::Ident,
            TokenType::Ge,
            TokenType::Ident,
            TokenType::Ne,
            TokenType::Ident,
            TokenType::EqEq,
            TokenType::Ident,
            TokenType::Lt,
            TokenType::Ident,
            TokenType::Gt,
            TokenType::Ident,
            TokenType::And,
            TokenType::Ident,
            TokenType::Or,
            TokenType::Ident,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn aug_assign() {
    assert_types(
        "a += 1; b -= 2; c *= 3; d **= 4",
        &[
            TokenType::Ident,
            TokenType::PlusAssign,
            TokenType::Int,
            TokenType::Semicolon,
            TokenType::Ident,
            TokenType::MinusAssign,
            TokenType::Int,
            TokenType::Semicolon,
            TokenType::Ident,
            TokenType::StarAssign,
            TokenType::Int,
            TokenType::Semicolon,
            TokenType::Ident,
            TokenType::DoubleStarAssign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn parenthesized_continuation() {
    assert_types(
        "x = (1 +\n     2)\ny = 3\n",
        &[
            TokenType::Ident,
            TokenType::Assign,
            TokenType::LParen,
            TokenType::Int,
            TokenType::Plus,
            TokenType::Int,
            TokenType::RParen,
            TokenType::Newline,
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn backslash_continuation() {
    assert_types(
        "x = 1 + \\\n    2\ny = 3\n",
        &[
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Plus,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Ident,
            TokenType::Assign,
            TokenType::Int,
            TokenType::Newline,
            TokenType::Eof,
        ],
    );
}

#[test]
fn positions() {
    let toks = tokenize("x = 1\nabc = 2\n");
    assert_eq!((toks[0].line, toks[0].col), (1, 1));
    assert_eq!((toks[4].line, toks[4].col), (2, 1));
}

#[test]
fn errors() {
    let cases = [
        ("unterminated string", "s = \"abc"),
        ("bad char", "x = 1 $ 2\n"),
        ("bad indent", "def f():\n\tpass\n"),
        ("unclosed paren", "x = (1 + 2\n"),
        ("extra paren", "x = 1)\n"),
        ("invalid number", "x = 1abc\n"),
        ("bad exponent", "x = 1e\n"),
    ];
    for (name, src) in cases {
        let mut l = Lexer::new(src);
        loop {
            let t = l.next();
            if t.ty == TokenType::Eof {
                break;
            }
        }
        assert!(l.err().is_some(), "{}: 期望报错，实际通过", name);
    }
}
