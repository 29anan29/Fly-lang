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
        Some("fmt") => cmd_fmt(&args[1..]),
        Some("analyze") => cmd_analyze(&args[1..]),
        Some("help") | Some("-h") | Some("--help") => {
            println!("{}", USAGE);
            ExitCode::SUCCESS
        }
        Some("lsp") => cmd_lsp(&args[1..]),
        Some("sandbox") => cmd_sandbox(&args[1..]),
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
  fly fmt [选项] <file.fly>...  格式化代码（token 级空白重排，注释/语义不变）
  fly analyze <file.fly>|<dir> 代码质量报告（复杂度/嵌套/重复/注释比例等）
  fly version                  显示版本
  fly error <E码>              查询错误码（示例报错与修复方法）
  fly update [选项]             检查/更新到最新版本

build 选项:
  -o <out.py>   指定输出文件（默认输出到 build/ 目录，保留相对路径）

fmt 选项:
  -w, --write   写回文件（默认输出 stdout）
  --check       只报告需要格式化的文件（CI 用，有差异退出码 1）

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

// cmd_sandbox：`fly sandbox` 桥接 fly-sandboxd（Go 保留组件，方案 B：安全关键代码不重写）。
// 发现顺序：FLY_SANDBOXD 环境变量 → 同目录 fly-sandboxd → PATH。
fn find_sandboxd() -> Option<PathBuf> {
    if let Ok(p) = std::env::var("FLY_SANDBOXD") {
        if !p.is_empty() {
            return Some(PathBuf::from(p));
        }
    }
    if let Ok(exe) = std::env::current_exe() {
        if let Some(dir) = exe.parent() {
            let cand = dir.join("fly-sandboxd");
            if cand.exists() {
                return Some(cand);
            }
        }
    }
    if let Ok(path) = std::env::var("PATH") {
        for dir in path.split(':') {
            let cand = PathBuf::from(dir).join("fly-sandboxd");
            if cand.exists() {
                return Some(cand);
            }
        }
    }
    None
}

fn cmd_sandbox(args: &[String]) -> ExitCode {
    let Some(sandboxd) = find_sandboxd() else {
        eprintln!("error: 找不到 fly-sandboxd（设置 FLY_SANDBOXD 环境变量指定路径）");
        return ExitCode::from(1);
    };
    let status = std::process::Command::new(&sandboxd)
        .args(args)
        .status();
    match status {
        Ok(s) => ExitCode::from(s.code().unwrap_or(1) as u8),
        Err(e) => {
            eprintln!("error: 启动 fly-sandboxd 失败: {}", e);
            ExitCode::from(1)
        }
    }
}

// cmd_fmt 格式化 .fly 文件：-w 写回（默认输出 stdout），--check 只报告差异（CI 用）。
// 前置 check 必须通过（语法错误文件跳过），与 Go 版行为一致。
fn cmd_fmt(args: &[String]) -> ExitCode {
    let mut write = false;
    let mut check_only = false;
    let mut files: Vec<String> = Vec::new();
    for a in args {
        match a.as_str() {
            "-w" | "--write" => write = true,
            "--check" => check_only = true,
            "-h" | "--help" => {
                eprintln!("用法: fly fmt [-w|--check] <file.fly>...（支持目录，递归查找 .fly）");
                return ExitCode::SUCCESS;
            }
            _ => files.push(a.clone()),
        }
    }
    if files.is_empty() {
        eprintln!("用法: fly fmt [-w|--check] <file.fly>...（支持目录，递归查找 .fly）");
        return ExitCode::from(2);
    }
    let mut paths: Vec<String> = Vec::new();
    for a in &files {
        match std::fs::metadata(a) {
            Err(e) => {
                eprintln!("error: {}", e);
                return ExitCode::from(1);
            }
            Ok(m) => {
                if m.is_dir() {
                    match walk_fly(Path::new(a), &mut paths) {
                        Ok(()) => {}
                        Err(e) => {
                            eprintln!("error: {}", e);
                            return ExitCode::from(1);
                        }
                    }
                } else {
                    paths.push(a.clone());
                }
            }
        }
    }
    let checkd = match checkd::find_checkd() {
        Some(c) => c,
        None => {
            eprintln!("error: 找不到 fly-checkd（设置 FLY_CHECKD 环境变量指定路径）");
            return ExitCode::from(1);
        }
    };
    let mut dirty = 0usize;
    for p in &paths {
        let src = match std::fs::read_to_string(p) {
            Ok(s) => s,
            Err(e) => {
                eprintln!("error: {}", e);
                return ExitCode::from(1);
            }
        };
        let has_err = match checkd::check_src(&checkd, &src, p, false) {
            Ok(r) => !r.diags.is_empty(),
            Err(_) => true,
        };
        if has_err {
            eprintln!("fmt: 跳过 {}（存在编译错误，格式化前请先修复）", p);
            dirty += 1;
            continue;
        }
        let out = fly_lang::fmt::format(&src);
        if out == src {
            continue;
        }
        dirty += 1;
        if check_only {
            println!("需要格式化: {}", p);
            continue;
        }
        if write {
            if let Err(e) = std::fs::write(p, out) {
                eprintln!("error: 写入 {} 失败: {}", p, e);
                return ExitCode::from(1);
            }
            println!("ok: {}", p);
        } else {
            print!("--- {} ---\n{}", p, out);
        }
    }
    if check_only && dirty > 0 {
        return ExitCode::from(1);
    }
    ExitCode::SUCCESS
}

// cmd_analyze 输出代码质量报告：循环复杂度/认知复杂度/嵌套/函数长度/参数/
// 重复/错误处理/注释比例/命名规范，100 制评分（与 Go 版报告口径一致）。
#[derive(Clone)]
struct Rep {
    path: String,
    met: fly_lang::analyze::Metrics,
    bad: f64,
}

fn cmd_analyze(args: &[String]) -> ExitCode {
    let mut files: Vec<String> = Vec::new();
    for a in args {
        match a.as_str() {
            "-h" | "--help" => {
                eprintln!("用法: fly analyze <file.fly>|<dir>...（支持目录，递归查找 .fly）");
                return ExitCode::SUCCESS;
            }
            _ => files.push(a.clone()),
        }
    }
    if files.is_empty() {
        eprintln!("用法: fly analyze <file.fly>|<dir>...（支持目录，递归查找 .fly）");
        return ExitCode::from(2);
    }
    let mut paths: Vec<String> = Vec::new();
    for a in &files {
        match std::fs::metadata(a) {
            Err(e) => {
                eprintln!("error: {}", e);
                return ExitCode::from(1);
            }
            Ok(m) => {
                if m.is_dir() {
                    match walk_fly(Path::new(a), &mut paths) {
                        Ok(()) => {}
                        Err(e) => {
                            eprintln!("error: {}", e);
                            return ExitCode::from(1);
                        }
                    }
                } else {
                    paths.push(a.clone());
                }
            }
        }
    }
    if paths.is_empty() {
        eprintln!("error: 未找到 .fly 文件");
        return ExitCode::from(1);
    }
    let mut reps: Vec<Rep> = Vec::new();
    let mut total = 0.0f64;
    let mut t_score = 0.0f64;
    let mut worst: Vec<Rep> = Vec::new();
    for p in &paths {
        let src = match std::fs::read_to_string(p) {
            Ok(s) => s,
            Err(e) => {
                eprintln!("error: {}", e);
                return ExitCode::from(1);
            }
        };
        let Some(met) = fly_lang::analyze::analyze(&src) else {
            eprintln!("analyze: 跳过 {}（语法错误）", p);
            continue;
        };
        let (s, b) = fly_lang::analyze::score(&met);
        let r = Rep {
            path: p.clone(),
            met,
            bad: b,
        };
        reps.push(r.clone());
        total += 1.0;
        t_score += s;
        if worst.len() < 5 {
            worst.push(r.clone());
            let mut i = worst.len() - 1;
            while i > 0 && worst[i].bad > worst[i - 1].bad {
                worst.swap(i, i - 1);
                i -= 1;
            }
        } else {
            for i in 0..worst.len() {
                if r.bad > worst[i].bad {
                    worst.insert(i, r.clone());
                    worst.pop();
                    break;
                }
            }
        }
    }
    if reps.is_empty() {
        eprintln!("error: 没有可分析的文件");
        return ExitCode::from(1);
    }
    let avg = t_score / total;
    println!("🌸 屎山代码分析报告 🌸\n");
    println!("  总体评分: {:.2} / 100 - {}", avg, fly_lang::analyze::level(avg));
    println!("  已分析 {} 个文件\n", reps.len());
    println!("◆ 评分指标详情（平均分项）");
    let am = aggregate(&reps);
    println!(
        "  ✓✓ 循环复杂度    {:.1}%  平均 {}（目标 ≤ 10）",
        rate_of(am.cyclomatic as f64, 10.0),
        am.cyclomatic
    );
    println!(
        "  ✓✓ 认知复杂度    {:.1}%  平均 {}（目标 ≤ 15）",
        rate_of(am.cognitive as f64, 15.0),
        am.cognitive
    );
    println!(
        "  ✓✓ 嵌套深度      {:.1}%  最大 {}（目标 ≤ 4）",
        rate_of(am.max_nest as f64, 4.0),
        am.max_nest
    );
    println!(
        "  ✓✓ 函数长度      {:.1}%  最长 {} 行（目标 ≤ 50）",
        rate_of(am.max_func_len as f64, 50.0),
        am.max_func_len
    );
    println!(
        "  ✓✓ 文件长度      {:.1}%  平均 {} 行（目标 ≤ 500）",
        rate_of(am.lines as f64, 500.0),
        am.lines
    );
    println!(
        "  ✓✓ 参数数量      {:.1}%  最多 {} 个（目标 ≤ 5）",
        rate_of(am.max_params as f64, 5.0),
        am.max_params
    );
    println!(
        "  ✓✓ 代码重复      {:.1}%  重复 {:.1}%",
        am.repeat_rate * 100.0,
        am.repeat_rate * 100.0
    );
    println!(
        "  ✓✓ 错误处理      {:.1}%  try {} / raise {}",
        rate_of(am.try_count as f64, (am.func_count + 1) as f64),
        am.try_count,
        am.raise_count
    );
    println!(
        "  ✓✓ 注释比例      {:.1}%  注释行 {:.1}%",
        100.0 - am.comment_rate * 100.0,
        am.comment_rate * 100.0
    );
    println!(
        "  ✓✓ 命名规范      {:.1}%  非 snake_case {:.1}%",
        100.0 - am.name_rate * 100.0,
        am.name_rate * 100.0
    );
    println!("\n◆ 最屎代码排行榜（糟糕指数前 5）");
    for (i, r) in worst.iter().enumerate() {
        println!("  {}. {:<55} (糟糕指数: {:.2})", i + 1, r.path, r.bad);
        for f in &r.met.functions {
            if f.complex {
                println!(
                    "     🔄 {}() L{}: 循环复杂度 {} 认知 {} 嵌套 {} 长度 {}",
                    f.name, f.line, f.cyclo, f.cognit, f.nest, f.length
                );
            }
        }
    }
    println!("\n◆ 诊断结论\n  🌸 {}", fly_lang::analyze::level(avg));
    ExitCode::SUCCESS
}

// aggregate 汇总所有文件指标均值。
fn aggregate(reps: &[Rep]) -> fly_lang::analyze::Metrics {
    let n = reps.len() as f64;
    let mut m = fly_lang::analyze::Metrics::default();
    for r in reps {
        m.cyclomatic += r.met.cyclomatic;
        m.cognitive += r.met.cognitive;
        m.max_nest += r.met.max_nest;
        m.max_func_len += r.met.max_func_len;
        m.lines += r.met.lines;
        m.max_params += r.met.max_params;
        m.try_count += r.met.try_count;
        m.raise_count += r.met.raise_count;
        m.repeat_rate += r.met.repeat_rate;
        m.comment_rate += r.met.comment_rate;
        m.name_rate += r.met.name_rate;
    }
    let div = |v: i64| (v as f64 / n) as i64;
    m.cyclomatic = div(m.cyclomatic);
    m.cognitive = div(m.cognitive);
    m.max_nest = div(m.max_nest);
    m.max_func_len = div(m.max_func_len);
    m.lines = div(m.lines);
    m.max_params = div(m.max_params);
    m.try_count = div(m.try_count);
    m.raise_count = div(m.raise_count);
    m.repeat_rate /= n;
    m.comment_rate /= n;
    m.name_rate /= n;
    m
}

fn rate_of(v: f64, target: f64) -> f64 {
    if v >= target {
        return 0.0;
    }
    (1.0 - v / target) * 100.0
}
