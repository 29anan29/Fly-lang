// main.go：fly CLI 入口。build/run/sandbox/update/lsp 走 Go 实现；check/version/error 由 Rust 版接棒。
package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"flylang/internal/analyze"
	"flylang/internal/ast"
	"flylang/internal/compile"
	"flylang/internal/format"
	"flylang/internal/gen"
	"flylang/internal/lsp"
	"flylang/internal/update"
	"flylang/internal/version"
)

func cmdError(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "用法: fly error <E码>（如 fly error E0031）")
		return 2
	}
	code := strings.ToUpper(args[0])
	if !strings.HasPrefix(code, "E") {
		code = "E" + code
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(code, "E")); err == nil {
		code = fmt.Sprintf("E%04d", n)
	}
	info, ok := ast.InfoForCode(code)
	if !ok {
		fmt.Fprintf(os.Stderr, "未知错误码 %s\n\n全部错误码见 docs/报错清单.md\n", code)
		return 1
	}
	fmt.Println(colorizeExample(info.Example))
	return 0
}

// colorizeExample 与 Rust 版 main.rs colorize_example 对齐：error[EXXXX] 亮红、箭头青、help 绿、note 黄。
func colorizeExample(s string) string {
	if !outOn {
		return s
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(s, "\n") {
		switch {
		case strings.HasPrefix(line, "error[E"):
			b.WriteString("\x1b[1;31m" + line + "\x1b[0m")
		case strings.Contains(line, "--> "):
			b.WriteString("\x1b[1;36m" + line + "\x1b[0m")
		case strings.HasPrefix(line, "   = help:"):
			b.WriteString("\x1b[32m" + line + "\x1b[0m")
		case strings.HasPrefix(line, "   = note:"):
			b.WriteString("\x1b[33m" + line + "\x1b[0m")
		default:
			b.WriteString(line)
		}
	}
	return b.String()
}

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
  fly build [选项] <file.fly>   转译为 Python（含沙箱运行时，拦截逃逸）
  fly check <file.fly>...       编译检查（支持目录递归，goroutine 并发）
  fly run <file.fly>            转译并在沙箱内执行
  fly sandbox <script.py>       进程级沙箱运行 Python（Landlock+seccomp+ns）
  fly version                  显示版本
  fly error <E码>              查询错误码（示例报错与修复方法）
  fly update [选项]             检查/更新到最新版本
  fly fmt [选项] <file.fly>...  格式化代码（token 级空白重排，注释/语义不变）
  fly analyze <file.fly>|<dir> 代码质量报告（复杂度/嵌套/重复/注释比例等）

build 选项:
  -o <out.py>   指定输出文件（默认输出到 build/ 目录，保留相对路径）
  --keep-annotations  产物保留安全关键字审计注释（零残留关键字的产物痕迹）

fmt 选项:
  -w            写回文件（默认输出到 stdout）
  --check       仅报告未格式化文件（有差异退出码 1，CI 用）

update 选项:
  --check       仅检查新版本（有新版退出码 2）
  --force       同版本也强制更新
  --insecure    跳过产物签名验证（危险，仅限自建测试源）
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
	case "sandbox":
		return cmdSandbox(args[1:])
	case "version":
		fmt.Println(version.String())
		return 0
	case "error":
		return cmdError(args[1:])
	case "update":
		return cmdUpdate(args[1:])
	case "lsp":
		return cmdLSP(args[1:])
	case "fmt":
		return cmdFmt(args[1:])
	case "analyze":
		return cmdAnalyze(args[1:])
	case "-h", "--help", "help":
		fmt.Println(usage)
		return 0
	}
	fmt.Fprintf(os.Stderr, "未知子命令 %q\n\n%s\n", args[0], usage)
	return 2
}

func cmdCheck(args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "用法: fly check <file.fly>...（支持目录，递归查找 .fly，并发检查）")
		return 2
	}
	var files []string
	for _, a := range args {
		info, err := os.Stat(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if info.IsDir() {
			err := filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(p, ".fly") {
					files = append(files, p)
				}
				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		} else {
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "error: 未找到 .fly 文件")
		return 1
	}
	type result struct {
		path string
		errs []ast.Diagnostic
	}
	results := make([]result, len(files))
	sem := make(chan struct{}, runtime.NumCPU()*2)
	var wg sync.WaitGroup
	for i, path := range files {
		wg.Add(1)
		go func(i int, path string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			errs, _ := compile.CheckFile(path)
			results[i] = result{path: path, errs: errs}
		}(i, path)
	}
	wg.Wait()
	failed := 0
	for _, r := range results {
		for _, d := range r.errs {
			fmt.Fprintln(os.Stderr, compile.FormatErrorColor(r.path, d, errOn))
		}
		if len(r.errs) > 0 {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d 个文件检查失败\n", failed)
		return 1
	}
	fmt.Printf("ok: %d 个文件检查通过\n", len(files))
	return 0
}

func cmdBuild(args []string) int {
	out := ""
	keepAnn := false
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
		case "--keep-annotations":
			keepAnn = true
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
	code, errs, err := compile.BuildFileOpts(file, gen.GenOpts{KeepAnnotations: keepAnn})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	for _, d := range errs {
		fmt.Fprintln(os.Stderr, compile.FormatErrorColor(file, d, errOn))
	}
	if len(errs) > 0 {
		return 1
	}
	if out == "" {
		out = defaultOutPath(file)
	}
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: 创建目录 %s 失败: %v\n", filepath.Dir(out), err)
		return 1
	}
	if err := os.WriteFile(out, []byte(code), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "error: 写入 %s 失败: %v\n", out, err)
		return 1
	}
	fmt.Printf("ok: %s -> %s\n", file, out)
	return 0
}

func defaultOutPath(file string) string {
	rel := ""
	if filepath.IsAbs(file) {
		if cwd, err := os.Getwd(); err == nil {
			if r, err := filepath.Rel(cwd, file); err == nil && !strings.HasPrefix(r, "..") {
				rel = r
			}
		}
	} else {
		rel = file
	}
	if rel == "" {
		rel = filepath.Base(file)
	}
	return filepath.Join("build", strings.TrimSuffix(rel, filepath.Ext(rel))+".py")
}

func cmdUpdate(args []string) int {
	checkOnly := false
	force := false
	proxy := ""
	insecure := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--check":
			checkOnly = true
		case "--force":
			force = true
		case "--insecure":
			insecure = true
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
	u.Insecure = insecure
	if u.Insecure {
		fmt.Fprintln(os.Stderr, errYellow("警告：--insecure 已跳过产物签名验证（仅建议自建测试源使用）"))
	}
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
		if code := retryWithSudo(exeReal, os.Args[1:]); code >= 0 {
			return code
		}
		fmt.Fprintln(os.Stderr, errRed(err.Error()))
		fmt.Fprintln(os.Stderr, errYellow(fmt.Sprintf("建议：sudo %s update%s 重试（或把 fly 安装到用户可写目录）",
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
		fmt.Fprintf(os.Stderr, "error: %v\n", errRed(err.Error()))
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

func retryWithSudo(exeReal string, args []string) int {
	if os.Geteuid() == 0 || !isTTY(os.Stdin) {
		return -1
	}
	if _, err := exec.LookPath("sudo"); err != nil {
		return -1
	}
	fmt.Println(yellow(fmt.Sprintf("安装目录不可写，将以 sudo 提权重试（%s %s）", exeReal, strings.Join(args, " "))))
	cmd := exec.Command("sudo", append([]string{exeReal}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return ee.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "error: 无法执行 sudo: %v\n", err)
		return 1
	}
	return 0
}

const (
	ansiRed    = "31"
	ansiGreen  = "32"
	ansiYellow = "33"
	ansiCyan   = "36"
	ansiBold   = "1"
)

// errOn：stderr 输出（诊断/错误提示）着色；outOn：stdout 输出着色。
// 判定：NO_COLOR 非空 → 无色；FORCE_COLOR 非空 → 强制彩色；否则按对应流的 isTTY。
var errOn = errColor()
var outOn = outColor()

func errColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return isTTY(os.Stderr)
}

func outColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("FORCE_COLOR") != "" {
		return true
	}
	return isTTY(os.Stdout)
}

func paint(code, s string, on bool) string {
	if !on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func red(s string) string    { return paint(ansiRed, s, outOn) }
func green(s string) string  { return paint(ansiGreen, s, outOn) }
func yellow(s string) string { return paint(ansiYellow, s, outOn) }
func cyan(s string) string   { return paint(ansiCyan, s, outOn) }
func bold(s string) string   { return paint(ansiBold, s, outOn) }

func errRed(s string) string    { return paint(ansiRed, s, errOn) }
func errYellow(s string) string { return paint(ansiYellow, s, errOn) }

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
		fmt.Fprintln(os.Stderr, compile.FormatErrorColor(file, d, errOn))
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

// cmdFmt 格式化 .fly 文件：-w 写回（默认输出 stdout），--check 只报告差异（CI 用）。
// 语法错误文件拒绝格式化（与 fly check 同一编译管线）。
func cmdFmt(args []string) int {
	write := false
	checkOnly := false
	var files []string
	for _, a := range args {
		switch a {
		case "-w", "--write":
			write = true
		case "--check":
			checkOnly = true
		case "-h", "--help":
			fmt.Fprintln(os.Stderr, "用法: fly fmt [-w|--check] <file.fly>...（支持目录，递归查找 .fly）")
			return 0
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "用法: fly fmt [-w|--check] <file.fly>...（支持目录，递归查找 .fly）")
		return 2
	}
	var paths []string
	for _, a := range files {
		info, err := os.Stat(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if info.IsDir() {
			err := filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(p, ".fly") {
					paths = append(paths, p)
				}
				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		} else {
			paths = append(paths, a)
		}
	}
	dirty := 0
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if errs, _ := compile.CheckFile(p); len(errs) > 0 {
			fmt.Fprintf(os.Stderr, "fmt: 跳过 %s（存在编译错误，格式化前请先修复）\n", p)
			dirty++
			continue
		}
		out := format.Format(string(src))
		if out == string(src) {
			continue
		}
		dirty++
		if checkOnly {
			fmt.Printf("需要格式化: %s\n", p)
			continue
		}
		if write {
			if err := os.WriteFile(p, []byte(out), 0644); err != nil {
				fmt.Fprintf(os.Stderr, "error: 写入 %s 失败: %v\n", p, err)
				return 1
			}
			fmt.Printf("ok: %s\n", p)
		} else {
			fmt.Printf("--- %s ---\n%s", p, out)
		}
	}
	if checkOnly && dirty > 0 {
		return 1
	}
	return 0
}

// cmdAnalyze 输出代码质量报告：循环复杂度/认知复杂度/嵌套/函数长度/参数/
// 重复/错误处理/注释比例/命名规范，100 制评分（仿 fuck-u-code 报告口径）。
func cmdAnalyze(args []string) int {
	var files []string
	for _, a := range args {
		switch a {
		case "-h", "--help":
			fmt.Fprintln(os.Stderr, "用法: fly analyze <file.fly>|<dir>...（支持目录，递归查找 .fly）")
			return 0
		default:
			files = append(files, a)
		}
	}
	if len(files) == 0 {
		fmt.Fprintln(os.Stderr, "用法: fly analyze <file.fly>|<dir>...（支持目录，递归查找 .fly）")
		return 2
	}
	var paths []string
	for _, a := range files {
		info, err := os.Stat(a)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if info.IsDir() {
			err := filepath.WalkDir(a, func(p string, d os.DirEntry, err error) error {
				if err != nil {
					return err
				}
				if !d.IsDir() && strings.HasSuffix(p, ".fly") {
					paths = append(paths, p)
				}
				return nil
			})
			if err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				return 1
			}
		} else {
			paths = append(paths, a)
		}
	}
	if len(paths) == 0 {
		fmt.Fprintln(os.Stderr, "error: 未找到 .fly 文件")
		return 1
	}
	reps := make([]rep, 0, len(paths))
	total, tScore := 0.0, 0.0
	worst := []rep{}
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		met := analyze.Analyze(string(src))
		if met == nil {
			fmt.Fprintf(os.Stderr, "analyze: 跳过 %s（语法错误）\n", p)
			continue
		}
		s, b := analyze.Score(met)
		r := rep{path: p, met: met, score: s, bad: b}
		reps = append(reps, r)
		total++
		tScore += s
		if len(worst) < 5 {
			worst = append(worst, r)
			for i := len(worst) - 1; i > 0 && worst[i].bad > worst[i-1].bad; i-- {
				worst[i], worst[i-1] = worst[i-1], worst[i]
			}
		} else {
			for i := range worst {
				if r.bad > worst[i].bad {
					copy(worst[i+1:], worst[i:len(worst)-1])
					worst[i] = r
					break
				}
			}
		}
	}
	if len(reps) == 0 {
		fmt.Fprintln(os.Stderr, "error: 没有可分析的文件")
		return 1
	}
	avg := tScore / total
	fmt.Printf("🌸 屎山代码分析报告 🌸\n\n")
	fmt.Printf("  总体评分: %.2f / 100 - %s\n", avg, analyze.Level(avg))
	fmt.Printf("  已分析 %d 个文件\n\n", len(reps))
	fmt.Println("◆ 评分指标详情（平均分项）")
	avgM := aggregate(reps)
	fmt.Printf("  ✓✓ 循环复杂度    %.1f%%  平均 %d（目标 ≤ 10）\n", rateOf(float64(avgM.Cyclomatic), 10), avgM.Cyclomatic)
	fmt.Printf("  ✓✓ 认知复杂度    %.1f%%  平均 %d（目标 ≤ 15）\n", rateOf(float64(avgM.Cognitive), 15), avgM.Cognitive)
	fmt.Printf("  ✓✓ 嵌套深度      %.1f%%  最大 %d（目标 ≤ 4）\n", rateOf(float64(avgM.MaxNest), 4), avgM.MaxNest)
	fmt.Printf("  ✓✓ 函数长度      %.1f%%  最长 %d 行（目标 ≤ 50）\n", rateOf(float64(avgM.MaxFuncLen), 50), avgM.MaxFuncLen)
	fmt.Printf("  ✓✓ 文件长度      %.1f%%  平均 %d 行（目标 ≤ 500）\n", rateOf(float64(avgM.Lines), 500), avgM.Lines)
	fmt.Printf("  ✓✓ 参数数量      %.1f%%  最多 %d 个（目标 ≤ 5）\n", rateOf(float64(avgM.MaxParams), 5), avgM.MaxParams)
	fmt.Printf("  ✓✓ 代码重复      %.1f%%  重复 %.1f%%\n", avgM.RepeatRate*100, avgM.RepeatRate*100)
	fmt.Printf("  ✓✓ 错误处理      %.1f%%  try %d / raise %d\n", rateOf(float64(avgM.TryCount), float64(avgM.FuncCount+1)), avgM.TryCount, avgM.RaiseCount)
	fmt.Printf("  ✓✓ 注释比例      %.1f%%  注释行 %.1f%%\n", 100-avgM.CommentRate*100, avgM.CommentRate*100)
	fmt.Printf("  ✓✓ 命名规范      %.1f%%  非 snake_case %.1f%%\n", 100-avgM.NameRate*100, avgM.NameRate*100)
	fmt.Println("\n◆ 最屎代码排行榜（糟糕指数前 5）")
	for i, r := range worst {
		fmt.Printf("  %d. %-55s (糟糕指数: %.2f)\n", i+1, r.path, r.bad)
		if len(r.met.Functions) > 0 {
			for _, f := range r.met.Functions {
				if f.Complex {
					fmt.Printf("     🔄 %s() L%d: 循环复杂度 %d 认知 %d 嵌套 %d 长度 %d\n",
						f.Name, f.Line, f.Cyclo, f.Cognit, f.Nest, f.Length)
				}
			}
		}
	}
	fmt.Printf("\n◆ 诊断结论\n  🌸 %s\n", analyze.Level(avg))
	return 0
}

type rep struct {
	path  string
	met   *analyze.Metrics
	score float64
	bad   float64
}

// aggregate 汇总所有文件指标均值。
func aggregate(reps []rep) analyze.Metrics {
	m := analyze.Metrics{}
	n := float64(len(reps))
	for _, r := range reps {
		m.Cyclomatic += r.met.Cyclomatic
		m.Cognitive += r.met.Cognitive
		m.MaxNest += r.met.MaxNest
		m.MaxFuncLen += r.met.MaxFuncLen
		m.Lines += r.met.Lines
		m.MaxParams += r.met.MaxParams
		m.TryCount += r.met.TryCount
		m.RaiseCount += r.met.RaiseCount
		m.RepeatRate += r.met.RepeatRate
		m.CommentRate += r.met.CommentRate
		m.NameRate += r.met.NameRate
	}
	m.Cyclomatic = int(float64(m.Cyclomatic) / n)
	m.Cognitive = int(float64(m.Cognitive) / n)
	m.MaxNest = int(float64(m.MaxNest) / n)
	m.MaxFuncLen = int(float64(m.MaxFuncLen) / n)
	m.Lines = int(float64(m.Lines) / n)
	m.MaxParams = int(float64(m.MaxParams) / n)
	m.TryCount = int(float64(m.TryCount) / n)
	m.RaiseCount = int(float64(m.RaiseCount) / n)
	m.RepeatRate /= n
	m.CommentRate /= n
	m.NameRate /= n
	return m
}

func rateOf(v, target float64) float64 {
	if v >= target {
		return 0
	}
	return (1 - v/target) * 100
}
