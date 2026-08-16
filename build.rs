use std::process::Command;

fn git(args: &[&str]) -> Option<String> {
    Command::new("git")
        .args(args)
        .output()
        .ok()
        .filter(|o| o.status.success())
        .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_string())
        .filter(|s| !s.is_empty())
}

fn main() {
    // 与 Go 版 ldflags 注入语义对齐：version/commit 均须显式注入
    // （release/CI 设置 FLY_VERSION/FLY_COMMIT），本地构建输出裸 "dev"。
    let version = match std::env::var("FLY_VERSION") {
        Ok(v) if !v.is_empty() => v,
        _ => "dev".to_string(),
    };
    let commit = match std::env::var("FLY_COMMIT") {
        Ok(v) if !v.is_empty() => v,
        _ => {
            // release 构建（显式 FLY_VERSION）且未给 commit 时，自动取 git HEAD 后备
            if version != "dev" {
                git(&["rev-parse", "HEAD"]).unwrap_or_default()
            } else {
                String::new()
            }
        }
    };
    println!("cargo:rustc-env=FLY_COMMIT={}", commit);
    println!("cargo:rustc-env=FLY_VERSION={}", version);
    println!("cargo:rustc-env=FLY_REPO=29anan29/PyFly-lang");
    println!("cargo:rerun-if-changed=build.rs");
    println!("cargo:rerun-if-env-changed=FLY_VERSION");
    println!("cargo:rerun-if-env-changed=FLY_COMMIT");
}
