// main.rs：Rust CLI 入口——check/version/error/build/run 子命令（P1-P3 已交付，checker 走 checkd 桥接）。
use std::io::IsTerminal;
use std::path::{Path, PathBuf};
use std::process::ExitCode;
use std::sync::{Condvar, Mutex};

use fly_lang::checkd;
use fly_lang::errorcode;
use fly_lang::format;
use fly_lang::http::Proxy;
use fly_lang::update::Updater;

// 报错渲染的彩色判定：走 stderr（诊断输出流）。stderr 是 TTY 或 FORCE_COLOR 非空 → 彩色；
// NO_COLOR 非空 → 强制无色。
fn err_color() -> bool {
    if !std::env::var("NO_COLOR").map_or(true, |v| v.is_empty()) {
        return false;
    }
    if !std::env::var("FORCE_COLOR").map_or(true, |v| v.is_empty()) {
        return true;
    }
    std::io::stderr().is_terminal()
}

// 普通输出（stdout 流）的彩色判定。
fn out_color() -> bool {
    if !std::env::var("NO_COLOR").map_or(true, |v| v.is_empty()) {
        return false;
    }
    if !std::env::var("FORCE_COLOR").map_or(true, |v| v.is_empty()) {
        return true;
    }
    std::io::stdout().is_terminal()
}

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
        Some("update") => cmd_update(&args[1..]),
        Some("help") | Some("-h") | Some("--help") => {
            println!("{}", USAGE);
            ExitCode::SUCCESS
        }
        Some("lsp") => cmd_lsp(&args[1..]),
        Some(cmd) if cmd == "sandbox" => {
            eprintln!("error: 子命令 sandbox 尚未迁移到 Rust 核心（P1-P4 已交付：version/help/error/check/build/run/update/lsp）");
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

    let color = err_color();
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

    let color = err_color();
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
            let color = out_color();
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

// ---- ANSI 输出工具（stdout 用 out_color，stderr 用 err_color）----
fn paint(code: &str, s: &str, on: bool) -> String {
    if !on {
        return s.to_string();
    }
    format!("\x1b[{}m{}\x1b[0m", code, s)
}
fn green(s: &str) -> String  { paint("32", s, out_color()) }
fn yellow(s: &str) -> String { paint("33", s, out_color()) }
fn cyan(s: &str) -> String   { paint("36", s, out_color()) }
fn bold(s: &str) -> String   { paint("1", s, out_color()) }
fn err_red(s: &str) -> String    { paint("31", s, err_color()) }
fn err_yellow(s: &str) -> String { paint("33", s, err_color()) }

// goos/goarch 映射（std::env::consts 到 Go 的 runtime.GOOS/GOARCH 命名，对齐产物命名）。
fn goos() -> &'static str {
    match std::env::consts::OS {
        "macos" => "darwin",
        other => other,
    }
}
fn goarch() -> &'static str {
    match std::env::consts::ARCH {
        "x86_64" => "amd64",
        "x86" => "386",
        "aarch64" => "arm64",
        other => other,
    }
}

fn proxy_arg(proxy: &str) -> String {
    if proxy.is_empty() {
        String::new()
    } else {
        format!(" --proxy {}", proxy)
    }
}

// retry_with_sudo：安装目录不可写时，TTY 下自动 sudo <exe> update <原参数> 提权重试。
fn retry_with_sudo(exe_real: &str, args: &[String]) -> Option<i32> {
    if !std::io::stdin().is_terminal() {
        return None;
    }
    if is_root() {
        return None;
    }
    let mut joined = String::new();
    for a in args {
        if !joined.is_empty() {
            joined.push(' ');
        }
        joined.push_str(a);
    }
    println!("{}", yellow(&format!(
        "安装目录不可写，将以 sudo 提权重试（{} {})", exe_real, joined)));
    let status = std::process::Command::new("sudo")
        .arg(exe_real)
        .args(args)
        .status();
    match status {
        Ok(s) => s.code(),
        Err(_) => {
            eprintln!("error: 无法执行 sudo");
            Some(1)
        }
    }
}

fn is_root() -> bool {
    if cfg!(unix) {
        let out = std::process::Command::new("id").arg("-u").output();
        if let Ok(o) = out {
            if let Ok(s) = String::from_utf8(o.stdout) {
                return s.trim() == "0";
            }
        }
    }
    false
}

// cmd_update：自更新（与 Go 版 cmdUpdate 行为对齐）。
fn cmd_update(args: &[String]) -> ExitCode {
    let mut check_only = false;
    let mut force = false;
    let mut insecure = false;
    let mut proxy = String::new();
    let mut i = 0usize;
    while i < args.len() {
        match args[i].as_str() {
            "--check" => check_only = true,
            "--force" => force = true,
            "--insecure" => insecure = true,
            "--proxy" => {
                if i + 1 >= args.len() {
                    eprintln!("--proxy 需要代理地址参数");
                    return ExitCode::from(2);
                }
                i += 1;
                proxy = args[i].clone();
            }
            other => {
                eprintln!("未知参数 {:?}", other);
                return ExitCode::from(2);
            }
        }
        i += 1;
    }
    let p = if proxy.is_empty() {
        None
    } else {
        match parse_proxy_arg(&proxy) {
            Ok(p) => Some(p),
            Err(e) => {
                eprintln!("error: {}", e);
                return ExitCode::from(1);
            }
        }
    };
    let mut u = Updater::new(p);
    u.insecure = insecure;
    if insecure {
        eprintln!("{}", err_yellow("警告：--insecure 已跳过产物签名验证（仅建议自建测试源使用）"));
    }
    let rel = match u.latest() {
        Ok(r) => r,
        Err(e) => {
            eprintln!("error: {}（可用 --proxy socks5://host:port 走代理）", e);
            return ExitCode::from(1);
        }
    };
    if !u.is_outdated(&rel.tag_name) && !force {
        println!("当前已是最新版本 {}", fly_lang::version::string());
        return ExitCode::SUCCESS;
    }
    if check_only {
        println!("发现新版本 {}（当前 {}）", rel.tag_name, fly_lang::version::string());
        return ExitCode::from(2);
    }
    let asset = match u.asset_for(goos(), goarch(), &rel) {
        Ok(a) => a,
        Err(e) => {
            eprintln!("error: {}", e);
            return ExitCode::from(1);
        }
    };
    let exe = match u.executable() {
        Ok(e) => e,
        Err(e) => {
            eprintln!("error: {}", e);
            return ExitCode::from(1);
        }
    };
    let exe_real = std::fs::canonicalize(&exe).unwrap_or(exe.clone());
    let install_dir = exe_real.parent().map(|p| p.to_path_buf()).unwrap_or_default();
    if let Err(e) = u.check_writable(&install_dir) {
        let sudo_args: Vec<String> = std::env::args().skip(1).collect();
        if let Some(code) = retry_with_sudo(exe_real.to_string_lossy().as_ref(), &sudo_args) {
            return ExitCode::from(code as u8);
        }
        eprintln!("{}", err_red(&e));
        eprintln!("{}", err_yellow(&format!(
            "建议：sudo {} update{} 重试（或把 fly 安装到用户可写目录）",
            exe.display(),
            proxy_arg(&proxy))));
        return ExitCode::from(1);
    }

    println!("{}", yellow(&format!(
        "发现新版本 {}（当前 {}）", rel.tag_name, fly_lang::version::string())));
    if !rel.body.trim().is_empty() {
        println!("{}", cyan("更新内容："));
        for line in rel.body.lines() {
            let line = line.trim();
            if !line.is_empty() {
                println!("  {}", line);
            }
        }
        println!();
    }
    print!("是否安装？[Y/n] ");
    std::io::Write::flush(&mut std::io::stdout()).ok();
    if !fly_lang::update::confirm() {
        println!("{}", green("bye"));
        return ExitCode::SUCCESS;
    }
    println!("{}", green("开始安装..."));
    let mut log = |step: &str| println!("{}", yellow(&format!("  {}", step)));
    if let Err(e) = u.install(&asset, &mut log) {
        eprintln!("error: {}", err_red(&e));
        return ExitCode::from(1);
    }
    println!("{}", green(&bold(&format!(
        "已更新到 {}，请重启后生效", rel.tag_name))));
    ExitCode::SUCCESS
}

fn parse_proxy_arg(proxy: &str) -> Result<Proxy, String> {
    if proxy.starts_with("http://") || proxy.starts_with("https://") {
        Ok(Proxy::Http(proxy.to_string()))
    } else if proxy.starts_with("socks5://") || proxy.starts_with("socks5h://") {
        Ok(Proxy::Socks5(proxy.to_string()))
    } else {
        Err(format!(
            "不支持的代理协议 {:?}（支持 http://、https://、socks5://）",
            proxy
        ))
    }
}

struct CheckOutcome {
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
    let color = err_color();

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
                        blocks,
                    }
                }
                Err(e) => CheckOutcome {
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
fn cmd_lsp(args: &[String]) -> ExitCode {
    if !args.is_empty() {
        eprintln!("用法: fly lsp（stdio JSON-RPC，供编辑器 LSP 客户端调用）");
        return ExitCode::from(2);
    }
    let server = match fly_lang::lsp::Server::new() {
        Ok(s) => std::sync::Arc::new(s),
        Err(e) => {
            eprintln!("error: {}", e);
            return ExitCode::from(1);
        }
    };
    let mut stdin = std::io::stdin();
    match server.run(&mut stdin) {
        Ok(()) => ExitCode::SUCCESS,
        Err(e) => {
            eprintln!("lsp: {}", e);
            ExitCode::from(1)
        }
    }
}
