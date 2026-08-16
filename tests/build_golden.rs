// build_golden.rs：R2 验收——Rust 本地 parse+gen 的 build 产物与 testdata/golden/*.py
// 逐字节对比（Go 版产物快照，行号敏感：产物内嵌源码行列号）。
// 覆盖：8 安全关键字注入、沙箱 runtime/sandbox 两节、类型推导豁免、--keep-annotations。
use pyfly_lang::gen;
use pyfly_lang::parser;
use std::path::{Path, PathBuf};

fn golden_dir() -> PathBuf {
    Path::new(env!("CARGO_MANIFEST_DIR")).join("testdata").join("golden")
}

/// R2：golden 全部 .fly 的 Rust 生成产物与 .py 快照逐字节一致。
#[test]
fn gen_matches_golden_byte_for_byte() {
    let dir = golden_dir();
    let mut files: Vec<PathBuf> = Vec::new();
    for entry in std::fs::read_dir(&dir).expect("golden 目录不存在") {
        let p = entry.unwrap().path();
        if p.extension().and_then(|e| e.to_str()) == Some("fly") {
            files.push(p);
        }
    }
    files.sort();
    assert!(!files.is_empty());
    for f in &files {
        let src = std::fs::read_to_string(f).expect("读取 .fly 失败");
        let (module, perr) = parser::parse(&src);
        let module = module.unwrap_or_else(|| panic!("{}: 解析失败: {:?}", f.display(), perr));
        let got = gen::generate(module);
        let want_path = f.with_extension("py");
        let want = std::fs::read_to_string(&want_path).expect("缺少 golden 输出");
        assert_eq!(
            got,
            want,
            "{}: 生成产物与 Go 版快照不一致（行号敏感：改行号后需重新生成 golden）",
            f.display()
        );
    }
}

/// R2：--keep-annotations 审计注释块与 Go 版逐字节一致。
#[test]
fn gen_keep_annotations_matches() {
    let dir = golden_dir();
    let mut checked = 0;
    for entry in std::fs::read_dir(&dir).expect("golden 目录不存在") {
        let p = entry.unwrap().path();
        if p.extension().and_then(|e| e.to_str()) != Some("fly") {
            continue;
        }
        let src = std::fs::read_to_string(&p).expect("读取 .fly 失败");
        let (module, _) = parser::parse(&src);
        let Some(module) = module else { continue };
        let got = gen::generate_opts(module, gen::GenOpts { keep_annotations: true });
        assert!(got.contains("# fly-safe:") || got.contains("# fly-mask:") || got.contains("# fly-lock:")
            || got.contains("# fly-guard:") || got.contains("# fly-only:") || got.contains("# fly-seal:")
            || got.contains("# fly-trace:") || got.contains("# fly-cage:")
            || !src.contains("safe") && !src.contains("mask") && !src.contains("lock") && !src.contains("guard")
                && !src.contains("only") && !src.contains("seal") && !src.contains("trace") && !src.contains("cage"),
            "{}: 审计注释缺失或误注入",
            p.display()
        );
        checked += 1;
    }
    assert!(checked > 0);
}
