// main.rs：Rust CLI 入口——check/version/error 子命令（P1/P2 已交付，checker 走 checkd 桥接）。
use std::io::IsTerminal;
use std::path::{Path, PathBuf};
use std::process::ExitCode;
use std::sync::{Condvar, Mutex};

use fly_lang::checkd;
use fly_lang::errorcode;
use fly_lang::format;

struct Semaphore {
    count: Mutex<usize>,
    cond: Condvar,
}

impl Semaphore {
    fn new(n: usize) -> Self {
        Semaphore {
            count: Mutex::new(n),
            cond: Condvar::new(),
        }
    }

    fn acquire(&self) {
        let mut c = self.count.lock().unwrap();
        while *c == 0 {
            c = self.cond.wait(c).unwrap();
        }
        *c -= 1;
    }

    fn release(&self) {
        let mut c = self.count.lock().unwrap();
        *c += 1;
        self.cond.notify_one();
    }
}

fn main() -> ExitCode {
    let args: Vec<String> = std::env::args().skip(1).collect();
    match args.first().map(String::as_str) {
        Some("version") => {
            println!("{}", fly_lang::version::string());
            ExitCode::SUCCESS
        }
        Some("check") => cmd_check(&args[1..]),
        Some("error") => cmd_error(&args[1..]),
        Some("help") | Some("-h") | Some("--help") => {
            println!("{}", USAGE);
            ExitCode::SUCCESS
        }
        Some(cmd) if matches!(cmd, "build" | "run" | "sandbox" | "update" | "lsp") => {
            eprintln!("error: 子命令 {} 尚未迁移到 Rust 核心（P1 已交付：version/help/error；P2 check 已迁移）", cmd);
            ExitCode::from(1)
        }
        Some(_) => {
            eprintln!("未知子命令 {:?}\n\n{}\n", args[0], USAGE);
            ExitCode::from(2)
        }
        None => {
            eprintln!("{}", USAGE);
            ExitCode::from(2)
        }
    }
}

fn cmd_error(args: &[String]) -> ExitCode {
    if args.len() != 1 {
        eprintln!("用法: fly error <E码>（如 fly error E0031）");
        return ExitCode::from(2);
    }
    let mut code = args[0].to_uppercase();
    if !code.starts_with('E') {
        code = format!("E{}", code);
    }
    if let Ok(n) = code.trim_start_matches('E').parse::<u32>() {
        code = format!("E{:04}", n);
    }
    match errorcode::info_for_code(&code) {
        Some(info) => {
            println!("{}", info.example);
            ExitCode::SUCCESS
        }
        None => {
            eprint!("未知错误码 {}\n\n全部错误码见 docs/报错清单.md\n", code);
            ExitCode::from(1)
        }
    }
}

struct CheckOutcome {
    path: String,
    blocks: Vec<String>,
}

fn cmd_check(args: &[String]) -> ExitCode {
    if args.is_empty() {
        eprintln!("用法: fly check <file.fly>...（支持目录，递归查找 .fly，并发检查）");
        return ExitCode::from(2);
    }
    let Some(checkd) = checkd::find_checkd() else {
        eprintln!("error: 找不到 fly-checkd（设置 FLY_CHECKD 环境变量指定路径）");
        return ExitCode::from(1);
    };

    let mut files: Vec<String> = Vec::new();
    for a in args {
        let p = Path::new(a);
        match std::fs::metadata(p) {
            Err(e) => {
                let errno = e.raw_os_error().unwrap_or(-1);
                eprintln!("error: {}", format::stat_like_error("stat", a, errno));
                return ExitCode::from(1);
            }
            Ok(m) => {
                if m.is_dir() {
                    match walk_fly(p, &mut files) {
                        Ok(()) => {}
                        Err(e) => {
                            let errno = e.raw_os_error().unwrap_or(-1);
                            eprintln!("error: {}", format::stat_like_error("stat", a, errno));
                            return ExitCode::from(1);
                        }
                    }
                } else {
                    files.push(a.clone());
                }
            }
        }
    }
    if files.is_empty() {
        eprintln!("error: 未找到 .fly 文件");
        return ExitCode::from(1);
    }

    let nproc = std::thread::available_parallelism().map(|n| n.get()).unwrap_or(4);
    let sem = Semaphore::new(nproc.saturating_mul(2));
    let color = std::io::stdout().is_terminal() && std::env::var("NO_COLOR").map_or(true, |v| v.is_empty());

    let results: Vec<CheckOutcome> = std::thread::scope(|scope| {
        let mut handles = Vec::with_capacity(files.len());
        for path in &files {
            let path = path.clone();
            let checkd = checkd.clone();
            let sem = &sem;
            handles.push(scope.spawn(move || {
                sem.acquire();
                let r = check_one(&checkd, &path, color);
                sem.release();
                r
            }));
        }
        handles.into_iter().map(|h| h.join().expect("线程")).collect()
    });

    let mut failed = 0usize;
    for r in &results {
        for b in &r.blocks {
            eprintln!("{}", b);
        }
        if !r.blocks.is_empty() {
            failed += 1;
        }
    }
    if failed > 0 {
        eprintln!("{} 个文件检查失败", failed);
        return ExitCode::from(1);
    }
    println!("ok: {} 个文件检查通过", files.len());
    ExitCode::SUCCESS
}

fn check_one(checkd: &PathBuf, path: &str, color: bool) -> CheckOutcome {
    match std::fs::read(path) {
        Err(e) => {
            let errno = e.raw_os_error().unwrap_or(-1);
            let d = format::file_diag(path, errno);
            let src = String::new();
            let block = format::format_error(path, &src, &d, color);
            CheckOutcome {
                path: path.to_string(),
                blocks: vec![block],
            }
        }
        Ok(bytes) => {
            let src = String::from_utf8_lossy(&bytes).into_owned();
            match checkd::check_src(checkd, &src, path, color) {
                Ok(r) => {
                    let blocks = if let Some(e) = r.server_error {
                        vec![format!("error: {}: {}", path, e)]
                    } else {
                        r.diags
                            .iter()
                            .map(|d| format::format_error(path, &src, d, color))
                            .collect()
                    };
                    CheckOutcome {
                        path: path.to_string(),
                        blocks,
                    }
                }
                Err(e) => CheckOutcome {
                    path: path.to_string(),
                    blocks: vec![format!("error: {}: {}", path, e)],
                },
            }
        }
    }
}

fn walk_fly(dir: &Path, out: &mut Vec<String>) -> std::io::Result<()> {
    let mut entries: Vec<_> = std::fs::read_dir(dir)?.collect::<Result<_, _>>()?;
    entries.sort_by_key(|e| e.file_name());
    for e in entries {
        let p = e.path();
        let ft = e.file_type()?;
        if ft.is_dir() {
            walk_fly(&p, out)?;
        } else if ft.is_file() && p.extension().map_or(false, |x| x == "fly") {
            out.push(p.to_string_lossy().into_owned());
        }
    }
    Ok(())
}

const USAGE: &str = "Fly-Lang 编译器

用法:
  fly build [选项] <file.fly>   转译为 Python（含沙箱运行时，拦截逃逸）
  fly check <file.fly>...       编译检查（支持目录递归，goroutine 并发）
  fly run <file.fly>            转译并在沙箱内执行
  fly sandbox <script.py>       进程级沙箱运行 Python（Landlock+seccomp+ns）
  fly version                  显示版本
  fly error <E码>              查询错误码（示例报错与修复方法）
  fly update [选项]             检查/更新到最新版本

build 选项:
  -o <out.py>   指定输出文件（默认输出到 build/ 目录，保留相对路径）

update 选项:
  --check       仅检查新版本（有新版退出码 2）
  --force       同版本也强制更新
  --proxy <url> 走代理（http://、https://、socks5://）";