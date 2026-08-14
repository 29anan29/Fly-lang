// analyze.go：fly analyze 的静态分析器——基于 AST 与源码度量
// 循环复杂度（McCabe）/认知复杂度/嵌套/函数长度/参数/重复/错误处理/
// 注释比例/命名规范，输出 100 制评分与分项扣分（仿 fuck-u-code 报告风格）。
package analyze

import (
	"regexp"
	"strings"

	"flylang/internal/ast"
	"flylang/internal/format"
	"flylang/internal/parser"
)

type Metrics struct {
	Cyclomatic  int     // 循环复杂度（McCabe）
	Cognitive   int     // 认知复杂度（简化 SonarQube）
	MaxNest     int     // 最大嵌套深度
	FuncCount   int     // 函数数
	MaxFuncLen  int     // 最长函数行数
	MaxParams   int     // 最多参数数
	TryCount    int     // try 语句数
	RaiseCount  int     // raise 语句数
	Lines       int     // 源码行数
	CodeLines   int     // 非空非注释行
	CommentRate float64 // 注释行占比
	RepeatRate  float64 // 重复代码行占比（3 行 n-gram）
	NameRate    float64 // 非 snake_case 标识符占比
	Functions   []FuncMetric
}

type FileReport struct {
	Path      string
	Metrics   Metrics
	Score     float64
	BadScore  float64 // 糟糕指数（扣分权重和）
	Functions []FuncMetric
}

type FuncMetric struct {
	Name    string
	Line    int
	Length  int
	Params  int
	Cyclo   int
	Cognit  int
	Nest    int
	Complex bool // 是否超阈值（需拆分）
}

// Analyze 对单个文件源码执行分析；语法错误返回 nil（调用方跳过并报告）。
func Analyze(src string) *Metrics {
	m := parser.New(src)
	mod := m.ParseModule()
	if m.Error() != nil {
		return nil
	}
	lines := strings.Split(src, "\n")
	metric := &Metrics{Lines: len(lines)}
	comments := format.CommentLines(src)
	commentSet := map[int]bool{}
	for _, l := range comments {
		commentSet[l] = true
	}
	walkStmts(mod.Stmts, 0, metric)
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || commentSet[i+1] {
			continue
		}
		metric.CodeLines++
	}
	metric.CommentRate = float64(len(comments)) / float64(len(lines))
	metric.RepeatRate = repeatRate(lines, commentSet)
	metric.NameRate = nameRate(mod)
	return metric
}

// analyzer 遍历器：统计复杂度/嵌套/函数/参数/try/raise。
type analyzer struct {
	cyclo int
	cog   int
	max   int
	fns   []FuncMetric
	try   int
	raise int
	// 当前函数上下文
	curMaxLine int
}

func walkStmts(stmts []ast.Stmt, depth int, m *Metrics) {
	a := &analyzer{}
	for _, s := range stmts {
		a.stmt(s, depth)
	}
	m.Cyclomatic = a.cyclo + 1
	m.Cognitive = a.cog
	m.MaxNest = a.max
	m.FuncCount = len(a.fns)
	m.TryCount = a.try
	m.RaiseCount = a.raise
	m.Functions = a.fns
	for _, f := range a.fns {
		if f.Length > m.MaxFuncLen {
			m.MaxFuncLen = f.Length
		}
		if f.Params > m.MaxParams {
			m.MaxParams = f.Params
		}
	}
}

// stmt 递归遍历语句，维护嵌套深度与认知复杂度。
func (a *analyzer) stmt(s ast.Stmt, depth int) {
	if s == nil {
		return
	}
	if depth+1 > a.max {
		a.max = depth + 1
	}
	if ln := s.Pos().Line; ln > a.curMaxLine {
		a.curMaxLine = ln
	}
	switch t := s.(type) {
	case *ast.FuncDef:
		a.cog += 1 + depth // 函数定义嵌套 +1（McCabe：函数定义不是分支）
		base := a.cyclo
		cogBase := a.cog
		maxBase := a.max
		start := t.Pos().Line
		prevMax := a.curMaxLine
		for _, p := range t.Params {
			a.expr(p.Default, depth+1)
		}
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
		a.fns = append(a.fns, FuncMetric{
			Name:    t.Name,
			Line:    start,
			Length:  a.curMaxLine - start + 1,
			Params:  len(t.Params),
			Cyclo:   a.cyclo - base,
			Cognit:  a.cog - cogBase,
			Nest:    a.max - maxBase,
			Complex: a.cyclo-base > 10 || a.cog-cogBase > 15 || a.max-maxBase > 4,
		})
		a.curMaxLine = prevMax
	case *ast.IfStmt:
		a.cyclo++
		a.cog += 1 + depth
		a.expr(t.Cond, depth+1)
		for _, b := range t.Then {
			a.stmt(b, depth+1)
		}
		for _, e := range t.Elifs {
			a.cyclo++
			a.cog += 1 + depth
			a.expr(e.Cond, depth+1)
			for _, b := range e.Body {
				a.stmt(b, depth+1)
			}
		}
		for _, b := range t.Else {
			a.stmt(b, depth+1)
		}
	case *ast.ForStmt:
		a.cyclo++
		a.cog += 1 + depth
		a.expr(t.Target, depth+1)
		a.expr(t.Iter, depth+1)
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
		for _, b := range t.Else {
			a.stmt(b, depth+1)
		}
	case *ast.WhileStmt:
		a.cyclo++
		a.cog += 1 + depth
		a.expr(t.Cond, depth+1)
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
		for _, b := range t.Else {
			a.stmt(b, depth+1)
		}
	case *ast.TryStmt:
		a.cyclo++
		a.cog += 1 + depth
		a.try++
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
		for _, e := range t.Handlers {
			a.cyclo++
			a.cog += 1 + depth
			for _, b := range e.Body {
				a.stmt(b, depth+1)
			}
		}
		for _, b := range t.Else {
			a.stmt(b, depth+1)
		}
		for _, b := range t.Finally {
			a.stmt(b, depth+1)
		}
	case *ast.RaiseStmt:
		a.raise++
		a.expr(t.Exc, depth+1)
		a.expr(t.From, depth+1)
	case *ast.AssignStmt:
		for _, l := range t.Left {
			a.expr(l, depth+1)
		}
		a.expr(t.Right, depth+1)
	case *ast.ExprStmt:
		a.expr(t.X, depth+1)
	case *ast.GuardStmt:
		a.cyclo++
		for _, c := range t.Conds {
			a.expr(c, depth+1)
		}
	case *ast.ClassDef:
		a.cog += 1 + depth
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
	case *ast.LockStmt:
		a.expr(t.Value, depth+1)
	case *ast.ReturnStmt:
		a.expr(t.Value, depth+1)
	case *ast.DeleteStmt:
		for _, x := range t.Targets {
			a.expr(x, depth+1)
		}
	case *ast.OnlyStmt:
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
	case *ast.TraceStmt:
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
	case *ast.CageStmt:
		for _, b := range t.Body {
			a.stmt(b, depth+1)
		}
	case *ast.SafeStmt, *ast.MaskStmt, *ast.ImportStmt, *ast.FromImportStmt,
		*ast.PassStmt, *ast.BreakStmt, *ast.ContinueStmt:
	}
}

func (a *analyzer) expr(e ast.Expr, depth int) {
	if e == nil {
		return
	}
	switch t := e.(type) {
	case *ast.CallExpr:
		a.expr(t.Func, depth)
		for _, x := range t.Args {
			a.expr(x, depth)
		}
		a.expr(t.Star, depth)
		a.expr(t.DblStar, depth)
		for _, kw := range t.Kwargs {
			a.expr(kw.Value, depth)
		}
	case *ast.AttrExpr:
		a.expr(t.X, depth)
	case *ast.SubscriptExpr:
		a.expr(t.X, depth)
		a.expr(t.Index, depth)
	case *ast.SliceExpr:
		a.expr(t.Lo, depth)
		a.expr(t.Hi, depth)
		a.expr(t.Step, depth)
	case *ast.BinOpExpr:
		a.expr(t.X, depth)
		a.expr(t.Y, depth)
	case *ast.UnaryOpExpr:
		a.expr(t.X, depth)
	case *ast.BoolOpExpr:
		if t.Op == "and" || t.Op == "or" {
			a.cyclo++
			a.cog += 1 + depth
		}
		a.expr(t.X, depth)
		a.expr(t.Y, depth)
	case *ast.CompareExpr:
		a.expr(t.X, depth)
		for _, y := range t.Ys {
			a.expr(y, depth)
		}
	case *ast.CondExpr:
		a.cyclo++
		a.cog += 1 + depth
		a.expr(t.Cond, depth)
		a.expr(t.Then, depth)
		a.expr(t.Else, depth)
	case *ast.ListLit:
		for _, el := range t.Elems {
			a.expr(el, depth)
		}
	case *ast.TupleLit:
		for _, el := range t.Elems {
			a.expr(el, depth)
		}
	case *ast.DictLit:
		for i := range t.Keys {
			a.expr(t.Keys[i], depth)
			a.expr(t.Vals[i], depth)
		}
	case *ast.SetLit:
		for _, el := range t.Elems {
			a.expr(el, depth)
		}
	case *ast.ListComp:
		a.expr(t.Elem, depth)
		for _, c := range t.Clauses {
			a.cyclo++
			a.cog += 1 + depth
			a.expr(c.Iter, depth)
			for _, cf := range c.Ifs {
				a.cyclo++
				a.cog += 1 + depth
				a.expr(cf, depth)
			}
		}
	}
}

// repeatRate 重复代码占比：3 行 n-gram 指纹（跳过空行/注释），出现 >1 次的码行比例。
func repeatRate(lines []string, commentSet map[int]bool) float64 {
	var code []string
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || commentSet[i+1] {
			code = append(code, "\x00")
		} else {
			code = append(code, t)
		}
	}
	counts := map[string]int{}
	for i := 0; i+3 <= len(code); i++ {
		if code[i] == "\x00" || code[i+1] == "\x00" || code[i+2] == "\x00" {
			continue
		}
		key := code[i] + "\x01" + code[i+1] + "\x01" + code[i+2]
		counts[key]++
	}
	if len(code) == 0 {
		return 0
	}
	dup := 0
	seen := map[int]bool{}
	for i := 0; i+3 <= len(code); i++ {
		if code[i] == "\x00" {
			continue
		}
		key := code[i] + "\x01" + code[i+1] + "\x01" + code[i+2]
		if counts[key] > 1 && !seen[i] {
			for j := i; j < i+3; j++ {
				if !seen[j] {
					seen[j] = true
					dup++
				}
			}
		}
	}
	return float64(dup) / float64(len(code))
}

var snakeRe = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

// nameRate 非 snake_case 标识符占比（函数名/参数名/赋值目标名）。
func nameRate(mod *ast.Module) float64 {
	total, bad := 0, 0
	check := func(n string) {
		if n == "" {
			return
		}
		total++
		if !snakeRe.MatchString(n) {
			bad++
		}
	}
	for _, s := range mod.Stmts {
		if f, ok := s.(*ast.FuncDef); ok {
			check(f.Name)
			for _, p := range f.Params {
				check(p.Name)
			}
		}
		if c, ok := s.(*ast.ClassDef); ok {
			check(c.Name)
		}
	}
	if total == 0 {
		return 0
	}
	return float64(bad) / float64(total)
}

// Score 计算 100 制评分与糟糕指数（扣分权重与 fuck-u-code 报告口径一致）。
func Score(m *Metrics) (score, bad float64) {
	var b float64
	deduct := func(w float64) { b += w }
	if m.Cyclomatic > 10 {
		deduct(float64(m.Cyclomatic-10) / 5 * 4)
	}
	if m.Cognitive > 15 {
		deduct(float64(m.Cognitive-15) / 5 * 3)
	}
	if m.MaxNest > 4 {
		deduct(float64(m.MaxNest-4) * 1.5)
	}
	if m.MaxFuncLen > 50 {
		deduct(float64(m.MaxFuncLen-50) / 10 * 1.5)
	}
	if m.Lines > 500 {
		deduct(float64(m.Lines-500) / 100 * 1.5)
	}
	if m.MaxParams > 5 {
		deduct(float64(m.MaxParams-5) * 2)
	}
	deduct(m.RepeatRate * 20)
	if m.FuncCount > 0 {
		covered := float64(m.TryCount) / float64(m.FuncCount)
		deduct((1 - covered) * 6)
	}
	switch {
	case m.CommentRate < 0.05:
		deduct(4)
	case m.CommentRate < 0.10:
		deduct(2)
	case m.CommentRate > 0.40:
		deduct((m.CommentRate - 0.40) * 10)
	}
	deduct(m.NameRate * 20)
	if b > 100 {
		b = 100
	}
	return 100 - b, b
}

// Level 按评分给出等级文案（对齐 fuck-u-code 口径）。
func Level(score float64) string {
	switch {
	case score >= 90:
		return "清流 - 代码洁癖者的骄傲"
	case score >= 75:
		return "略带清香 - 偶尔飘过一丝酸爽"
	case score >= 60:
		return "屎气扑鼻 - 代码开始散发气味，谨慎维护"
	default:
		return "屎山 - 建议大重构"
	}
}
