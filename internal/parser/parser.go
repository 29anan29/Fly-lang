package parser

import (
	"fmt"
	"strings"

	"flylang/internal/ast"
	"flylang/internal/lexer"
)

type Parser struct {
	lex *lexer.Lexer
	tok lexer.Token
	err *ast.Diagnostic
}

func New(src string) *Parser {
	p := &Parser{lex: lexer.New(src)}
	p.next()
	return p
}

func (p *Parser) next() {
	p.tok = p.lex.Next()
}

func (p *Parser) errorf(pos ast.Position, format string, args ...interface{}) {
	if p.err == nil {
		p.err = &ast.Diagnostic{Pos: pos, Msg: fmt.Sprintf(format, args...)}
	}
}

func (p *Parser) Error() *ast.Diagnostic {
	if p.err != nil {
		return p.err
	}
	return p.lex.Err()
}

func (p *Parser) expect(tt lexer.TokenType) lexer.Token {
	if p.tok.Type != tt {
		p.errorf(p.tok.Pos, "期望 %s，实际为 %s", tt, p.tok.Lit)
		return lexer.Token{Type: lexer.ILLEGAL, Pos: p.tok.Pos}
	}
	t := p.tok
	p.next()
	return t
}

func (p *Parser) ParseModule() *ast.Module {
	if p.lex.Err() != nil {
		return nil
	}
	m := &ast.Module{Pos_: p.tok.Pos}
	for p.tok.Type != lexer.EOF && p.err == nil {
		if s := p.statement(); s != nil {
			m.Stmts = append(m.Stmts, s)
		}
	}
	return m
}

func (p *Parser) statement() ast.Stmt {
	switch p.tok.Type {
	case lexer.NEWLINE:
		p.next()
		return nil
	case lexer.SEMICOLON:
		for p.tok.Type == lexer.SEMICOLON {
			p.next()
		}
		return nil
	case lexer.DEF:
		return p.funcDef(nil)
	case lexer.CLASS:
		return p.classDef(nil)
	case lexer.IF:
		return p.ifStmt()
	case lexer.FOR:
		return p.forStmt()
	case lexer.WHILE:
		return p.whileStmt()
	case lexer.RETURN:
		pos := p.tok.Pos
		p.next()
		r := &ast.ReturnStmt{Pos_: pos}
		if !p.atEnd() {
			r.Value = p.parseTestList()
		}
		p.stmtEnd()
		return r
	case lexer.RAISE:
		pos := p.tok.Pos
		p.next()
		r := &ast.RaiseStmt{Pos_: pos}
		if !p.atEnd() {
			r.Exc = p.parseTestList()
			if p.tok.Type == lexer.FROM {
				p.next()
				r.From = p.parseTestList()
			}
		}
		p.stmtEnd()
		return r
	case lexer.TRY:
		return p.tryStmt()
	case lexer.PASS:
		pos := p.tok.Pos
		p.next()
		p.stmtEnd()
		return &ast.PassStmt{Pos_: pos}
	case lexer.BREAK:
		pos := p.tok.Pos
		p.next()
		p.stmtEnd()
		return &ast.BreakStmt{Pos_: pos}
	case lexer.CONTINUE:
		pos := p.tok.Pos
		p.next()
		p.stmtEnd()
		return &ast.ContinueStmt{Pos_: pos}
	case lexer.DEL:
		pos := p.tok.Pos
		p.next()
		d := &ast.DeleteStmt{Pos_: pos}
		for {
			d.Targets = append(d.Targets, p.parseTestList())
			if p.tok.Type != lexer.COMMA {
				break
			}
			p.next()
		}
		p.stmtEnd()
		return d
	case lexer.IMPORT:
		return p.importStmt()
	case lexer.FROM:
		return p.fromImportStmt()
	case lexer.AT:
		return p.decoratedStmt()
	case lexer.ASSERT, lexer.WITH, lexer.YIELD, lexer.GLOBAL, lexer.NONLOCAL, lexer.LAMBDA, lexer.ASYNC, lexer.AWAIT:
		p.errorf(p.tok.Pos, "关键字 %s 暂不支持", p.tok.Lit)
		return nil
	case lexer.SAFE, lexer.ONLY, lexer.LOCK, lexer.MASK, lexer.CAGE, lexer.GUARD, lexer.SEAL, lexer.TRACE:
		p.errorf(p.tok.Pos, "关键字 %s 将在后续阶段支持", p.tok.Lit)
		return nil
	default:
		return p.simpleStmt()
	}
}

func (p *Parser) atEnd() bool {
	return p.tok.Type == lexer.NEWLINE || p.tok.Type == lexer.EOF || p.tok.Type == lexer.SEMICOLON
}

func (p *Parser) stmtEnd() {
	for p.tok.Type == lexer.SEMICOLON {
		p.next()
	}
	if p.tok.Type != lexer.NEWLINE && p.tok.Type != lexer.EOF {
		p.errorf(p.tok.Pos, "期望语句结束，实际为 %q", p.tok.Lit)
	}
	if p.tok.Type == lexer.NEWLINE {
		p.next()
	}
}

func (p *Parser) simpleStmt() ast.Stmt {
	var first ast.Stmt
	for {
		var s ast.Stmt
		exprs := p.parseTestList()
		if exprs == nil {
			return nil
		}
		if p.tok.Type == lexer.ASSIGN || p.isAugAssign() {
			left := []ast.Expr{exprs}
			op := p.tok.Lit
			p.next()
			right := p.parseTestList()
			if p.tok.Type == lexer.ASSIGN {
				for {
					if !p.checkTarget(right) {
						p.errorf(p.posOf(right), "非法赋值目标")
					}
					left = append(left, right)
					op = "="
					p.next()
					right = p.parseTestList()
					if p.tok.Type != lexer.ASSIGN {
						break
					}
				}
			}
			if !p.checkTarget(left[0]) {
				p.errorf(p.posOf(left[0]), "非法赋值目标")
			}
			s = &ast.AssignStmt{Pos_: p.posOf(left[0]), Left: left, Op: op, Right: right}
		} else {
			s = &ast.ExprStmt{Pos_: p.posOf(exprs), X: exprs}
		}
		if first == nil {
			first = s
		}
		if p.tok.Type != lexer.SEMICOLON {
			break
		}
		p.next()
		if p.atEnd() {
			break
		}
	}
	p.stmtEnd()
	return first
}

func (p *Parser) isAugAssign() bool {
	switch p.tok.Type {
	case lexer.PLUS_ASSIGN, lexer.MINUS_ASSIGN, lexer.STAR_ASSIGN, lexer.SLASH_ASSIGN,
		lexer.PERCENT_ASSIGN, lexer.DOUBLESTAR_ASSIGN, lexer.FLOORDIV_ASSIGN,
		lexer.SHL_ASSIGN, lexer.SHR_ASSIGN, lexer.AMP_ASSIGN, lexer.PIPE_ASSIGN, lexer.CARET_ASSIGN:
		return true
	}
	return false
}

func (p *Parser) checkTarget(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.Name, *ast.AttrExpr, *ast.SubscriptExpr:
		return true
	case *ast.TupleLit:
		for _, el := range t.Elems {
			if !p.checkTarget(el) {
				return false
			}
		}
		return true
	case *ast.ListLit:
		for _, el := range t.Elems {
			if !p.checkTarget(el) {
				return false
			}
		}
		return true
	case *ast.UnaryOpExpr:
		if t.Op == "*" {
			_, ok := t.X.(*ast.Name)
			return ok
		}
	}
	return false
}

func (p *Parser) posOf(e ast.Expr) ast.Position {
	if e == nil {
		return p.tok.Pos
	}
	return e.Pos()
}

func (p *Parser) funcDef(decorators []ast.Expr) ast.Stmt {
	pos := p.tok.Pos
	p.next()
	name := p.expect(lexer.IDENT)
	f := &ast.FuncDef{Pos_: pos, Name: name.Lit, Decorators: decorators}
	p.expect(lexer.LPAREN)
	f.Params = p.params()
	p.expect(lexer.RPAREN)
	if p.tok.Type == lexer.ARROW {
		p.next()
		f.ReturnType = p.parseTest()
	}
	f.Body = p.suite()
	return f
}

func (p *Parser) params() []ast.Param {
	var params []ast.Param
	if p.tok.Type == lexer.RPAREN {
		return params
	}
	for {
		var prm ast.Param
		if p.tok.Type == lexer.STAR {
			p.next()
			if p.tok.Type == lexer.IDENT {
				prm.Star = true
				prm.Name = p.expect(lexer.IDENT).Lit
			} else {
				prm.Star = true
			}
		} else if p.tok.Type == lexer.DOUBLESTAR {
			p.next()
			prm.DblStar = true
			prm.Name = p.expect(lexer.IDENT).Lit
		} else {
			prm.Name = p.expect(lexer.IDENT).Lit
		}
		if p.tok.Type == lexer.COLON {
			p.next()
			prm.Anno = p.parseTest()
		}
		if p.tok.Type == lexer.ASSIGN {
			p.next()
			prm.Default = p.parseTest()
		}
		params = append(params, prm)
		if p.tok.Type != lexer.COMMA {
			break
		}
		p.next()
		if p.tok.Type == lexer.RPAREN {
			break
		}
	}
	return params
}

func (p *Parser) classDef(decorators []ast.Expr) ast.Stmt {
	pos := p.tok.Pos
	p.next()
	name := p.expect(lexer.IDENT)
	c := &ast.ClassDef{Pos_: pos, Name: name.Lit, Decorators: decorators}
	if p.tok.Type == lexer.LPAREN {
		p.next()
		if p.tok.Type != lexer.RPAREN {
			for {
				c.Bases = append(c.Bases, p.parseTest())
				if p.tok.Type != lexer.COMMA {
					break
				}
				p.next()
			}
		}
		p.expect(lexer.RPAREN)
	}
	c.Body = p.suite()
	return c
}

func (p *Parser) ifStmt() ast.Stmt {
	pos := p.tok.Pos
	p.next()
	s := &ast.IfStmt{Pos_: pos}
	s.Cond = p.parseTestList()
	s.Then = p.suite()
	for p.tok.Type == lexer.ELIF {
		el := ast.ElifClause{Pos_: p.tok.Pos}
		p.next()
		el.Cond = p.parseTestList()
		el.Body = p.suite()
		s.Elifs = append(s.Elifs, el)
	}
	if p.tok.Type == lexer.ELSE {
		p.next()
		s.Else = p.suite()
	}
	return s
}

func (p *Parser) forStmt() ast.Stmt {
	pos := p.tok.Pos
	p.next()
	s := &ast.ForStmt{Pos_: pos}
	s.Target = p.parseExprList()
	p.expect(lexer.IN)
	s.Iter = p.parseTestList()
	s.Body = p.suite()
	if p.tok.Type == lexer.ELSE {
		p.next()
		s.Else = p.suite()
	}
	return s
}

func (p *Parser) whileStmt() ast.Stmt {
	pos := p.tok.Pos
	p.next()
	s := &ast.WhileStmt{Pos_: pos}
	s.Cond = p.parseTestList()
	s.Body = p.suite()
	if p.tok.Type == lexer.ELSE {
		p.next()
		s.Else = p.suite()
	}
	return s
}

func (p *Parser) tryStmt() ast.Stmt {
	pos := p.tok.Pos
	p.next()
	s := &ast.TryStmt{Pos_: pos}
	s.Body = p.suite()
	for p.tok.Type == lexer.EXCEPT {
		ec := ast.ExceptClause{Pos_: p.tok.Pos}
		p.next()
		if p.tok.Type != lexer.COLON {
			ec.Type = p.parseTest()
			if p.tok.Type == lexer.AS {
				p.next()
				ec.Name = p.expect(lexer.IDENT).Lit
			}
		}
		ec.Body = p.suite()
		s.Handlers = append(s.Handlers, ec)
	}
	if p.tok.Type == lexer.ELSE {
		p.next()
		s.Else = p.suite()
	}
	if p.tok.Type == lexer.FINALLY {
		p.next()
		s.Finally = p.suite()
	}
	return s
}

func (p *Parser) importStmt() ast.Stmt {
	pos := p.tok.Pos
	p.next()
	s := &ast.ImportStmt{Pos_: pos}
	for {
		item := ast.ImportItem{}
		item.Name = p.dottedName()
		if p.tok.Type == lexer.AS {
			p.next()
			item.Alias = p.expect(lexer.IDENT).Lit
		}
		s.Items = append(s.Items, item)
		if p.tok.Type != lexer.COMMA {
			break
		}
		p.next()
	}
	p.stmtEnd()
	return s
}

func (p *Parser) fromImportStmt() ast.Stmt {
	pos := p.tok.Pos
	p.next()
	s := &ast.FromImportStmt{Pos_: pos}
	for p.tok.Type == lexer.DOT {
		s.Module += "."
		p.next()
	}
	if p.tok.Type == lexer.IDENT {
		s.Module += p.dottedName()
	}
	p.expect(lexer.IMPORT)
	if p.tok.Type == lexer.STAR {
		p.next()
		s.Items = append(s.Items, ast.ImportItem{Name: "*"})
	} else {
		for {
			if p.tok.Type == lexer.STAR {
				p.next()
				s.Items = append(s.Items, ast.ImportItem{Name: "*"})
				break
			}
			item := ast.ImportItem{}
			item.Name = p.expect(lexer.IDENT).Lit
			if p.tok.Type == lexer.AS {
				p.next()
				item.Alias = p.expect(lexer.IDENT).Lit
			}
			s.Items = append(s.Items, item)
			if p.tok.Type != lexer.COMMA {
				break
			}
			p.next()
		}
	}
	p.stmtEnd()
	return s
}

func (p *Parser) dottedName() string {
	var parts []string
	for {
		parts = append(parts, p.expect(lexer.IDENT).Lit)
		if p.tok.Type != lexer.DOT {
			break
		}
		p.next()
	}
	return strings.Join(parts, ".")
}

func (p *Parser) decoratedStmt() ast.Stmt {
	var decs []ast.Expr
	for p.tok.Type == lexer.AT {
		p.next()
		decs = append(decs, p.parseTestList())
		if p.tok.Type == lexer.NEWLINE {
			p.next()
			continue
		}
		p.stmtEnd()
		break
	}
	switch p.tok.Type {
	case lexer.DEF:
		return p.funcDef(decs)
	case lexer.CLASS:
		return p.classDef(decs)
	}
	p.errorf(p.tok.Pos, "装饰器后必须跟随 def 或 class")
	return nil
}

func (p *Parser) suite() []ast.Stmt {
	p.expect(lexer.COLON)
	if p.tok.Type == lexer.NEWLINE {
		p.next()
		p.expect(lexer.INDENT)
		var stmts []ast.Stmt
		for p.tok.Type != lexer.DEDENT && p.tok.Type != lexer.EOF && p.err == nil {
			if s := p.statement(); s != nil {
				stmts = append(stmts, s)
			}
		}
		p.expect(lexer.DEDENT)
		return stmts
	}
	var stmts []ast.Stmt
	for {
		if s := p.simpleStmt(); s != nil {
			stmts = append(stmts, s)
		}
		if p.tok.Type != lexer.SEMICOLON {
			break
		}
		p.next()
		if p.atEnd() {
			break
		}
	}
	return stmts
}

func (p *Parser) parseTestList() ast.Expr {
	pos := p.tok.Pos
	first := p.testItem()
	if p.tok.Type != lexer.COMMA {
		return first
	}
	t := &ast.TupleLit{Pos_: pos, Elems: []ast.Expr{first}}
	for p.tok.Type == lexer.COMMA {
		p.next()
		if p.atEnd() || p.tok.Type == lexer.COLON || p.tok.Type == lexer.RPAREN ||
			p.tok.Type == lexer.RBRACKET || p.tok.Type == lexer.RBRACE {
			break
		}
		t.Elems = append(t.Elems, p.testItem())
	}
	return t
}

func (p *Parser) testItem() ast.Expr {
	if p.tok.Type == lexer.STAR {
		pos := p.tok.Pos
		p.next()
		x := p.parseTest()
		return &ast.UnaryOpExpr{Pos_: pos, Op: "*", X: x}
	}
	return p.parseTest()
}

func (p *Parser) parseExprList() ast.Expr {
	pos := p.tok.Pos
	first := p.exprItem()
	if p.tok.Type != lexer.COMMA {
		return first
	}
	t := &ast.TupleLit{Pos_: pos, Elems: []ast.Expr{first}}
	for p.tok.Type == lexer.COMMA {
		p.next()
		if p.atEnd() || p.tok.Type == lexer.COLON {
			break
		}
		t.Elems = append(t.Elems, p.exprItem())
	}
	return t
}

func (p *Parser) exprItem() ast.Expr {
	if p.tok.Type == lexer.STAR {
		pos := p.tok.Pos
		p.next()
		x := p.parseExpr()
		return &ast.UnaryOpExpr{Pos_: pos, Op: "*", X: x}
	}
	return p.parseExpr()
}

func (p *Parser) parseTest() ast.Expr {
	x := p.parseOrTest()
	if p.tok.Type == lexer.IF {
		pos := x.Pos()
		p.next()
		cond := p.parseOrTest()
		p.expect(lexer.ELSE)
		y := p.parseTest()
		return &ast.CondExpr{Pos_: pos, Cond: cond, Then: x, Else: y}
	}
	return x
}

func (p *Parser) parseExpr() ast.Expr {
	return p.parseBitOr()
}

func (p *Parser) parseOrTest() ast.Expr {
	x := p.parseAndTest()
	for p.tok.Type == lexer.OR {
		pos := p.tok.Pos
		op := p.tok.Lit
		p.next()
		y := p.parseAndTest()
		x = &ast.BoolOpExpr{Pos_: pos, Op: op, X: x, Y: y}
	}
	return x
}

func (p *Parser) parseAndTest() ast.Expr {
	x := p.parseNotTest()
	for p.tok.Type == lexer.AND {
		pos := p.tok.Pos
		op := p.tok.Lit
		p.next()
		y := p.parseNotTest()
		x = &ast.BoolOpExpr{Pos_: pos, Op: op, X: x, Y: y}
	}
	return x
}

func (p *Parser) parseNotTest() ast.Expr {
	if p.tok.Type == lexer.NOT {
		pos := p.tok.Pos
		p.next()
		x := p.parseNotTest()
		return &ast.UnaryOpExpr{Pos_: pos, Op: "not", X: x}
	}
	return p.parseComparison()
}

func (p *Parser) parseComparison() ast.Expr {
	x := p.parseBitOr()
	if !p.isCompOp() {
		return x
	}
	c := &ast.CompareExpr{Pos_: x.Pos(), X: x}
	for p.isCompOp() {
		op := p.compOp()
		c.Ops = append(c.Ops, op)
		c.Ys = append(c.Ys, p.parseBitOr())
	}
	return c
}

func (p *Parser) isCompOp() bool {
	switch p.tok.Type {
	case lexer.LT, lexer.GT, lexer.LE, lexer.GE, lexer.EQEQ, lexer.NE, lexer.IN, lexer.IS, lexer.NOT:
		return true
	}
	return false
}

func (p *Parser) compOp() string {
	switch p.tok.Type {
	case lexer.LT:
		p.next()
		return "<"
	case lexer.GT:
		p.next()
		return ">"
	case lexer.LE:
		p.next()
		return "<="
	case lexer.GE:
		p.next()
		return ">="
	case lexer.EQEQ:
		p.next()
		return "=="
	case lexer.NE:
		p.next()
		return "!="
	case lexer.IN:
		p.next()
		return "in"
	case lexer.IS:
		p.next()
		if p.tok.Type == lexer.NOT {
			p.next()
			return "is not"
		}
		return "is"
	case lexer.NOT:
		p.next()
		p.expect(lexer.IN)
		return "not in"
	}
	return ""
}

func (p *Parser) parseBitOr() ast.Expr {
	x := p.parseBitXor()
	for p.tok.Type == lexer.PIPE {
		pos := p.tok.Pos
		p.next()
		y := p.parseBitXor()
		x = &ast.BinOpExpr{Pos_: pos, Op: "|", X: x, Y: y}
	}
	return x
}

func (p *Parser) parseBitXor() ast.Expr {
	x := p.parseBitAnd()
	for p.tok.Type == lexer.CARET {
		pos := p.tok.Pos
		p.next()
		y := p.parseBitAnd()
		x = &ast.BinOpExpr{Pos_: pos, Op: "^", X: x, Y: y}
	}
	return x
}

func (p *Parser) parseBitAnd() ast.Expr {
	x := p.parseShift()
	for p.tok.Type == lexer.AMP {
		pos := p.tok.Pos
		p.next()
		y := p.parseShift()
		x = &ast.BinOpExpr{Pos_: pos, Op: "&", X: x, Y: y}
	}
	return x
}

func (p *Parser) parseShift() ast.Expr {
	x := p.parseArith()
	for p.tok.Type == lexer.SHL || p.tok.Type == lexer.SHR {
		pos := p.tok.Pos
		op := p.tok.Lit
		p.next()
		y := p.parseArith()
		x = &ast.BinOpExpr{Pos_: pos, Op: op, X: x, Y: y}
	}
	return x
}

func (p *Parser) parseArith() ast.Expr {
	x := p.parseTerm()
	for p.tok.Type == lexer.PLUS || p.tok.Type == lexer.MINUS {
		pos := p.tok.Pos
		op := p.tok.Lit
		p.next()
		y := p.parseTerm()
		x = &ast.BinOpExpr{Pos_: pos, Op: op, X: x, Y: y}
	}
	return x
}

func (p *Parser) parseTerm() ast.Expr {
	x := p.parseFactor()
	for p.tok.Type == lexer.STAR || p.tok.Type == lexer.SLASH || p.tok.Type == lexer.FLOORDIV || p.tok.Type == lexer.PERCENT {
		pos := p.tok.Pos
		op := p.tok.Lit
		p.next()
		y := p.parseFactor()
		x = &ast.BinOpExpr{Pos_: pos, Op: op, X: x, Y: y}
	}
	return x
}

func (p *Parser) parseFactor() ast.Expr {
	if p.tok.Type == lexer.PLUS || p.tok.Type == lexer.MINUS || p.tok.Type == lexer.TILDE {
		pos := p.tok.Pos
		op := p.tok.Lit
		p.next()
		x := p.parseFactor()
		return &ast.UnaryOpExpr{Pos_: pos, Op: op, X: x}
	}
	return p.parsePower()
}

func (p *Parser) parsePower() ast.Expr {
	x := p.parseAtomExpr()
	if p.tok.Type == lexer.DOUBLESTAR {
		pos := p.tok.Pos
		p.next()
		y := p.parseFactor()
		x = &ast.BinOpExpr{Pos_: pos, Op: "**", X: x, Y: y}
	}
	return x
}

func (p *Parser) parseAtomExpr() ast.Expr {
	x := p.parseAtom()
	for {
		switch p.tok.Type {
		case lexer.LPAREN:
			x = p.callArgs(x)
		case lexer.LBRACKET:
			x = p.subscript(x)
		case lexer.DOT:
			p.next()
			name := p.expect(lexer.IDENT)
			x = &ast.AttrExpr{Pos_: name.Pos, X: x, Name: name.Lit}
		default:
			return x
		}
	}
}

func (p *Parser) callArgs(f ast.Expr) ast.Expr {
	pos := p.tok.Pos
	p.next()
	c := &ast.CallExpr{Pos_: pos, Func: f}
	for p.tok.Type != lexer.RPAREN {
		if p.tok.Type == lexer.STAR {
			p.next()
			c.Star = p.parseTest()
		} else if p.tok.Type == lexer.DOUBLESTAR {
			p.next()
			c.DblStar = p.parseTest()
		} else {
			e := p.parseTest()
			if p.tok.Type == lexer.ASSIGN {
				name, ok := e.(*ast.Name)
				if !ok {
					p.errorf(e.Pos(), "关键字参数名必须是标识符")
					p.next()
					continue
				}
				p.next()
				c.Kwargs = append(c.Kwargs, ast.KwArg{Pos_: name.Pos_, Name: name.Name, Value: p.parseTest()})
			} else {
				c.Args = append(c.Args, e)
			}
		}
		if p.tok.Type != lexer.COMMA {
			break
		}
		p.next()
	}
	p.expect(lexer.RPAREN)
	return c
}

func (p *Parser) subscript(x ast.Expr) ast.Expr {
	pos := p.tok.Pos
	p.next()
	index := p.parseSubscriptIndex()
	p.expect(lexer.RBRACKET)
	return &ast.SubscriptExpr{Pos_: pos, X: x, Index: index}
}

func (p *Parser) parseSubscriptIndex() ast.Expr {
	pos := p.tok.Pos
	var lo ast.Expr
	if p.tok.Type != lexer.COLON && p.tok.Type != lexer.RBRACKET && p.tok.Type != lexer.COMMA {
		lo = p.parseTest()
	}
	if p.tok.Type == lexer.COLON {
		p.next()
		s := &ast.SliceExpr{Pos_: pos, Lo: lo}
		if p.tok.Type != lexer.COLON && p.tok.Type != lexer.RBRACKET && p.tok.Type != lexer.COMMA {
			s.Hi = p.parseTest()
		}
		if p.tok.Type == lexer.COLON {
			p.next()
			if p.tok.Type != lexer.RBRACKET && p.tok.Type != lexer.COMMA {
				s.Step = p.parseTest()
			}
		}
		return s
	}
	if p.tok.Type == lexer.COMMA {
		t := &ast.TupleLit{Pos_: pos, Elems: []ast.Expr{lo}, Paren: false}
		for p.tok.Type == lexer.COMMA {
			p.next()
			if p.tok.Type == lexer.RBRACKET {
				break
			}
			t.Elems = append(t.Elems, p.parseTest())
		}
		return t
	}
	return lo
}

func (p *Parser) parseAtom() ast.Expr {
	t := p.tok
	switch t.Type {
	case lexer.IDENT:
		p.next()
		return &ast.Name{Pos_: t.Pos, Name: t.Lit}
	case lexer.INT:
		p.next()
		return &ast.IntLit{Pos_: t.Pos, Value: t.Lit}
	case lexer.FLOAT:
		p.next()
		return &ast.FloatLit{Pos_: t.Pos, Value: t.Lit}
	case lexer.STRING:
		var parts []string
		for p.tok.Type == lexer.STRING {
			parts = append(parts, p.tok.Lit)
			p.next()
		}
		return &ast.StringLit{Pos_: t.Pos, Value: strings.Join(parts, " ")}
	case lexer.NONE:
		p.next()
		return &ast.Name{Pos_: t.Pos, Name: "None"}
	case lexer.TRUE:
		p.next()
		return &ast.Name{Pos_: t.Pos, Name: "True"}
	case lexer.FALSE:
		p.next()
		return &ast.Name{Pos_: t.Pos, Name: "False"}
	case lexer.ELLIPSIS:
		p.next()
		return &ast.EllipsisLit{Pos_: t.Pos}
	case lexer.LPAREN:
		p.next()
		var e ast.Expr
		if p.tok.Type == lexer.RPAREN {
			e = &ast.TupleLit{Pos_: t.Pos, Paren: true}
		} else {
			e = p.parseTestList()
			if tup, ok := e.(*ast.TupleLit); ok {
				tup.Paren = true
			}
		}
		p.expect(lexer.RPAREN)
		return e
	case lexer.LBRACKET:
		return p.listAtom(t.Pos)
	case lexer.LBRACE:
		return p.dictSetAtom(t.Pos)
	}
	p.errorf(t.Pos, "期望表达式，实际为 %q", t.Lit)
	return nil
}

func (p *Parser) listAtom(pos ast.Position) ast.Expr {
	p.next()
	if p.tok.Type == lexer.RBRACKET {
		p.next()
		return &ast.ListLit{Pos_: pos}
	}
	first := p.parseTest()
	if p.tok.Type == lexer.FOR {
		l := &ast.ListComp{Pos_: pos, Elem: first}
		for p.tok.Type == lexer.FOR {
			p.next()
			var cl ast.CompClause
			cl.Target = p.parseExprList()
			p.expect(lexer.IN)
			cl.Iter = p.parseOrTest()
			for p.tok.Type == lexer.IF {
				p.next()
				cl.Ifs = append(cl.Ifs, p.parseOrTest())
			}
			l.Clauses = append(l.Clauses, cl)
		}
		p.expect(lexer.RBRACKET)
		return l
	}
	l := &ast.ListLit{Pos_: pos, Elems: []ast.Expr{first}}
	for p.tok.Type == lexer.COMMA {
		p.next()
		if p.tok.Type == lexer.RBRACKET {
			break
		}
		l.Elems = append(l.Elems, p.parseTest())
	}
	p.expect(lexer.RBRACKET)
	return l
}

func (p *Parser) dictSetAtom(pos ast.Position) ast.Expr {
	p.next()
	if p.tok.Type == lexer.RBRACE {
		p.next()
		return &ast.DictLit{Pos_: pos}
	}
	if p.tok.Type == lexer.DOUBLESTAR {
		p.errorf(p.tok.Pos, "字典解包 {} 暂不支持")
		p.next()
		p.parseTest()
		p.expect(lexer.RBRACE)
		return &ast.DictLit{Pos_: pos}
	}
	first := p.parseTest()
	if p.tok.Type == lexer.FOR {
		p.errorf(p.tok.Pos, "字典/集合推导式暂不支持")
		for p.tok.Type != lexer.RBRACE && p.tok.Type != lexer.EOF {
			p.next()
		}
		p.expect(lexer.RBRACE)
		return &ast.SetLit{Pos_: pos, Elems: []ast.Expr{first}}
	}
	if p.tok.Type == lexer.COLON {
		p.next()
		d := &ast.DictLit{Pos_: pos, Keys: []ast.Expr{first}}
		d.Vals = append(d.Vals, p.parseTest())
		for p.tok.Type == lexer.COMMA {
			p.next()
			if p.tok.Type == lexer.RBRACE {
				break
			}
			d.Keys = append(d.Keys, p.parseTest())
			p.expect(lexer.COLON)
			d.Vals = append(d.Vals, p.parseTest())
		}
		p.expect(lexer.RBRACE)
		return d
	}
	s := &ast.SetLit{Pos_: pos, Elems: []ast.Expr{first}}
	for p.tok.Type == lexer.COMMA {
		p.next()
		if p.tok.Type == lexer.RBRACE {
			break
		}
		s.Elems = append(s.Elems, p.parseTest())
	}
	p.expect(lexer.RBRACE)
	return s
}
