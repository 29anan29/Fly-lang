# Fly-Lang 威胁模型

> 面向 AI Agent 与外部输入的 Python 代码，编译期安全转译的威胁模型。

## 资产与信任边界

| 资产 | 保护目标 |
|------|----------|
| Agent 宿主进程 | 不被注入代码 / 不被任意命令执行 / 不被耗死 |
| 宿主数据 | 不被读取外泄（密钥、用户数据、内部状态） |
| 外部系统 | 不被代理 SSRF/命令执行攻击 |
| 产物可用性 | 通过编译的产物不崩溃（动态错误统一兜底） |

信任边界：**源码与产物之间是编译期信任**（源码中的风险代码进不了产物）；
**第三方模块内部是供应链信任**（超出转译器能力，按 pip 生态惯例处理）。

## 攻击者视角（威胁场景）

### T1 反序列化 RCE（pickle）
外部输入直接 `pickle.loads/load/Unpickler` → `__reduce__` 任意代码执行。
**防御**：`safe` 声明 + 污点汇点拦截（含别名/容器传播）；演示见
`examples/cve-pickle/`。**剩余风险**：跨文件导入后的再投递（见边界 B1）。

### T2 动态执行注入（eval/exec/compile）
外部输入拼入 eval/exec/compile 表达式 → 任意代码执行。
**防御**：同上，污点汇点含 eval/exec/compile。

### T3 命令注入（os.system/subprocess/shell）
外部输入拼入 shell 命令 → 任意命令执行。
**防御**：污点汇点含 os.system/popen/spawn*/subprocess；`only` 白名单
可进一步封禁 subprocess 模块。

### T4 敏感数据外泄（mask）
被标注 mask 的数据（密钥/token/用户隐私）流入 print/logging/f-string → 日志/输出外泄。
**防御**：mask 污点 + 输出上下文拦截。**剩余风险**：写文件/网络发送是"输出"
吗？当前静态判定覆盖 print/logging/字符串插值；写文件/请求体请用 only 白名单
封禁或人工评审（边界 B4）。

### T5 资源耗尽（cage）
Agent 被诱导无限循环/超大内存 → 宿主 DoS。
**防御**：cage 函数级 max_time/max_memory（SIGALRM + RLIMIT_AS）。

### T6 数据投毒（外部输入未校验）
外部输入直接进入业务逻辑（类型错误、脏数据）→ 逻辑错误/下游崩溃。
**防御**：`guard` 运行时断言 + `int()/float()/bool()` 清洗（清洗即污点清除）。

### T7 反射篡改（lock/seal 对抗）
通过 globals()/setattr/类实例属性修改绕过常量与封装。
**防御**：lock 反射拦截（编译期拒绝 globals/locals/setattr 触碰锁定名）；
seal 运行时令牌拦截实例属性修改。

## 污点数据流模型

```
源点: input() / request.* / os.environ / safe 声明的参数
传播: 赋值 → 容器(list/dict/set) → 属性 → 下标 → f-string → 函数参数/返回值
清洗: int()/float()/bool() 调用后污点清除（显式净化）
汇点: pickle.loads/load/Unpickler, eval/exec/compile,
      os.system/popen/spawn*, subprocess.*, 输出上下文(print/logging)
```

- 非路径敏感（不做分支条件分析）：`if c: x = tainted; 使用 x` 会被拦截
  （x 的污点在 if 分支内标记）——误报偏安全侧。
- 函数内污点传播按参数-返回签名建模：`def f(a): return a` 的参数污点
  会带出（返回表达式直接引用参数）。**不跟踪跨文件**（边界 B1）。

## 对抗措施有效性评估

| 对抗方式 | 结果 |
|----------|------|
| `from pickle import loads as l; l(x)` 别名 | 拦截（Symbol 溯源源模块） |
| `import pickle as p; p.loads(x)` | 拦截 |
| `__reduce__` 改换成其他 callable | 拦截（汇点是 loads 本身，不是载荷内容） |
| 把 x 放进 list/dict 再取出 | 拦截（容器传播） |
| `int(x)` 清洗后再用 | 放行（显式清洗语义正确） |
| `eval("x")` 动态拼接 | 拦截（eval 是汇点） |
| 反射 getattr(obj, "secret") | 静态部分拦截；动态字符串名需人工评审（B4） |

## 边界（不保证项）

- **B1 跨文件污点**：单文件编译模型，`import` 的其他 .fly 文件不参与污点分析。
- **B2 通配导入**：`from x import *` 引入的名字未定义检查豁免。
- **B3 第三方模块内部**：模块自身代码是供应链信任，不静态审查。
- **B4 动态反射**：`getattr(obj, 动态字符串)`、`globals()[name]` 的动态形式。
- **B5 业务逻辑**：鉴权、权限、加密等业务语义不在安全关键字职责内。
- **B6 运行时兜底是"统一报错"**：不阻止逻辑错误本身（如除以负值无意义），
  只保证错误被捕获并带行列号。

## 结论（一句话）

**Fly-Lang 把"风险代码必须编译失败"作为承诺**：通过 `fly check` 的源码中，
外部输入与危险操作之间必须存在显式清洗。运行时护栏保证"即使出错也带着
源码位置、不裸抛、可控恢复"；剩余风险集中在跨文件与动态反射，
用 `only` 白名单与人工评审补齐。
