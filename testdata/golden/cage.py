import functools as _fly_functools
import resource as _fly_resource
import signal as _fly_signal


class ResourceExhaustedError(RuntimeError):
    """Fly cage 资源超限"""


def _fly_timeout_handler(signum, frame):
    raise TimeoutError("cage: 执行超时")


def _fly_cage(max_time=None, max_memory=None):
    def deco(fn):
        @_fly_functools.wraps(fn)
        def wrapped(*args, **kwargs):
            prev_alarm = None
            prev_rlimit = None
            try:
                if max_time is not None:
                    prev_alarm = _fly_signal.getsignal(_fly_signal.SIGALRM)
                    _fly_signal.signal(_fly_signal.SIGALRM, _fly_timeout_handler)
                    _fly_signal.setitimer(_fly_signal.ITIMER_REAL, max_time)
                if max_memory is not None:
                    prev_rlimit = _fly_resource.getrlimit(_fly_resource.RLIMIT_AS)
                    soft, hard = prev_rlimit
                    if soft == _fly_resource.RLIM_INFINITY or max_memory < soft:
                        soft = max_memory
                    _fly_resource.setrlimit(_fly_resource.RLIMIT_AS, (soft, hard))
                try:
                    return fn(*args, **kwargs)
                except MemoryError:
                    raise ResourceExhaustedError(
                        "cage: 内存超限（限制 %d 字节）" % max_memory
                    )
            finally:
                if max_time is not None:
                    _fly_signal.setitimer(_fly_signal.ITIMER_REAL, 0)
                    _fly_signal.signal(_fly_signal.SIGALRM, prev_alarm)
                if max_memory is not None:
                    _fly_resource.setrlimit(_fly_resource.RLIMIT_AS, prev_rlimit)

        return wrapped

    return deco

import operator as _fly_op

_FLY_OPS = {
    "add": _fly_op.add, "sub": _fly_op.sub, "mul": _fly_op.mul,
    "truediv": _fly_op.truediv, "floordiv": _fly_op.floordiv, "mod": _fly_op.mod,
    "pow": _fly_op.pow, "lshift": _fly_op.lshift, "rshift": _fly_op.rshift,
    "and_": _fly_op.and_, "or_": _fly_op.or_, "xor": _fly_op.xor,
    "matmul": _fly_op.matmul, "neg": _fly_op.neg, "pos": _fly_op.pos,
    "invert": _fly_op.invert, "lt": _fly_op.lt, "le": _fly_op.le,
    "gt": _fly_op.gt, "ge": _fly_op.ge, "contains": _fly_op.contains,
}


class FlyRuntimeError(RuntimeError):
    """Fly 运行时兜底：动态错误统一携带源码行列号"""

    pass


_FLY_SAFE_ERRORS = (
    ZeroDivisionError, TypeError, OverflowError, IndexError,
    KeyError, AttributeError, ValueError,
)


def _fly_loc(line, col):
    return "src:%d:%d" % (line, col)


def _fly_binop(a, b, op, line, col):
    if op == "add":
        if isinstance(a, int) and isinstance(b, int):
            return a + b
        if isinstance(a, str) and isinstance(b, str):
            return a + b
        if isinstance(a, (list, tuple)) and isinstance(b, (list, tuple)):
            return a + b
    elif op == "sub":
        if isinstance(a, int) and isinstance(b, int):
            return a - b
    elif op == "mul":
        if isinstance(a, int) and isinstance(b, int):
            return a * b
    elif op == "mod":
        if isinstance(a, int) and isinstance(b, int) and b != 0:
            return a % b
    elif op == "floordiv":
        if isinstance(a, int) and isinstance(b, int) and b != 0:
            return a // b
    elif op == "lt":
        if isinstance(a, int) and isinstance(b, int):
            return a < b
        if isinstance(a, str) and isinstance(b, str):
            return a < b
    elif op == "le":
        if isinstance(a, int) and isinstance(b, int):
            return a <= b
    elif op == "gt":
        if isinstance(a, int) and isinstance(b, int):
            return a > b
    elif op == "ge":
        if isinstance(a, int) and isinstance(b, int):
            return a >= b
    elif op == "eq":
        if isinstance(a, int) and isinstance(b, int):
            return a == b
    elif op == "ne":
        if isinstance(a, int) and isinstance(b, int):
            return a != b
    try:
        return _FLY_OPS[op](a, b)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 运算 %s 失败: %s" % (_fly_loc(line, col), op, e)
        ) from None


def _fly_unary(x, op, line, col):
    try:
        return _FLY_OPS[op](x)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 运算 %s 失败: %s" % (_fly_loc(line, col), op, e)
        ) from None


def _fly_get(x, k, line, col):
    if isinstance(k, str) and k in _FLY_SB_REFLECT:
        _fly_sb_audit("禁止反射下标访问 " + k, line, col)
        raise FlyRuntimeError(
            "%s: 沙箱: 禁止反射下标访问 %s" % (_fly_loc(line, col), k)
        )
    try:
        return x[k]
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 下标访问失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_set(x, k, v, line, col):
    if isinstance(k, str) and k in _FLY_SB_REFLECT:
        _fly_sb_audit("禁止反射下标赋值 " + k, line, col)
        raise FlyRuntimeError(
            "%s: 沙箱: 禁止反射下标赋值 %s" % (_fly_loc(line, col), k)
        )
    try:
        x[k] = v
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 下标赋值失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_attr(x, name, line, col):
    if name in _FLY_SB_REFLECT:
        _fly_sb_audit("禁止反射访问 " + name, line, col)
        raise FlyRuntimeError(
            "%s: 沙箱: 禁止反射访问 %s" % (_fly_loc(line, col), name)
        )
    if type(x) is _fly_sb_types.ModuleType and (
        name in _FLY_SB_BLOCKED_MODS or name in _FLY_SB_MOD_ATTRS
    ):
        _fly_sb_audit("禁止访问模块属性 " + name, line, col)
        raise FlyRuntimeError("沙箱: 禁止访问模块属性 " + name)
    try:
        return _fly_sb_builtins.getattr(x, name)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性访问 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_setattr(x, name, v, line, col):
    if name in _FLY_SB_REFLECT:
        _fly_sb_audit("禁止反射赋值 " + name, line, col)
        raise FlyRuntimeError(
            "%s: 沙箱: 禁止反射赋值 %s" % (_fly_loc(line, col), name)
        )
    if type(x) is _fly_sb_types.ModuleType and (
        name in _FLY_SB_BLOCKED_MODS or name in _FLY_SB_MOD_ATTRS
    ):
        _fly_sb_audit("禁止访问模块属性 " + name, line, col)
        raise FlyRuntimeError("沙箱: 禁止访问模块属性 " + name)
    try:
        _fly_sb_builtins.setattr(x, name, v)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性赋值 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


# plain 版护栏（gen 类型推断豁免路径）：编译期已保证 name 非反射名单、
# x 非模块（typeinfer 非 Unknown 类型恒非模块）、key 非 str，故移除
# 运行时检查；保留 try/except 错误行列号包装（与 _fly_binop 成功快路径
# + fallback 包装的豁免语义一致——错误路径恒带行列号）。
def _fly_attr_plain(x, name, line, col):
    try:
        return _fly_sb_builtins.getattr(x, name)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性访问 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_setattr_plain(x, name, v, line, col):
    try:
        _fly_sb_builtins.setattr(x, name, v)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性赋值 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_get_plain(x, k, line, col):
    try:
        return x[k]
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 下标访问失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_set_plain(x, k, v, line, col):
    try:
        x[k] = v
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 下标赋值失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_cmp(a, b, op, line, col):
    x, y = a(), b()
    try:
        return _FLY_OPS[op](x, y)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 比较失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_iter(x, line, col):
    try:
        return iter(x)
    except TypeError as e:
        raise FlyRuntimeError(
            "%s: 不可迭代: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_cast(fn, *args, line, col):
    try:
        return fn(*args)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 类型转换失败: %s" % (_fly_loc(line, col), e)
        ) from None

import builtins as _fly_sb_builtins
import sys as _fly_sb_sys
import types as _fly_sb_types

_fly_sb_module_globals = globals()


def _fly_sb_audit(msg, line=None, col=None):
    where = ""
    if line is not None and col is not None:
        where = " @ src:%d:%d" % (line, col)
    print("[fly-sandbox] audit: " + msg + where, file=_fly_sb_sys.stderr)


def _fly_sb_is_module(x):
    return type(x) is _fly_sb_types.ModuleType


def _fly_sb_check_modattr(x, name, line=None, col=None):
    if _fly_sb_is_module(x) and (
        name in _FLY_SB_BLOCKED_MODS or name in _FLY_SB_MOD_ATTRS
    ):
        _fly_sb_audit("禁止访问模块属性 " + name, line, col)
        raise FlyRuntimeError("沙箱: 禁止访问模块属性 " + name)

_FLY_SB_DANGEROUS = frozenset((
    "eval", "exec", "compile", "open", "__import__", "getattr", "globals",
    "locals", "vars", "input", "breakpoint", "help", "dir", "memoryview",
    "__loader__",
))

_FLY_SB_REFLECT = frozenset((
    "__class__", "__bases__", "__base__", "__mro__", "__subclasses__",
    "__globals__", "__code__", "__closure__", "__dict__", "__reduce__",
    "__reduce_ex__", "__getattribute__", "__setattr__", "__delattr__",
    "__init_subclass__", "__prepare__", "__builtins__", "__traceback__",
    "gi_frame", "ag_frame", "cr_frame", "f_globals", "f_locals", "__loader__",
))

_FLY_SB_MOD_ATTRS = frozenset(("attrgetter", "itemgetter"))

_FLY_SB_BLOCKED_MODS = frozenset((
    "os", "sys", "subprocess", "socket", "ctypes", "shutil", "tempfile",
    "pty", "importlib", "imp", "marshal", "copyreg", "shelve",
    "multiprocessing", "pickle", "pickletools", "pathlib", "glob", "io",
    "codecs", "builtins", "csv", "sqlite3", "urllib", "ftplib", "telnetlib",
    "smtplib", "poplib", "imaplib", "http", "ssl", "zipfile", "tarfile",
    "gzip", "bz2", "lzma", "readline", "site", "pydoc", "gc", "dis",
    "inspect", "platform", "sysconfig", "pwd", "grp", "spwd", "getpass",
    "mmap", "fcntl", "select", "termios", "tty", "types", "trace",
    "tracemalloc", "faulthandler", "codeop", "code", "pkgutil", "py_compile",
    "compileall", "dbm", "email", "webbrowser", "cgi", "cgitb", "configparser",
    # 公开名的二进制/原生模块（CPython C 扩展，可与字节码互操作逃逸）：
    "pyexpat", "zlib",
))

_FLY_SB_ALLOWED_MODS = frozenset((
    "math", "cmath", "json", "time", "random", "secrets", "re", "string",
    "textwrap", "collections", "itertools", "functools", "operator",
    "logging", "signal", "resource", "statistics", "fractions", "decimal",
    "numbers", "datetime", "calendar", "zoneinfo", "uuid", "hashlib",
    "hmac", "base64", "binascii", "struct", "array", "bisect", "heapq",
    "copy", "pprint", "reprlib", "difflib", "unicodedata", "enum", "abc",
    "typing", "dataclasses", "contextlib", "threading", "queue", "warnings",
    "ast", "token", "keyword", "symtable", "this", "exceptions", "html",
    "xml", "unittest", "requests",
))


# 白名单模块的无害纯 Python 依赖（sys.modules 缓存读取的显式替代——
# 不再对"任意已加载模块"放行，枚举永远追不上，规则化显式清单才完备）。
_FLY_SB_ALLOWED_DEP_MODS = frozenset((
    "posixpath", "ntpath", "encodings",
))

# 私有模块规则：root 以下划线开头（CPython 内部实现/C 扩展，如 _json/_ssl/_ctypes）
# 一律禁止导入（与编译期 escape.go checkModule 同步）。
def _fly_sb_import(name, line=None, col=None, globals=None, locals=None, fromlist=(), level=0):
    # gen 注入调用：_fly_sb_import(name, 行, 列)
    # 但沙箱代理生效后由 CPython IMPORT_NAME 字节码发起的调用是 5 位置参数
    # __import__(name, globals, locals, fromlist, level) —— 签名错位，需重排。
    if isinstance(line, dict):
        globals, locals, fromlist, level = line, col, globals, locals
        line, col = None, None
    root = name.split(".")[0]
    if root in _FLY_SB_BLOCKED_MODS:
        _fly_sb_audit("禁止导入危险模块 " + root, line, col)
        raise FlyRuntimeError("沙箱: 禁止导入危险模块 " + root)
    if root.startswith("_"):
        _fly_sb_audit("禁止导入私有模块 " + root, line, col)
        raise FlyRuntimeError("沙箱: 禁止导入私有模块 " + root)
    if root in _FLY_SB_ALLOWED_MODS or root in _FLY_SB_ALLOWED_DEP_MODS:
        return _fly_sb_builtins.__import__(name, globals, locals, fromlist, level)
    _fly_sb_audit("模块 " + root + " 不在白名单", line, col)
    raise FlyRuntimeError("沙箱: 模块 " + root + " 不在白名单")


class _FlySandbox:
    def __getattr__(self, name):
        if name == "__import__":
            return _fly_sb_import
        if name in _FLY_SB_DANGEROUS:
            _fly_sb_audit("禁止访问内建 " + name)
            raise FlyRuntimeError("沙箱: 禁止访问内建 " + name)
        if name in ("FlyRuntimeError", "GuardError", "ResourceExhaustedError"):
            v = _fly_sb_module_globals[name]
        else:
            v = _fly_sb_builtins.getattr(_fly_sb_builtins, name)
        self.__dict__[name] = v
        return v

    def __getitem__(self, name):
        try:
            return self.__dict__[name]
        except KeyError:
            return self.__getattr__(name)


__builtins__ = _FlySandbox()

time = _fly_sb_import("time", 1, 1)
@_fly_cage(2, 52428800)
def heavy():
    data = _fly_binop([0], 100_000_000, "mul", 5, 20)
    return len(data)
@_fly_cage(2, 52428800)
def busy():
    while True:
        _fly_attr(time, "sleep", 10, 18)(0.01)
@_fly_cage(0.5)
def quick():
    return 1
print("cage-ready")
