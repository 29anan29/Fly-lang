package compile

import (
	"fmt"
	"os"
	"strings"

	"flylang/internal/ast"
	"flylang/internal/checker"
	"flylang/internal/gen"
	"flylang/internal/parser"
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
	p := parser.New(src)
	m := p.ParseModule()
	if d := p.Error(); d != nil {
		return "", []ast.Diagnostic{*d}, nil
	}
	if errs := checker.Check(m); len(errs) > 0 {
		return "", errs, nil
	}
	return gen.Generate(m), nil, nil
}

func CheckFile(path string) ([]ast.Diagnostic, error) {
	_, errs := ParseFile(path)
	if len(errs) > 0 {
		return errs, nil
	}
	return nil, nil
}

func BuildFile(path string) (string, []ast.Diagnostic, error) {
	m, errs := ParseFile(path)
	if len(errs) > 0 {
		return "", errs, nil
	}
	return gen.Generate(m), nil, nil
}

func FormatError(path string, d ast.Diagnostic) string {
	src, _ := os.ReadFile(path)
	return formatError(path, string(src), d)
}

func formatError(path, src string, d ast.Diagnostic) string {
	info, ok := ast.InfoForCode(d.Code)
	if !ok {
		if d.Pos.Line == 0 {
			return fmt.Sprintf("error: %s: %s", path, d.Msg)
		}
		return fmt.Sprintf("error: %s:%d:%d: %s", path, d.Pos.Line, d.Pos.Col, d.Msg)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "error[%s]: %s\n", d.Code, info.Title)
	fmt.Fprintf(&b, "  --> %s:%d:%d\n", path, d.Pos.Line, d.Pos.Col)
	if line, ok := srcLine(src, d.Pos.Line); ok {
		len := underlineLen(line, d.Pos.Col)
		fmt.Fprintf(&b, "   |\n")
		fmt.Fprintf(&b, "%4d | %s\n", d.Pos.Line, line)
		fmt.Fprintf(&b, "   | %s^%s\n", strings.Repeat(" ", d.Pos.Col-1), strings.Repeat("^", len-1))
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
