package main

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"flylang/internal/ast"
)

// gen_errorinfo：把 Go 版错误码注册表（internal/ast/errors.go 的 errorInfo）
// 生成为 Rust 源文件 src/errorinfo.rs。产物入库，生成器可不保留。
func main() {
	codes := make([]string, 0, len(ast.AllErrorInfos()))
	for code := range ast.AllErrorInfos() {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	var b strings.Builder
	b.WriteString("// 由 tools/gen_errorinfo 从 internal/ast/errors.go 生成，勿手改。\n")
	b.WriteString("// 错误码注册表：修改 Go 版后需重新生成（cargo test 有全码抽查）。\n\n")
	b.WriteString("pub struct ErrorInfo {\n")
	b.WriteString("    pub code: &'static str,\n")
	b.WriteString("    pub title: &'static str,\n")
	b.WriteString("    pub help: &'static str,\n")
	b.WriteString("    pub note: &'static str,\n")
	b.WriteString("    pub example: &'static str,\n")
	b.WriteString("}\n\n")
	b.WriteString("pub static ERROR_INFO: &[ErrorInfo] = &[\n")
	for _, code := range codes {
		info, _ := ast.AllErrorInfos()[code]
		fmt.Fprintf(&b, "    ErrorInfo {\n")
		fmt.Fprintf(&b, "        code: %s,\n", strq(info.Code))
		fmt.Fprintf(&b, "        title: %s,\n", strq(info.Title))
		fmt.Fprintf(&b, "        help: %s,\n", strq(info.Help))
		fmt.Fprintf(&b, "        note: %s,\n", strq(info.Note))
		fmt.Fprintf(&b, "        example: %s,\n", strq(info.Example))
		b.WriteString("    },\n")
	}
	b.WriteString("];\n")
	os.WriteFile("src/errorinfo.rs", []byte(b.String()), 0644)
}

func strq(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\r':
			b.WriteString("\\r")
		case '\t':
			b.WriteString("\\t")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
