// help.rs：`fly help <指令名>` 全面指令教程（每个子命令：用途/用法/选项/示例/注意事项）。
// 教程文本与各 cmd_* 实现保持同步；改命令选项时同步更新此处。

pub const HELP_HINT: &str = "PyFly 指令教程

用法:
  fly help <指令名>   查看子命令详细教程（build/check/run/sandbox/fmt/analyze/
                      version/error/update/lsp/help）
  fly help            查看本总览
  fly <指令名>        直接运行（各指令也支持 -h/--help 查看简要用法）

可用指令:
  build     转译 .fly 为 Python（含沙箱运行时注入）
  check     编译检查（支持目录递归与并发）
  run       转译并在沙箱内执行
  sandbox   进程级沙箱运行任意 Python
  fmt       格式化代码（token 级空白重排）
  analyze   代码质量报告（100 制评分）
  version   显示版本
  error     查询错误码（示例报错与修复方法）
  update    检查/更新到最新版本
  lsp       语言服务器（stdio JSON-RPC，供编辑器调用）
";

const HELP_BUILD: &str = "fly build —— 转译 .fly 为 Python（含沙箱运行时注入）

用途:
  把 Fly（Python 安全超集）源码转译为纯 Python 3.10+ 产物。
  产物恒注入 runtime + sandbox 两个运行时节（8 个安全关键字 + 沙箱代理），
  编译期未拦住的逃逸由运行时兜底。

用法:
  fly build [-o out.py] [--keep-annotations] <file.fly>

选项:
  -o <out.py>           指定输出文件；默认输出到 build/ 目录并保留相对路径
  --keep-annotations    保留关键字审计注释（# fly-safe: 等），默认剥离

示例:
  fly build app.fly                  # 输出 build/app.fly → build/app.py
  fly build app.fly -o app.py        # 指定输出路径
  python3 app.py                     # 直接运行产物

说明:
  - 编译错误输出 Rust 风格诊断: error[EXXXX]: 标题 + --> file:line:col
    + 源码行下划线高亮 + = help: 修复建议 + = note: 文档链接
  - 产物行为可用 python3 实跑验证（AGENTS.md 验证约定）
  - 退出码: 0 成功 / 1 编译错误 / 2 参数错误
";

const HELP_CHECK: &str = "fly check —— 编译检查（与编辑器诊断同一管线）

用途:
  只检查不输出产物。checker（Go 守护进程 fly-checkd）执行与 LSP 诊断
  完全相同的编译管线——改 checker 行为自动同步编辑器诊断。

用法:
  fly check <file.fly>... | <目录>...

选项:
  （无选项；多个文件或目录按输入顺序并发检查）

示例:
  fly check src/                          # 递归检查目录下全部 .fly
  fly check a.fly b.fly                   # 多文件并发
  fly check testdata/golden/              # CI 门禁常用

说明:
  - 目录递归查找 .fly，线程并发（信号量 = 可用核心 × 2）
  - 任一文件失败退出码 1
  - 找不到 fly-checkd 时设置 FLY_CHECKD 环境变量指定路径
";

const HELP_RUN: &str = "fly run —— 转译并在沙箱内执行

用途:
  build + 沙箱执行的组合：先编译检查，通过后在进程级沙箱内运行产物。

用法:
  fly run <file.fly>

示例:
  fly run hello.fly

说明:
  - 依赖同目录/FLY_CHECKD/PATH 中的 fly-checkd（编译检查）与 fly-sandboxd（沙箱）
  - 沙箱机制随平台: Linux=ns+Landlock+seccomp / macOS=Seatbelt /
    Windows=Job Object（见 fly help sandbox）
  - 退出码透传沙箱内 python 进程的退出码
";

const HELP_SANDBOX: &str = "fly sandbox —— 进程级沙箱运行任意 Python

用途:
  把不受信任的 Python 脚本（不一定是 Fly 产物）放进 OS 层进程沙箱执行，
  作为编译期拦截之外的第二道纵深防线。

用法:
  fly sandbox <script.py> [选项]

选项:
  --cap-fs-read <path>   只读访问白名单（可重复，默认仅系统库+脚本）
  --cap-net-host <host>  网络 host 白名单（预留，当前网络一律禁用）
  --mem-limit-mb <n>     内存上限 MB（默认 512）
  --timeout-ms <n>       墙钟超时毫秒（默认 5000）
  --cpu-sec <n>          CPU 时间上限秒（默认 10）
  --nofile <n>           文件描述符上限（默认 64，Linux）
  --no-audit             关闭审计日志（默认开启，JSON 行输出到 stderr）
  --debug-ns             跳过命名空间（仅 Linux，调试隔离层用）

示例:
  fly sandbox script.py
  fly sandbox script.py --mem-limit-mb 128 --timeout-ms 3000
  fly sandbox script.py --cap-fs-read /etc/hosts

说明: （平台机制与强度声明）
  Linux      clone ns + Landlock + seccomp + rlimit——最强（内核级隔离）
  macOS      Seatbelt（sandbox-exec SBPL）：文件写仅限临时/用户/当前目录、
             网络全禁、内存(RLIMIT_AS)/CPU 上限——用户态策略
  Windows    Job Object：进程树强制终止 + 总内存/墙钟/进程数上限——用户态限制
  （各平台均为进程级隔离，不防御内核漏洞；审计日志前缀 [fly-sandbox]）
";

const HELP_FMT: &str = "fly fmt —— 格式化代码（token 级空白重排）

用途:
  统一代码风格。token 流级重排：注释保留、语义不变；格式化前先编译检查，
  语法错误文件跳过（如 testdata/errors/ 反例）。

用法:
  fly fmt [-w|--write] [--check] <file.fly>... | <目录>...

选项:
  -w, --write   写回文件（默认输出到 stdout）
  --check       只报告需要格式化的文件（CI 用，有差异退出码 1）

示例:
  fly fmt src/                       # 全部 .fly 格式化为 stdout
  fly fmt -w src/                    # 写回
  fly fmt --check testdata/golden/   # CI 门禁：不干净即失败

说明:
  - 改行号会影响产物内嵌行列号——fmt 后需重新生成 golden 快照
    （fly build -o testdata/golden/x.py testdata/golden/x.fly）
  - 退出码: 0 干净 / 1 存在差异（--check）/ 2 参数错误
";

const HELP_ANALYZE: &str = "fly analyze —— 代码质量报告（100 制评分）

用途:
  输出代码质量评分与维度明细，用于 CI 质量门禁或代码评审。

用法:
  fly analyze <file.fly> | <目录>...

评分维度:
  McCabe 循环复杂度 / 认知复杂度 / 嵌套深度 / 重复代码 / 注释比例 / 命名规范

示例:
  fly analyze app.fly
  fly analyze src/

说明:
  - 支持目录递归；输出 100 制总分 + 各维度明细
";

const HELP_VERSION: &str = "fly version —— 显示版本

用途:
  显示当前版本与构建类型。用于排查升级/兼容问题（如 VSCode 插件
  要求 fly 含 lsp 子命令，旧二进制连接会失败）。

用法:
  fly version

输出示例:
  v0.6.6 (release)     正式版（版本号注入自构建时环境变量）
  0.6.7-dev (abc1234)  dev 版（未打 tag 的本地构建）

说明:
  - fly update 只检查 GitHub 最新正式版（dev 版同样可更新）
";

const HELP_ERROR: &str = "fly error —— 查询错误码（示例报错与修复方法）

用途:
  查询任意错误码的示例报错与修复建议，无需翻文档。

用法:
  fly error <E码>

E码格式:
  E0031 / 31 均支持（自动补零到 EXXXX 四位）

示例:
  fly error E0063        # 查询危险内建拦截错误
  fly error 31           # 等价于 fly error E0031

说明:
  - 全部错误码见 docs/报错清单.md（E0001 起连续编号）
  - 错误码注册表: internal/ast/errors.go（Go）↔ src/errorinfo.rs（Rust）
    双真源，改任一须同步另一处（scripts/check-errorinfo.sh 校验）
";

const HELP_UPDATE: &str = "fly update —— 检查/更新到最新版本

用途:
  从 GitHub 官方 Release 下载最新正式版并自替换（含 fly-checkd/fly-sandboxd
  三二进制，安装包按名解包）。

用法:
  fly update [选项]

选项:
  --check           仅检查新版本（有新版本退出码 2，便于 CI 判断）
  --force           同版本也强制更新
  --insecure        跳过产物验签（默认强制验签：ed25519 公钥内嵌，
                    缺 .sig 拒绝安装——仅测试源使用）
  --proxy <url>     走代理: http://、https://、socks5://[user:pass@]host:port

示例:
  fly update
  fly update --check
  fly update --proxy socks5://127.0.0.1:1080

说明:
  - 不可写时终端（TTY）自动 sudo 提权重试，非 TTY 提示手动 sudo
  - 校验链: GitHub Release 产物 .sig（ed25519）→ 内嵌公钥验签 → 替换
";

const HELP_LSP: &str = "fly lsp —— 语言服务器（stdio JSON-RPC）

用途:
  供编辑器 LSP 客户端调用（VSCode 插件 29anan29.pyfly-lang 内置连接）。
  提供诊断（publishDiagnostics）、hover（8 个关键字文档）等。

用法:
  fly lsp
  （无参数；stdio 帧协议 Content-Length，JSON-RPC 2.0）

支持的通知/请求:
  initialize / initialized / shutdown / exit
  didOpen / didChange(full) / didSave / didClose
  自定义通知: fly/forceCheck

说明:
  - 诊断由 fly-checkd（Go 编译管线 CheckSource）驱动，与 fly check 同一管线
  - 行号转换: 诊断 Line/Col（1 基）→ LSP 0 基；severity 恒为 1（Error）
  - 不需要用户手动运行——编辑器自动拉起（fly.path 配置指定二进制）
";

const HELP_HELP: &str = "fly help —— 指令教程入口

用途:
  查看任一子命令的全面教程（用途/用法/选项/示例/注意事项）。

用法:
  fly help <指令名>

示例:
  fly help build
  fly help sandbox
  fly help update

说明:
  - 无参数时显示指令总览
  - 与 fly <指令> -h/--help 的简要用法提示互补（教程更全面）
";

pub fn topic(name: &str) -> Option<&'static str> {
    match name {
        "build" => Some(HELP_BUILD),
        "check" => Some(HELP_CHECK),
        "run" => Some(HELP_RUN),
        "sandbox" => Some(HELP_SANDBOX),
        "fmt" => Some(HELP_FMT),
        "analyze" => Some(HELP_ANALYZE),
        "version" => Some(HELP_VERSION),
        "error" => Some(HELP_ERROR),
        "update" => Some(HELP_UPDATE),
        "lsp" => Some(HELP_LSP),
        "help" => Some(HELP_HELP),
        _ => None,
    }
}

pub const COMMANDS: &[&str] = &[
    "build", "check", "run", "sandbox", "fmt", "analyze", "version", "error", "update", "lsp", "help",
];

#[cfg(test)]
mod tests {
    use super::*;

    // 每个指令都应有教程；教程必须包含 用法/选项/说明 三个要点。
    #[test]
    fn every_command_has_full_tutorial() {
        assert!(!COMMANDS.is_empty());
        for c in COMMANDS {
            let t = topic(c).unwrap_or_else(|| panic!("{} 缺少教程", c));
            assert!(t.contains("用法:"), "{} 教程缺用法", c);
            assert!(t.contains("说明:"), "{} 教程缺说明", c);
        }
    }

    // 教程内容与实际命令行为锚点对齐（选项名抽查）。
    #[test]
    fn tutorials_cover_key_options() {
        assert!(topic("build").unwrap().contains("--keep-annotations"));
        assert!(topic("sandbox").unwrap().contains("--mem-limit-mb"));
        assert!(topic("sandbox").unwrap().contains("--timeout-ms"));
        assert!(topic("update").unwrap().contains("--insecure"));
        assert!(topic("update").unwrap().contains("--proxy"));
        assert!(topic("fmt").unwrap().contains("--check"));
        assert!(topic("error").unwrap().contains("E0031"));
        assert!(topic("lsp").unwrap().contains("fly/forceCheck"));
    }
}