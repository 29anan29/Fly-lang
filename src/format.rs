// format.rs：错误渲染——Rust 风格 error[EXXXX] + 源码行下划线高亮 + help/note。
use crate::diagnostic::{Diagnostic, Position};
use crate::errorcode;

// 与 Go 版 internal/compile/compile.go formatError 逐行对齐（零差异约束）。
pub fn format_error(path: &str, src: &str, d: &Diagnostic, color: bool) -> String {
    let info = errorcode::info_for_code(&d.code);
    let Some(info) = info else {
        // Go 版 formatError 对无错误码诊断不追加 \n（Fprintln 统一加）
        if d.pos.line == 0 {
            return format!("error: {}: {}", path, d.msg);
        }
        return format!("error: {}:{}:{}: {}", path, d.pos.line, d.pos.col, d.msg);
    };
    let mut b = String::new();
    b.push_str(&color_wrap(color, "bred", &format!("error[{}]", d.code)));
    b.push_str(": ");
    b.push_str(info.title);
    b.push('\n');
    b.push_str(&color_wrap(
        color,
        "cyan",
        &format!("  --> {}:{}:{}", path, d.pos.line, d.pos.col),
    ));
    b.push('\n');
    if let Some(line) = src_line(src, d.pos.line) {
        let len = underline_len(&line, d.pos.col);
        b.push_str("   |\n");
        b.push_str(&format!("{:4} | {}\n", d.pos.line, line));
        let under = "^".repeat(len.saturating_sub(1));
        b.push_str("   | ");
        b.push_str(&color_wrap(
            color,
            "red",
            &format!(
                "{}^{}",
                " ".repeat(d.pos.col.saturating_sub(1) as usize),
                under
            ),
        ));
        b.push('\n');
    }
    b.push_str("   |\n");
    if d.msg != info.title {
        b.push_str(&format!("   = help: {}。{}\n", d.msg, info.help));
    } else {
        b.push_str(&format!("   = help: {}\n", info.help));
    }
    b.push_str(&format!("   = note: {}\n", info.note));
    b
}

fn color_wrap(color: bool, code: &str, s: &str) -> String {
    if !color {
        return s.to_string();
    }
    let (start, reset) = match code {
        "red" => ("\x1b[31m", "\x1b[0m"),
        "bred" => ("\x1b[1;31m", "\x1b[0m"),
        "cyan" => ("\x1b[1;36m", "\x1b[0m"),
        _ => ("", ""),
    };
    format!("{}{}{}", start, s, reset)
}

fn src_line(src: &str, line: u32) -> Option<String> {
    if src.is_empty() {
        return None;
    }
    let mut cur = 1u32;
    for l in src.split_inclusive('\n') {
        if cur == line {
            let trimmed = l.strip_suffix('\n').unwrap_or(l);
            return Some(trimmed.to_string());
        }
        cur += 1;
    }
    None
}

fn underline_len(line: &str, col: u32) -> usize {
    let bytes = line.as_bytes();
    let col_off = col.saturating_sub(1) as usize;
    if col_off > bytes.len() {
        return 1;
    }
    let rest = &bytes[col_off..];
    let mut n = 0;
    while n < rest.len() && n < 32 && rest[n] != b' ' {
        n += 1;
    }
    if n == 0 {
        return 1;
    }
    n
}

pub fn errno_text(errno: i32) -> String {
    match errno {
        2 => "no such file or directory".to_string(),
        13 => "permission denied".to_string(),
        20 => "not a directory".to_string(),
        21 => "is a directory".to_string(),
        40 => "too many levels of symbolic links".to_string(),
        36 => "file name too long".to_string(),
        _ => format!("errno {}", errno),
    }
}

pub fn stat_like_error(op: &str, path: &str, errno: i32) -> String {
    format!("{} {}: {}", op, path, errno_text(errno))
}

// checkd 读文件失败构造的"伪诊断"（无错误码、line 0），对齐 Go CheckFile 行为。
pub fn file_diag(path: &str, errno: i32) -> Diagnostic {
    Diagnostic {
        pos: Position { line: 0, col: 0 },
        code: String::new(),
        msg: format!("open {}: {}", path, errno_text(errno)),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn diag(code: &str, line: u32, col: u32, msg: &str) -> Diagnostic {
        Diagnostic {
            pos: Position { line, col },
            code: code.to_string(),
            msg: msg.to_string(),
        }
    }

    #[test]
    fn syntax_error_block() {
        let src = "x = (1 +\n";
        let d = diag("E0002", 2, 1, "期望表达式，实际为 \"\"");
        let out = format_error("test.fly", src, &d, false);
        assert!(out.starts_with("error[E0002]: "), "{}", out);
        assert!(out.contains("  --> test.fly:2:1"));
        assert!(!out.contains("   2 | "), "src 只有 1 行，Go 版同样无源码行");
        assert!(out.contains("   = help: "));
        assert!(out.contains("   = note: "));
    }

    #[test]
    fn with_source_line_underline() {
        let src = "def f(:\npass\n";
        let d = diag("E0001", 1, 6, "期望 标识符，实际为 换行");
        let out = format_error("a.fly", src, &d, false);
        assert!(out.contains("   1 | def f(:\n"), "{}", out);
        assert!(out.contains("   |      ^^\n"), "{}", out);
    }

    #[test]
    fn no_code_mid_line() {
        let d = diag("", 3, 5, "boom");
        assert_eq!(
            format_error("a.fly", "1\n2\n3\n", &d, false),
            "error: a.fly:3:5: boom"
        );
    }

    #[test]
    fn no_code_zero_line() {
        let d = diag("", 0, 0, "open x: no such file or directory");
        assert_eq!(
            format_error("x.fly", "", &d, false),
            "error: x.fly: open x: no such file or directory"
        );
    }

    #[test]
    fn color_wrapped() {
        let src = "x = 1\n";
        let d = diag("E0002", 1, 1, "zzz");
        let out = format_error("t.fly", src, &d, true);
        assert!(out.contains("\x1b[1;31merror[E0002]\x1b[0m"));
    }
}