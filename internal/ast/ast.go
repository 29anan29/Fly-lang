package ast

type Module struct {
	Pos_  Position
	Stmts []Stmt
}

func (m *Module) Pos() Position { return m.Pos_ }

type ImportItem struct {
	Name  string
	Alias string
}

type ImportStmt struct {
	Pos_  Position
	Items []ImportItem
}

func (s *ImportStmt) Pos() Position { return s.Pos_ }
func (*ImportStmt) stmtNode()       {}

type FromImportStmt struct {
	Pos_   Position
	Module string
	Items  []ImportItem
}

func (s *FromImportStmt) Pos() Position { return s.Pos_ }
func (*FromImportStmt) stmtNode()       {}

type LockStmt struct {
	Pos_  Position
	Name  string
	Value Expr
}

func (s *LockStmt) Pos() Position { return s.Pos_ }
func (*LockStmt) stmtNode()       {}

type SafeStmt struct {
	Pos_  Position
	Names []string
}

func (s *SafeStmt) Pos() Position { return s.Pos_ }
func (*SafeStmt) stmtNode()       {}

type MaskStmt struct {
	Pos_  Position
	Names []string
}

func (s *MaskStmt) Pos() Position { return s.Pos_ }
func (*MaskStmt) stmtNode()       {}

type GuardStmt struct {
	Pos_  Position
	Name  string
	Type  Expr
	Conds []Expr
}

func (s *GuardStmt) Pos() Position { return s.Pos_ }
func (*GuardStmt) stmtNode()       {}

type AssignStmt struct {
	Pos_  Position
	Left  []Expr
	Op    string
	Right Expr
}

func (s *AssignStmt) Pos() Position { return s.Pos_ }
func (*AssignStmt) stmtNode()       {}

type ExprStmt struct {
	Pos_ Position
	X    Expr
}

func (s *ExprStmt) Pos() Position { return s.Pos_ }
func (*ExprStmt) stmtNode()       {}

type Param struct {
	Name    string
	Anno    Expr
	Default Expr
	Star    bool
	DblStar bool
}

type FuncDef struct {
	Pos_       Position
	Name       string
	Params     []Param
	ReturnType Expr
	Body       []Stmt
	Decorators []Expr
}

func (s *FuncDef) Pos() Position { return s.Pos_ }
func (*FuncDef) stmtNode()       {}

type ClassDef struct {
	Pos_       Position
	Name       string
	Bases      []Expr
	Body       []Stmt
	Decorators []Expr
	Seal       bool
}

func (s *ClassDef) Pos() Position { return s.Pos_ }
func (*ClassDef) stmtNode()       {}

type OnlyStmt struct {
	Pos_    Position
	Modules []string
	Body    []Stmt
}

func (s *OnlyStmt) Pos() Position { return s.Pos_ }
func (*OnlyStmt) stmtNode()       {}

type TraceStmt struct {
	Pos_  Position
	Level string
	Args  bool
	Ret   bool
	Body  []Stmt
}

func (s *TraceStmt) Pos() Position { return s.Pos_ }
func (*TraceStmt) stmtNode()       {}

type CageStmt struct {
	Pos_      Position
	HasTime   bool
	MaxTime   float64
	HasMem    bool
	MaxMemory int64
	Body      []Stmt
}

func (s *CageStmt) Pos() Position { return s.Pos_ }
func (*CageStmt) stmtNode()       {}

type ElifClause struct {
	Pos_ Position
	Cond Expr
	Body []Stmt
}

type IfStmt struct {
	Pos_  Position
	Cond  Expr
	Then  []Stmt
	Elifs []ElifClause
	Else  []Stmt
}

func (s *IfStmt) Pos() Position { return s.Pos_ }
func (*IfStmt) stmtNode()       {}

type ForStmt struct {
	Pos_   Position
	Target Expr
	Iter   Expr
	Body   []Stmt
	Else   []Stmt
}

func (s *ForStmt) Pos() Position { return s.Pos_ }
func (*ForStmt) stmtNode()       {}

type WhileStmt struct {
	Pos_ Position
	Cond Expr
	Body []Stmt
	Else []Stmt
}

func (s *WhileStmt) Pos() Position { return s.Pos_ }
func (*WhileStmt) stmtNode()       {}

type ReturnStmt struct {
	Pos_  Position
	Value Expr
}

func (s *ReturnStmt) Pos() Position { return s.Pos_ }
func (*ReturnStmt) stmtNode()       {}

type RaiseStmt struct {
	Pos_ Position
	Exc  Expr
	From Expr
}

func (s *RaiseStmt) Pos() Position { return s.Pos_ }
func (*RaiseStmt) stmtNode()       {}

type ExceptClause struct {
	Pos_ Position
	Type Expr
	Name string
	Body []Stmt
}

type TryStmt struct {
	Pos_     Position
	Body     []Stmt
	Handlers []ExceptClause
	Else     []Stmt
	Finally  []Stmt
}

func (s *TryStmt) Pos() Position { return s.Pos_ }
func (*TryStmt) stmtNode()       {}

type PassStmt struct {
	Pos_ Position
}

func (s *PassStmt) Pos() Position { return s.Pos_ }
func (*PassStmt) stmtNode()       {}

type BreakStmt struct {
	Pos_ Position
}

func (s *BreakStmt) Pos() Position { return s.Pos_ }
func (*BreakStmt) stmtNode()       {}

type ContinueStmt struct {
	Pos_ Position
}

func (s *ContinueStmt) Pos() Position { return s.Pos_ }
func (*ContinueStmt) stmtNode()       {}

type DeleteStmt struct {
	Pos_    Position
	Targets []Expr
}

func (s *DeleteStmt) Pos() Position { return s.Pos_ }
func (*DeleteStmt) stmtNode()       {}

type Name struct {
	Pos_ Position
	Name string
}

func (e *Name) Pos() Position { return e.Pos_ }
func (*Name) exprNode()       {}

type IntLit struct {
	Pos_  Position
	Value string
}

func (e *IntLit) Pos() Position { return e.Pos_ }
func (*IntLit) exprNode()       {}

type FloatLit struct {
	Pos_  Position
	Value string
}

func (e *FloatLit) Pos() Position { return e.Pos_ }
func (*FloatLit) exprNode()       {}

type StringLit struct {
	Pos_  Position
	Value string
}

func (e *StringLit) Pos() Position { return e.Pos_ }
func (*StringLit) exprNode()       {}

type EllipsisLit struct {
	Pos_ Position
}

func (e *EllipsisLit) Pos() Position { return e.Pos_ }
func (*EllipsisLit) exprNode()       {}

type ListLit struct {
	Pos_  Position
	Elems []Expr
}

func (e *ListLit) Pos() Position { return e.Pos_ }
func (*ListLit) exprNode()       {}

type TupleLit struct {
	Pos_  Position
	Elems []Expr
	Paren bool
}

func (e *TupleLit) Pos() Position { return e.Pos_ }
func (*TupleLit) exprNode()       {}

type DictLit struct {
	Pos_ Position
	Keys []Expr
	Vals []Expr
}

func (e *DictLit) Pos() Position { return e.Pos_ }
func (*DictLit) exprNode()       {}

type SetLit struct {
	Pos_  Position
	Elems []Expr
}

func (e *SetLit) Pos() Position { return e.Pos_ }
func (*SetLit) exprNode()       {}

type KwArg struct {
	Pos_  Position
	Name  string
	Value Expr
}

type CallExpr struct {
	Pos_    Position
	Func    Expr
	Args    []Expr
	Kwargs  []KwArg
	Star    Expr
	DblStar Expr
}

func (e *CallExpr) Pos() Position { return e.Pos_ }
func (*CallExpr) exprNode()       {}

type AttrExpr struct {
	Pos_ Position
	X    Expr
	Name string
}

func (e *AttrExpr) Pos() Position { return e.Pos_ }
func (*AttrExpr) exprNode()       {}

type SliceExpr struct {
	Pos_ Position
	Lo   Expr
	Hi   Expr
	Step Expr
}

func (e *SliceExpr) Pos() Position { return e.Pos_ }
func (*SliceExpr) exprNode()       {}

type SubscriptExpr struct {
	Pos_  Position
	X     Expr
	Index Expr
}

func (e *SubscriptExpr) Pos() Position { return e.Pos_ }
func (*SubscriptExpr) exprNode()       {}

type BinOpExpr struct {
	Pos_ Position
	Op   string
	X    Expr
	Y    Expr
}

func (e *BinOpExpr) Pos() Position { return e.Pos_ }
func (*BinOpExpr) exprNode()       {}

type UnaryOpExpr struct {
	Pos_ Position
	Op   string
	X    Expr
}

func (e *UnaryOpExpr) Pos() Position { return e.Pos_ }
func (*UnaryOpExpr) exprNode()       {}

type BoolOpExpr struct {
	Pos_ Position
	Op   string
	X    Expr
	Y    Expr
}

func (e *BoolOpExpr) Pos() Position { return e.Pos_ }
func (*BoolOpExpr) exprNode()       {}

type CompareExpr struct {
	Pos_ Position
	X    Expr
	Ops  []string
	Ys   []Expr
}

func (e *CompareExpr) Pos() Position { return e.Pos_ }
func (*CompareExpr) exprNode()       {}

type CondExpr struct {
	Pos_ Position
	Cond Expr
	Then Expr
	Else Expr
}

func (e *CondExpr) Pos() Position { return e.Pos_ }
func (*CondExpr) exprNode()       {}

type CompClause struct {
	Target Expr
	Iter   Expr
	Ifs    []Expr
}

type ListComp struct {
	Pos_    Position
	Elem    Expr
	Clauses []CompClause
}

func (e *ListComp) Pos() Position { return e.Pos_ }
func (*ListComp) exprNode()       {}
