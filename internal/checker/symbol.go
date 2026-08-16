// symbol.go：符号表——作用域、名字解析、污点传播（含 x.attr 属性污点，兑现威胁模型 R2）。
package checker

import "pyfly/internal/ast"

type Kind int

const (
	KVar Kind = iota
	KFunc
	KClass
	KParam
	KImport
)

type Symbol struct {
	Kind       Kind
	Pos        ast.Position
	Anno       ast.Expr
	Taint      Taint
	Attrs      map[string]Taint // 实例属性污点（obj.attr = tainted 传播）
	Seal       bool
	Func       *ast.FuncDef
	Module     string // KImport：顶层模块名（import pickle → "pickle"）
	Orig       string // KImport：模块内原名（from pickle import loads as l → "loads"）
	Scope      *Scope // KClass：类作用域（方法查找）
	Params     []string
	SinkParams map[string]bool
	RetParam   bool
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
	"Exception": true, "BaseException": true, "ValueError": true, "TypeError": true,
	"KeyError": true, "RuntimeError": true, "AttributeError": true, "IndexError": true,
	"ZeroDivisionError": true, "StopIteration": true, "AssertionError": true,
	"NameError": true, "ImportError": true, "KeyboardInterrupt": true,
	"NotImplemented": true, "OverflowError": true, "ArithmeticError": true,
	"LookupError": true, "OSError": true, "FileNotFoundError": true,
	"IOError": true, "MemoryError": true, "UnicodeError": true, "SystemExit": true,
	"GeneratorExit": true, "EOFError": true, "FloatingPointError": true,
	"IndentationError": true, "SyntaxError": true, "NotImplementedError": true,
	"RecursionError": true, "UnboundLocalError": true, "EnvironmentError": true,
	"__name__": true, "__file__": true, "__doc__": true, "__builtins__": true,
	"TimeoutError": true, "GuardError": true, "FlyRuntimeError": true,
	"ResourceExhaustedError": true,
}
