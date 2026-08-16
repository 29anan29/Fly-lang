// lock.go：lock 编译期检查——变量不可再赋值/删除/反射修改（E0033-E0038）。
package checker

import "pyfly/internal/ast"

func (c *Checker) checkLock(t *ast.LockStmt) {
	if t.Value != nil {
		c.cur.Define(t.Name, &Symbol{Kind: KVar, Pos: t.Pos_})
	} else if _, ok := c.cur.Lookup(t.Name); !ok {
		c.errorf(t.Pos_, "lock 变量 %s 未定义", t.Name)
		return
	}
	if _, ok := c.locked[t.Name]; !ok {
		c.locked[t.Name] = t.Pos_
	}
}
