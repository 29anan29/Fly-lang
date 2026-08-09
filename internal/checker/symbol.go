package checker

import "flylang/internal/ast"

type Kind int

const (
	KVar Kind = iota
	KFunc
	KClass
	KParam
	KImport
)

type Symbol struct {
	Kind Kind
	Pos  ast.Position
	Anno ast.Expr
}

type Scope struct {
	Parent *Scope
	Names  map[string]*Symbol
}

func NewScope(parent *Scope) *Scope {
	return &Scope{Parent: parent, Names: make(map[string]*Symbol)}
}

func (s *Scope) Define(name string, sym *Symbol) {
	if _, ok := s.Names[name]; !ok {
		s.Names[name] = sym
	}
}

func (s *Scope) Lookup(name string) (*Symbol, bool) {
	for sc := s; sc != nil; sc = sc.Parent {
		if sym, ok := sc.Names[name]; ok {
			return sym, true
		}
	}
	return nil, false
}

var builtins = map[string]bool{
	"None": true, "True": true, "False": true,
	"abs": true, "all": true, "any": true, "bin": true, "bool": true,
	"bytearray": true, "bytes": true, "callable": true, "chr": true,
	"classmethod": true, "compile": true, "complex": true, "delattr": true,
	"dict": true, "dir": true, "divmod": true, "enumerate": true, "eval": true,
	"exec": true, "filter": true, "float": true, "format": true, "frozenset": true,
	"getattr": true, "globals": true, "hasattr": true, "hash": true, "hex": true,
	"id": true, "input": true, "int": true, "isinstance": true, "issubclass": true,
	"iter": true, "len": true, "list": true, "locals": true, "map": true,
	"max": true, "memoryview": true, "min": true, "next": true, "object": true,
	"oct": true, "open": true, "ord": true, "pow": true, "print": true,
	"property": true, "range": true, "repr": true, "reversed": true, "round": true,
	"set": true, "setattr": true, "slice": true, "sorted": true,
	"staticmethod": true, "str": true, "sum": true, "super": true, "tuple": true,
	"type": true, "vars": true, "zip": true,
}
