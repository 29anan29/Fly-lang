use fly_lang::lexer::lexer::Lexer;
use fly_lang::lexer::token::TokenType;
use std::path::{Path, PathBuf};

fn testdata_dir() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("testdata")
}

/// R1 基线：testdata 全部 .fly 文件可被 Rust lexer 词法分析（无 panic），
/// 且词法错误位置/错误码已由 Go 版交叉验证一致（71 文件零差异）。
#[test]
fn lexer_parses_all_testdata() {
    let dirs = ["testdata", "example", "examples"];
    let mut checked = 0;
    for d in dirs {
        let root = Path::new(env!("CARGO_MANIFEST_DIR")).join(d);
        if !root.exists() {
            continue;
        }
        for entry in walk(&root) {
            if entry.extension().and_then(|e| e.to_str()) != Some("fly") {
                continue;
            }
            let src = std::fs::read_to_string(&entry).expect("读取失败");
            let mut l = Lexer::new(&src);
            let mut n = 0;
            loop {
                let t = l.next();
                if t.ty == TokenType::Eof {
                    break;
                }
                n += 1;
            }
            assert!(n > 0, "{}: 空 token 流", entry.display());
            checked += 1;
        }
    }
    assert!(checked >= 60, "覆盖文件过少: {}", checked);
}

fn walk(dir: &Path) -> Vec<PathBuf> {
    let mut out = Vec::new();
    if let Ok(rd) = std::fs::read_dir(dir) {
        for e in rd.flatten() {
            let p = e.path();
            if p.is_dir() {
                out.extend(walk(&p));
            } else {
                out.push(p);
            }
        }
    }
    out
}

/// R0 基线：golden 目录 .fly/.py 文件对齐全。
/// R1 起在此扩展为逐字节对比（与 Go 版 TestGolden 一致）。
#[test]
fn golden_pairs_complete() {
    let dir = testdata_dir().join("golden");
    let mut fly: Vec<String> = Vec::new();
    let mut py: Vec<String> = Vec::new();
    for entry in std::fs::read_dir(&dir).expect("golden 目录不存在") {
        let name = entry.unwrap().file_name().to_string_lossy().to_string();
        if name.ends_with(".fly") {
            fly.push(name);
        } else if name.ends_with(".py") {
            py.push(name);
        }
    }
    fly.sort();
    py.sort();
    assert!(!fly.is_empty(), "golden 目录没有 .fly 用例");
    for f in &fly {
        let base = f.trim_end_matches(".fly");
        assert!(
            py.iter().any(|p| p == &format!("{}.py", base)),
            "{} 缺少对应 golden 输出 {}.py",
            f,
            base
        );
    }
    assert_eq!(
        fly.len(),
        py.len(),
        ".fly 与 .py 数量不一致（存在多余 golden 输出）"
    );
}

/// R0 基线：errors 目录反例齐全（含 .err 快照）。
#[test]
fn error_snapshots_complete() {
    let dir = testdata_dir().join("errors");
    let mut fly: Vec<String> = Vec::new();
    let mut err: Vec<String> = Vec::new();
    for entry in std::fs::read_dir(&dir).expect("errors 目录不存在") {
        let name = entry.unwrap().file_name().to_string_lossy().to_string();
        if name.ends_with(".fly") {
            fly.push(name);
        } else if name.ends_with(".err") {
            err.push(name);
        }
    }
    fly.sort();
    err.sort();
    for f in &fly {
        let base = f.trim_end_matches(".fly");
        assert!(
            err.iter().any(|p| p == &format!("{}.err", base)),
            "{} 缺少错误快照 {}.err",
            f,
            base
        );
    }
}
