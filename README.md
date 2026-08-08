<div align="center">

<svg
  xmlns="http://www.w3.org/2000/svg"
  viewBox="0 0 512 512"
  width="128"
  height="128"
  role="img"
  aria-labelledby="title desc"
>
  <title id="title">Geometric F Logo</title>
  <desc id="desc">
    An original layered geometric letter F logo in emerald green and dark blue.
  </desc>

  <!-- 外层：绿色折叠轮廓 -->
  <path
    fill="#35C98B"
    d="
      M72 64
      H440
      L386 158
      H232
      V218
      H350
      L298 308
      H232
      V448
      H126
      V158
      Z
    "
  />

  <!-- 内层：深蓝色 F -->
  <path
    fill="#183B56"
    d="
      M126 64
      H350
      L302 146
      H214
      V238
      H300
      L258 310
      H214
      V384
      L174 448
      V128
      Z
    "
  />

  <!-- 中心高光切面 -->
  <path
    fill="#A7F3D0"
    d="
      M126 64
      H174
      V448
      L126 416
      V158
      L72 64
      Z
    "
    opacity="0.92"
  />
</svg>

# Fly-Lang

**Python 3.10+ 的安全增强超集转译器**（Go 实现，Fly → Python，类似 TypeScript → JavaScript）

</div>

Fly 是 Python 3.10+ 的安全增强超集：用 Go 实现的转译器，把 `.fly` 源码转译为纯净 Python。新增 8 个安全关键字，在编译期静态检查 + 展开删除，零运行时残留语法。

详细设计见 [方案.md](方案.md)（语言设计）与 [Plan.md](Plan.md)（实现方案）。

## 构建

```bash
go build -o fly ./cmd/fly
```

## 用法

```
./fly build [选项] <file.fly>   转译为 Python
./fly check <file.fly>          仅编译检查（出错退出码 1）
./fly run <file.fly>            转译并执行（python3）
```

build 选项：

```
-o <out.py>   指定输出文件（默认与源文件同名 .py）
```

示例：

```bash
./fly build testdata/golden/hello.fly -o out.py
python3 out.py
./fly check app.fly
./fly run testdata/golden/basic.fly
```

## 8 个安全关键字

| 关键字 | 含义 | 防御目标 |
| :--- | :--- | :--- |
| `safe` | 强制净化污点变量 | SQL/命令/代码注入（编译期污点追踪） |
| `only` | 白名单权限块 | 恶意模块调用（编译期 + `__builtins__` 代理） |
| `lock` | 锁定常量 + 防反射读取 | 常量篡改、`globals()` 泄露（编译期符号表拦截） |
| `mask` | 遮蔽敏感数据 | 密码/token 经日志打印泄露（编译期输出上下文检测） |
| `cage` | 限制资源（内存/CPU/时间） | 无限循环、大内存分配（运行时 signal + resource 装饰器） |
| `guard` | 强制输入验证（类型/格式/范围） | 未校验外部输入（编译期生成断言） |
| `seal` | 冻结对象，禁止增删改属性 | 对象属性被篡改（编译期生成 `__setattr__` 拦截） |
| `trace` | 强制审计日志 | 关键操作无记录（编译期插入 logging） |

## 报错格式

```
error: <file>.fly:12:5: <消息>
```

## 测试

```bash
go test ./...
go vet ./...
```

- `testdata/golden/`：正例，转译输出与同名 `.py` golden 对比
- `testdata/errors/`：反例，必须编译报错
- 生成的 `.py` 需 Python 3.10+ 可运行

## 目录结构

```
cmd/fly/           CLI 入口（build/check/run）
internal/lexer/    词法分析
internal/ast/      AST 节点（带位置信息）
internal/parser/   递归下降解析器
internal/checker/  编译期语义检查（P1 起）
internal/gen/      代码生成 + 运行时注入
internal/runtime/  fly_runtime.py 运行时支持库（P4 起）
editor/vscode-fly/ VSCode 插件（P5 起）
testdata/          正反例测试文件
```
