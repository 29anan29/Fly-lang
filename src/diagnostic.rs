// diagnostic.rs：Rust CLI 的诊断模型——Position/Diagnostic 展示与错误码映射。
// 错误码表与 Go 版 internal/ast/errors.go 的 codeForFormat 双份同步
// （新增 errorf 消息必须两侧各登记一条）。
use std::fmt;

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub struct Position {
    pub line: u32,
    pub col: u32,
}

impl fmt::Display for Position {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}:{}", self.line, self.col)
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Diagnostic {
    pub pos: Position,
    pub code: String,
    pub msg: String,
}

impl fmt::Display for Diagnostic {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        if self.code.is_empty() {
            write!(f, "{}: {}", self.pos, self.msg)
        } else {
            write!(f, "{}: error[{}]: {}", self.pos, self.code, self.msg)
        }
    }
}

// ERROR_CODES 格式化消息 → 错误码（E0001 起连续编号，见 Go 版错误码注册表）。
// 遍历查找：诊断路径低频，顺序扫描 77 条的开销可忽略。
static ERROR_CODES: &[(&str, &str)] = &[
    ("期望 %s，实际为 %s", "E0001"),
    ("期望表达式，实际为 %q", "E0002"),
    ("期望语句结束，实际为 %q", "E0003"),
    ("关键字 %s 暂不支持", "E0004"),
    ("关键字参数名必须是标识符", "E0005"),
    ("非法赋值目标", "E0006"),
    ("装饰器后必须跟随 def 或 class", "E0007"),
    ("字典/集合推导式暂不支持", "E0008"),
    ("字典解包 {} 暂不支持", "E0009"),
    ("lock 需要一个变量名", "E0010"),
    ("seal 后必须跟随 class 定义", "E0011"),
    ("level 必须是字符串（如 \"WARN\"）", "E0012"),
    ("args 必须是 True 或 False", "E0013"),
    ("ret 必须是 True 或 False", "E0014"),
    ("trace 参数 %s 未知（支持 level/args/ret）", "E0015"),
    ("cage 参数 %s 必须是字符串（如 max_time=\"5s\"）", "E0016"),
    ("max_time 格式非法：%q（支持 500ms/5s/2m/1h）", "E0017"),
    ("max_memory 格式非法：%q（支持 64KB/100MB/2GB）", "E0018"),
    ("cage 参数 %s 未知（支持 max_time/max_memory）", "E0019"),
    ("cage 需至少指定 max_time 或 max_memory 之一", "E0020"),
    ("未闭合的括号", "E0021"),
    ("行首不支持制表符缩进", "E0022"),
    ("缩进级别与上层不一致", "E0023"),
    ("字符串未闭合", "E0024"),
    ("指数部分缺少数字: %s", "E0025"),
    ("非法数字字面量 %s", "E0026"),
    ("意外的字符 %q", "E0027"),
    ("意外的字符 '%s'", "E0027"),
    ("多余的右括号 '%s'", "E0028"),
    ("多余的右括号 ')'", "E0028"),
    ("多余的右括号 ']'", "E0028"),
    ("多余的右括号 '}'", "E0028"),
    ("名字 %s 重复定义（第一次在第 %d 行）", "E0029"),
    ("函数参数 %s 重复定义", "E0030"),
    ("未净化的外部输入 %s 流入 %s（危险汇点）", "E0031"),
    ("未净化的外部输入 %s 流入函数 %s 的参数 %s（危险汇点）", "E0031"),
    ("敏感数据 %s 不可流入 %s（输出上下文）", "E0032"),
    ("敏感数据 %s 流入函数 %s 的参数 %s（输出上下文）", "E0032"),
    ("lock 变量 %s 未定义", "E0033"),
    ("lock 变量 %s 不可删除", "E0034"),
    ("lock 变量 %s 不可再赋值", "E0035"),
    ("lock 变量 %s 不可通过反射修改", "E0036"),
    ("lock 变量 %s 不可通过 setattr 修改", "E0037"),
    ("lock 变量 %s 不可通过 %s() 反射读取", "E0038"),
    ("guard 变量 %s 未定义", "E0039"),
    ("guard 类型必须是简单类型名（如 int、str）", "E0040"),
    ("guard 类型 %s 与参数注解 %s 不一致", "E0041"),
    ("guard 至少需要一个类型或条件", "E0042"),
    ("guard 条件中引用了未定义的名字 %s", "E0043"),
    ("only 白名单不能为空（需至少一个模块，如 only (json):）", "E0044"),
    ("only 块禁止访问 %s（不在白名单 %v）", "E0045"),
    ("seal 类实例 %s 的属性 %s 不可修改", "E0046"),
    ("trace 级别 %s 非法（支持 DEBUG/INFO/WARNING/ERROR/CRITICAL）", "E0047"),
    ("trace 块内函数名 %s 不能以 _fly_ 开头（保留前缀）", "E0048"),
    ("trace 块内参数名 %s 不能以 _fly_ 开头（保留前缀）", "E0049"),
    ("未定义的名字 %s（safe 需要先赋值）", "E0050"),
    ("未定义的名字 %s（mask 需要先赋值）", "E0051"),
    ("未定义的名字 %s", "E0052"),
    ("函数 %s 需要至少 %d 个参数（实际 %d 个）", "E0053"),
    ("函数 %s 最多接受 %d 个位置参数（实际 %d 个）", "E0054"),
    ("函数 %s 没有名为 %s 的参数", "E0055"),
    ("函数 %s 参数 %s 重复传值", "E0056"),
    ("常量表达式除数为零", "E0057"),
    ("运算符 %s 不支持 %s 与 %s", "E0058"),
    ("运算符 ~ 不支持 %s", "E0059"),
    ("运算符 %s 不支持 %s", "E0060"),
    ("in 右侧不支持 %s", "E0061"),
    ("%s 不可下标访问", "E0062"),
    ("禁止调用内建 %s（沙箱逃逸风险）", "E0063"),
    ("禁止反射访问属性 %s（沙箱逃逸风险）", "E0064"),
    ("禁止反射下标访问 %s（沙箱逃逸风险）", "E0064"),
    ("禁止访问模块属性 %s（沙箱逃逸风险）", "E0064"),
    ("f-string 内禁止反射访问 %s（沙箱逃逸风险）", "E0064"),
    ("f-string 内禁止访问内建 %s（沙箱逃逸风险）", "E0063"),
    ("禁止访问 __builtins__（沙箱逃逸风险）", "E0065"),
    ("禁止导入危险模块 %s（沙箱逃逸风险）", "E0066"),
];

/// 格式化消息 → 错误码；未登记返回空串（调用方走无码诊断）。
pub fn error_code(format: &str) -> &'static str {
    ERROR_CODES
        .iter()
        .find(|(f, _)| *f == format)
        .map(|(_, c)| *c)
        .unwrap_or("")
}
