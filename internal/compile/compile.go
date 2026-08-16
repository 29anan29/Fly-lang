package compile

import (
	"fmt"
	"os"
	"strings"

	"pyfly/internal/ast"
	"pyfly/internal/checker"
	"pyfly/internal/gen"
	"pyfly/internal/parser"
)

type Result struct {
	Code string
	Errs []ast.Diagnostic
}

func (r *Result) Failed() bool {
	return len(r.Errs) > 0
}

func ParseFile(path string) (*ast.Module, []ast.Diagnostic) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, []ast.Diagnostic{{Msg: err.Error()}}
	}
	p := parser.New(string(src))
	m := p.ParseModule()
	if d := p.Error(); d != nil {
		return nil, []ast.Diagnostic{*d}
	}
	if errs := checker.Check(m); len(errs) > 0 {
		return nil, errs
	}
	return m, nil
}

func CheckSource(src string) []ast.Diagnostic {
	p := parser.New(src)
	m := p.ParseModule()
	if d := p.Error(); d != nil {
		return []ast.Diagnostic{*d}
	}
	return checker.Check(m)
}

func BuildSource(src string) (string, []ast.Diagnostic, error) {
	return BuildSourceOpts(src, gen.GenOpts{})
}

// BuildSourceOpts 同 BuildSource，支持代码生成选项（如 --keep-annotations）。
func BuildSourceOpts(src string, opts gen.GenOpts) (string, []ast.Diagnostic, error) {
	p := parser.New(src)
	m := p.ParseModule()
	if d := p.Error(); d != nil {
		return "", []ast.Diagnostic{*d}, nil
	}
	if errs := checker.Check(m); len(errs) > 0 {
		return "", errs, nil
	}
	return gen.GenerateOpts(m, opts), nil, nil
}

func CheckFile(path string) ([]ast.Diagnostic, error) {
	_, errs := ParseFile(path)
	if len(errs) > 0 {
		return errs, nil
	}
	return nil, nil
}

func BuildFile(path string) (string, []ast.Diagnostic, error) {
	return BuildFileOpts(path, gen.GenOpts{})
}

// BuildFileOpts 同 BuildFile，支持代码生成选项（如 --keep-annotations）。
func BuildFileOpts(path string, opts gen.GenOpts) (string, []ast.Diagnostic, error) {
	m, errs := ParseFile(path)
	if len(errs) > 0 {
		return "", errs, nil
	}
	return gen.GenerateOpts(m, opts), nil, nil
}

func FormatError(path string, d ast.Diagnostic) string {
	src, _ := os.ReadFile(path)
	return formatError(path, string(src), d, false)
}

func FormatErrorColor(path string, d ast.Diagnostic, color bool) string {
	src, _ := os.ReadFile(path)
	return formatError(path, string(src), d, color)
}

var colorCodes = map[string]string{
	"red":   "\x1b[31m",
	"bred":  "\x1b[1;31m",
	"cyan":  "\x1b[1;36m",
	"reset": "\x1b[0m",
}

func colorWrap(color bool, code, s string) string {
	if !color {
		return s
	}
	return colorCodes[code] + s + colorCodes["reset"]
}

func formatError(path, src string, d ast.Diagnostic, color bool) string {
	info, ok := ast.InfoForCode(d.Code)
	if !ok {
		if d.Pos.Line == 0 {
			return fmt.Sprintf("error: %s: %s", path, d.Msg)
		}
		return fmt.Sprintf("error: %s:%d:%d: %s", path, d.Pos.Line, d.Pos.Col, d.Msg)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s\n",
		colorWrap(color, "bred", "error["+d.Code+"]"), info.Title)
	fmt.Fprintf(&b, "%s\n",
		colorWrap(color, "cyan", fmt.Sprintf("  --> %s:%d:%d", path, d.Pos.Line, d.Pos.Col)))
	if line, ok := srcLine(src, d.Pos.Line); ok {
		len := underlineLen(line, d.Pos.Col)
		fmt.Fprintf(&b, "   |\n")
		fmt.Fprintf(&b, "%4d | %s\n", d.Pos.Line, line)
		under := strings.Repeat("^", len-1)
		fmt.Fprintf(&b, "   | %s\n", colorWrap(color, "red", strings.Repeat(" ", d.Pos.Col-1)+"^"+under))
	}
	fmt.Fprintf(&b, "   |\n")
	if d.Msg != info.Title {
		fmt.Fprintf(&b, "   = help: %s。%s\n", d.Msg, info.Help)
	} else {
		fmt.Fprintf(&b, "   = help: %s\n", info.Help)
	}
	fmt.Fprintf(&b, "   = note: %s\n", info.Note)
	return b.String()
}

func srcLine(src string, line int) (string, bool) {
	if src == "" {
		return "", false
	}
	cur := 1
	for _, l := range strings.SplitAfter(src, "\n") {
		if cur == line {
			return strings.TrimRight(l, "\n"), true
		}
		cur++
	}
	return "", false
}

func underlineLen(line string, col int) int {
	if col-1 > len(line) {
		return 1
	}
	rest := line[col-1:]
	n := 0
	for n < len(rest) && n < 32 && rest[n] != ' ' {
		n++
	}
	if n == 0 {
		return 1
	}
	return n
}
