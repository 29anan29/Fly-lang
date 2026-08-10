package compile

import (
	"fmt"
	"os"

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
	if d.Pos.Line == 0 {
		return fmt.Sprintf("error: %s: %s", path, d.Msg)
	}
	return fmt.Sprintf("error: %s:%d:%d: %s", path, d.Pos.Line, d.Pos.Col, d.Msg)
}
