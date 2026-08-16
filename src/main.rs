// main.rs：Rust CLI 入口——check/version/error/build/run 子命令（P1-P3 已交付，checker 走 checkd 桥接）。
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
        Some("build") => cmd_build(&args[1..]),
        Some("run") => cmd_run(&args[1..]),
        Some("error") => cmd_error(&args[1..]),
        Some("help") | Some("-h") | Some("--help") => {
            println!("{}", USAGE);
            ExitCode::SUCCESS
        }
        Some(cmd) if matches!(cmd, "sandbox" | "update" | "lsp") => {
            eprintln!("error: 子命令 {} 尚未迁移到 Rust 核心（P1 已交付：version/help/error；P2 check；P3 build/run 已迁移）", cmd);
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

// cmd_build 转译 .fly 为 .py（与 Go 版逐字节对照：-o/默认 build/ 目录/--keep-annotations）。
fn cmd_build(args: &[String]) -> ExitCode {
    let mut out = String::new();
    let mut keep_ann = false;
    let mut file: Option<String> = None;
    let mut i = 0usize;
    while i < args.len() {
        match args[i].as_str() {
            "-o" => {
                if i + 1 >= args.len() {
                    eprintln!("-o 需要文件名参数");
                    return ExitCode::from(2);
                }
                i += 1;
                out = args[i].clone();
            }
            "--keep-annotations" => keep_ann = true,
            _ => {
                if file.is_some() {
                    eprintln!("只能指定一个输入文件");
                    return ExitCode::from(2);
                }
                file = Some(args[i].clone());
            }
        }
        i += 1;
    }
    let Some(file) = file else {
        eprintln!("用法: fly build [-o out.py] <file.fly>");
        return ExitCode::from(2);
    };

    let src = match std::fs::read(&file) {
        Ok(b) => String::from_utf8_lossy(&b).into_owned(),
        Err(e) => {
            let op = if std::fs::metadata(&file).map(|m| m.is_dir()).unwrap_or(false) {
                "read"
            } else {
                "open"
            };
            let errno = e.raw_os_error().unwrap_or(-1);
            eprintln!("error: {}: {} {}: {}", file, op, file, format::errno_text(errno));
            return ExitCode::from(1);
        }
    };

    let color = std::io::stdout().is_terminal() && std::env::var("NO_COLOR").map_or(true, |v| v.is_empty());
    let Some(checkd) = checkd::find_checkd() else {
        eprintln!("error: 找不到 fly-checkd（设置 FLY_CHECKD 环境变量指定路径）");
        return ExitCode::from(1);
    };
    match checkd::check_src(&checkd, &src, &file, color) {
        Ok(r) => {
            if let Some(e) = r.server_error {
                eprintln!("error: {}: {}", file, e);
                return ExitCode::from(1);
            }
            for d in &r.diags {
                eprintln!("{}", format::format_error(&file, &src, d, color));
            }
            if !r.diags.is_empty() {
                return ExitCode::from(1);
            }
        }
        Err(e) => {
            eprintln!("error: {}: {}", file, e);
            return ExitCode::from(1);
        }
    }

    let (module, perr) = fly_lang::parser::parse(&src);
    let Some(module) = module else {
        if let Some(d) = perr {
            eprintln!("{}", format::format_error(&file, &src, &d, color));
        } else {
            eprintln!("error: {}: 解析失败", file);
        }
        return ExitCode::from(1);
    };

    let code = fly_lang::gen::generate_opts(module, fly_lang::gen::GenOpts { keep_annotations: keep_ann });
    let out = if out.is_empty() {
        default_out_path(&file)
    } else {
        out
    };
    if let Some(dir) = Path::new(&out).parent() {
        if !dir.as_os_str().is_empty() {
            if let Err(e) = std::fs::create_dir_all(dir) {
                eprintln!("error: 创建目录 {} 失败: {}", dir.display(), e);
                return ExitCode::from(1);
            }
        }
    }
    if let Err(e) = std::fs::write(&out, &code) {
        eprintln!("error: 写入 {} 失败: {}", out, e);
        return ExitCode::from(1);
    }
    println!("ok: {} -> {}", file, out);
    ExitCode::SUCCESS
}

// default_out_path 复刻 Go defaultOutPath：默认输出到 build/ 目录，保留相对路径。
fn default_out_path(file: &str) -> String {
    let p = Path::new(file);
    let rel = if p.is_absolute() {
        match std::env::current_dir() {
            Ok(cwd) => match p.strip_prefix(&cwd) {
                Ok(r) => {
                    let r = r.to_string_lossy().into_owned();
                    if !r.starts_with("..") {
                        r
                    } else {
                        String::new()
                    }
                }
                Err(_) => String::new(),
            },
            Err(_) => String::new(),
        }
    } else {
        file.to_string()
    };
    let rel = if rel.is_empty() {
        p.file_name().map(|n| n.to_string_lossy().into_owned()).unwrap_or_default()
    } else {
        rel
    };
    let mut out = PathBuf::from("build");
    out.push(Path::new(&rel).with_extension("py"));
    out.to_string_lossy().into_owned()
}

// cmd_run 转译并在沙箱内执行（临时 .py + python3，退出码透传）。
fn cmd_run(args: &[String]) -> ExitCode {
    if args.len() != 1 {
        eprintln!("用法: fly run <file.fly>");
        return ExitCode::from(2);
    }
    let file = &args[0];
    let src = match std::fs::read(file) {
        Ok(b) => String::from_utf8_lossy(&b).into_owned(),
        Err(e) => {
            let op = if std::fs::metadata(&file).map(|m| m.is_dir()).unwrap_or(false) {
                "read"
            } else {
                "open"
            };
            let errno = e.raw_os_error().unwrap_or(-1);
            eprintln!("error: {}: {} {}: {}", file, op, file, format::errno_text(errno));
            return ExitCode::from(1);
        }
    };

    let color = std::io::stdout().is_terminal() && std::env::var("NO_COLOR").map_or(true, |v| v.is_empty());
    let Some(checkd) = checkd::find_checkd() else {
        eprintln!("error: 找不到 fly-checkd（设置 FLY_CHECKD 环境变量指定路径）");
        return ExitCode::from(1);
    };
    match checkd::check_src(&checkd, &src, file, color) {
        Ok(r) => {
            if let Some(e) = r.server_error {
                eprintln!("error: {}: {}", file, e);
                return ExitCode::from(1);
            }
            for d in &r.diags {
                eprintln!("{}", format::format_error(file, &src, d, color));
            }
            if !r.diags.is_empty() {
                return ExitCode::from(1);
            }
        }
        Err(e) => {
            eprintln!("error: {}: {}", file, e);
            return ExitCode::from(1);
        }
    }

    let (module, perr) = fly_lang::parser::parse(&src);
    let Some(module) = module else {
        if let Some(d) = perr {
            eprintln!("{}", format::format_error(file, &src, &d, color));
        } else {
            eprintln!("error: {}: 解析失败", file);
        }
        return ExitCode::from(1);
    };

    let code = fly_lang::gen::generate(module);
    let tmp = std::env::temp_dir().join(format!(
        "fly-{}-{}.py",
        std::process::id(),
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .map(|d| d.subsec_nanos())
            .unwrap_or(0)
    ));
    if let Err(e) = std::fs::write(&tmp, &code) {
        eprintln!("error: {}", e);
        return ExitCode::from(1);
    }
    let status = std::process::Command::new("python3")
        .arg(&tmp)
        .status();
    let _ = std::fs::remove_file(&tmp);
    match status {
        Ok(st) => match st.code() {
            Some(c) => ExitCode::from(c as u8),
            None => ExitCode::from(1),
        },
        Err(e) => {
            eprintln!("error: 无法执行 python3: {}", e);
            ExitCode::from(1)
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
            let color = std::io::stdout().is_terminal()
                && std::env::var("NO_COLOR").map_or(true, |v| v.is_empty());
            println!("{}", colorize_example(&info.example, color));
            ExitCode::SUCCESS
        }
        None => {
            eprint!("未知错误码 {}\n\n全部错误码见 docs/报错清单.md\n", code);
            ExitCode::from(1)
        }
    }
}

// fly error 的示例报错渲染 ANSI（与 format_error 同风格：error[EXXXX] 亮红、箭头青、help 绿、note 黄）。
fn colorize_example(s: &str, color: bool) -> String {
    if !color {
        return s.to_string();
    }
    let mut out = String::new();
    for line in s.split_inclusive('\n') {
        if line.starts_with("error[E") {
            out.push_str("\x1b[1;31m");
            out.push_str(line);
            out.push_str("\x1b[0m");
        } else if line.contains("--> ") {
            out.push_str("\x1b[1;36m");
            out.push_str(line);
            out.push_str("\x1b[0m");
        } else if line.starts_with("   = help:") {
            out.push_str("\x1b[32m");
            out.push_str(line);
            out.push_str("\x1b[0m");
        } else if line.starts_with("   = note:") {
            out.push_str("\x1b[33m");
            out.push_str(line);
            out.push_str("\x1b[0m");
        } else {
            out.push_str(line);
        }
    }
    out
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