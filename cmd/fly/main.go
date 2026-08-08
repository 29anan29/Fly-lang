package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"flylang/internal/compile"
	"flylang/internal/lsp"
	"flylang/internal/update"
	"flylang/internal/version"
)

func cmdLSP(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "用法: fly lsp（stdio JSON-RPC，供编辑器 LSP 客户端调用）")
		return 2
	}
	if err := lsp.New().Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "lsp: %v\n", err)
		return 1
	}
	return 0
}

const usage = `Fly-Lang 编译器

用法:
  fly build [选项] <file.fly>   转译为 Python
  fly check <file.fly>          仅编译检查
  fly run <file.fly>            转译并执行
  fly version                  显示版本
  fly update [选项]             检查/更新到最新版本

build 选项:
  -o <out.py>   指定输出文件（默认与源文件同名 .py）

update 选项:
  --check       仅检查新版本（有新版退出码 2）
  --force       同版本也强制更新
  --proxy <url> 走代理（http://、https://、socks5://）`

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "build":
		return cmdBuild(args[1:])
	case "check":
		return cmdCheck(args[1:])
	case "run":
		return cmdRun(args[1:])
	case "version":
		fmt.Println(version.String())
		return 0
	case "update":
		return cmdUpdate(args[1:])
	case "lsp":
		return cmdLSP(args[1:])
	case "-h", "--help", "help":
		fmt.Println(usage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "未知子命令 %q\n\n%s\n", args[0], usage)
	return 2
}

func cmdCheck(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "用法: fly check <file.fly>")
		return 2
	}
	path := args[0]
	errs, err := compile.CheckFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	for _, d := range errs {
		fmt.Fprintln(os.Stderr, compile.FormatError(path, d))
	}
	if len(errs) > 0 {
		return 1
	}
	return 0
}

func cmdBuild(args []string) int {
	out := ""
	var file string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "-o 需要文件名参数")
				return 2
			}
			i++
			out = args[i]
		default:
			if file != "" {
				fmt.Fprintln(os.Stderr, "只能指定一个输入文件")
				return 2
			}
			file = args[i]
		}
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, "用法: fly build [-o out.py] <file.fly>")
		return 2
	}
	code, errs, err := compile.BuildFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	for _, d := range errs {
		fmt.Fprintln(os.Stderr, compile.FormatError(file, d))
	}
	if len(errs) > 0 {
		return 1
	}
	if out == "" {
		base := strings.TrimSuffix(filepath.Base(file), ".fly")
		out = filepath.Join(filepath.Dir(file), base+".py")
	}
	if err := os.WriteFile(out, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: 写入 %s 失败: %v\n", out, err)
		return 1
	}
	fmt.Printf("ok: %s -> %s\n", file, out)
	return 0
}

func cmdUpdate(args []string) int {
	checkOnly := false
	force := false
	proxy := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			checkOnly = true
		case "--force":
			force = true
		case "--proxy":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "--proxy 需要代理地址参数")
				return 2
			}
			i++
			proxy = args[i]
		default:
			fmt.Fprintf(os.Stderr, "未知参数 %q\n", args[i])
			return 2
		}
	}
	u := update.New()
	if proxy != "" {
		if err := u.SetProxy(proxy); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	rel, err := u.Latest()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v（可用 --proxy socks5://host:port 走代理）\n", err)
		return 1
	}
	if !u.IsOutdated(rel.TagName) && !force {
		fmt.Printf("当前已是最新版本 %s\n", version.String())
		return 0
	}
	if checkOnly {
		fmt.Printf("发现新版本 %s（当前 %s）\n", rel.TagName, version.String())
		return 2
	}
	asset, err := u.AssetFor(runtime.GOOS, runtime.GOARCH, rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	exe, err := u.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	exeReal, err := filepath.EvalSymlinks(exe)
	if err != nil {
		exeReal = exe
	}
	installDir := filepath.Dir(exeReal)
	if err := u.CheckWritable(installDir); err != nil {
		fmt.Fprintln(os.Stderr, red(err.Error()))
		fmt.Fprintln(os.Stderr, yellow(fmt.Sprintf("建议：sudo %s update%s 重试（或把 fly 安装到用户可写目录）",
			exe, proxyArg(proxy))))
		return 1
	}

	fmt.Println(yellow(fmt.Sprintf("发现新版本 %s（当前 %s）", rel.TagName, version.String())))
	if strings.TrimSpace(rel.Body) != "" {
		fmt.Println(cyan("更新内容："))
		for _, line := range strings.Split(rel.Body, "\n") {
			line = strings.TrimSpace(line)
			if line != "" {
				fmt.Println("  " + line)
			}
		}
		fmt.Println()
	}
	fmt.Print("是否安装？[Y/n] ")
	yes, err := update.Confirm(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	if !yes {
		fmt.Println(green("bye"))
		return 0
	}
	fmt.Println(green("开始安装..."))
	if err := u.InstallVerbose(asset, func(step string) {
		fmt.Println(yellow("  " + step))
	}); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", red(err.Error()))
		return 1
	}
	fmt.Println(green(bold(fmt.Sprintf("已更新到 %s，请重启后生效", rel.TagName))))
	return 0
}

func proxyArg(proxy string) string {
	if proxy == "" {
		return ""
	}
	return " --proxy " + proxy
}

const (
	ansiRed    = "31"
	ansiGreen  = "32"
	ansiYellow = "33"
	ansiCyan   = "36"
	ansiBold   = "1"
)

var colorOn = isTTY(os.Stdout) && os.Getenv("NO_COLOR") == ""

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

func paint(code, s string) string {
	if !colorOn {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func red(s string) string    { return paint(ansiRed, s) }
func green(s string) string  { return paint(ansiGreen, s) }
func yellow(s string) string { return paint(ansiYellow, s) }
func cyan(s string) string   { return paint(ansiCyan, s) }
func bold(s string) string   { return paint(ansiBold, s) }

func cmdRun(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "用法: fly run <file.fly>")
		return 2
	}
	file := args[0]
	code, errs, err := compile.BuildFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	for _, d := range errs {
		fmt.Fprintln(os.Stderr, compile.FormatError(file, d))
	}
	if len(errs) > 0 {
		return 1
	}
	tmp, err := os.CreateTemp("", "fly-*.py")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)
	if _, err := tmp.WriteString(code); err != nil {
		tmp.Close()
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	tmp.Close()
	cmd := exec.Command("python3", tmpPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "error: 无法执行 python3: %v\n", err)
		return 1
	}
	return 0
}
