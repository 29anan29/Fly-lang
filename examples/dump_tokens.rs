use std::env;
use std::fs;

use fly_lang::lexer::lexer::Lexer;

fn main() {
    let path = env::args().nth(1).expect("usage: dump_tokens <file>");
    let src = fs::read_to_string(&path).unwrap();
    let mut l = Lexer::new(&src);
    loop {
        let t = l.next();
        println!("{} {}", fly_lang::lexer::token::name(t.ty), t.lit);
        if t.ty == fly_lang::lexer::token::TokenType::Eof {
            break;
        }
    }
    if let Some(e) = l.err() {
        eprintln!("{}:{}: error[{}]: {}", e.pos.line, e.pos.col, e.code, e.msg);
    }
}
