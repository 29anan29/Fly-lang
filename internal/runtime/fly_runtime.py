# Fly-Lang 运行时库（go:embed 注入生成代码）
# 节标记： # fly:section:<名称>   —— gen 按需提取注入

# fly:section:guard
class GuardError(Exception):
    """Fly guard 断言失败"""

    pass

# fly:section:only
import builtins as _fly_builtins
import sys as _fly_sys

_FLY_SAFE_BUILTINS = frozenset((
    "len", "int", "float", "str", "bool", "list", "dict", "set", "tuple",
    "range", "enumerate", "zip", "map", "filter", "sorted", "sum", "min",
    "max", "abs", "round", "repr", "isinstance", "issubclass", "type",
    "hasattr", "any", "all", "id", "chr", "ord", "iter", "next", "slice",
    "reversed", "format", "frozenset", "bytes", "bytearray", "hash",
    "divmod", "pow", "callable", "classmethod", "staticmethod", "property",
    "super", "object", "print", "Exception", "ValueError", "TypeError",
    "KeyError", "RuntimeError", "AttributeError", "IndexError",
    "StopIteration", "NotImplemented", "ZeroDivisionError",
    "AssertionError", "NameError", "ImportError", "KeyboardInterrupt",
    "BaseException",
))

class _FlyOnly:
    def __init__(self, mods):
        self._mods = frozenset(mods)

    def __getattr__(self, name):
        if name.startswith("__") and name.endswith("__"):
            return _fly_sb_builtins.getattr(_fly_builtins, name)
        if name.startswith("_fly_") or name in ("FlyRuntimeError", "GuardError"):
            return _fly_sb_module_globals[name]
        if name in self._mods:
            if name in _fly_sys.modules:
                return _fly_sys.modules[name]
            return _fly_sb_builtins.__import__(name)
        if name in _FLY_SAFE_BUILTINS:
            return _fly_sb_builtins.getattr(_fly_builtins, name)
        raise RuntimeError("only: 禁止访问未白名单名称 " + name)

    def __getitem__(self, name):
        return self.__getattr__(name)


def _fly_patch_builtins(fn, mods):
    proxy = _FlyOnly(mods)
    g = fn.__globals__
    def wrapped(*args, **kwargs):
        old = g.get("__builtins__")
        g["__builtins__"] = proxy
        try:
            return fn(*args, **kwargs)
        finally:
            if old is None:
                del g["__builtins__"]
            else:
                g["__builtins__"] = old
    wrapped.__name__ = fn.__name__
    return wrapped

# fly:section:trace
import logging as _fly_log

# fly:section:cage
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

# fly:section:runtime
import operator as _fly_op


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
        return _fly_sb_builtins.getattr(_fly_op, op)(a, b)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 运算 %s 失败: %s" % (_fly_loc(line, col), op, e)
        ) from None


def _fly_unary(x, op, line, col):
    try:
        return _fly_sb_builtins.getattr(_fly_op, op)(x)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 运算 %s 失败: %s" % (_fly_loc(line, col), op, e)
        ) from None


def _fly_get(x, k, line, col):
    if isinstance(k, str) and k in _FLY_SB_REFLECT:
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
        raise FlyRuntimeError(
            "%s: 沙箱: 禁止反射访问 %s" % (_fly_loc(line, col), name)
        )
    try:
        return _fly_sb_builtins.getattr(x, name)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性访问 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_setattr(x, name, v, line, col):
    if name in _FLY_SB_REFLECT:
        raise FlyRuntimeError(
            "%s: 沙箱: 禁止反射赋值 %s" % (_fly_loc(line, col), name)
        )
    try:
        _fly_sb_builtins.setattr(x, name, v)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性赋值 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_cmp(a, b, op, line, col):
    x, y = a(), b()
    try:
        return _fly_sb_builtins.getattr(_fly_op, op)(x, y)
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


# fly:section:sandbox
import builtins as _fly_sb_builtins
import sys as _fly_sb_sys

_fly_sb_module_globals = globals()

_FLY_SB_DANGEROUS = frozenset((
    "eval", "exec", "compile", "open", "__import__", "getattr", "globals",
    "locals", "vars", "input", "breakpoint", "help", "dir", "memoryview",
    "__loader__",
))

_FLY_SB_REFLECT = frozenset((
    "__class__", "__bases__", "__base__", "__mro__", "__subclasses__",
    "__globals__", "__code__", "__closure__", "__dict__", "__reduce__",
    "__reduce_ex__", "__getattribute__", "__setattr__", "__delattr__",
    "__init_subclass__", "__prepare__",
))

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
    "xml", "unittest",
))


def _fly_sb_import(name, globals=None, locals=None, fromlist=(), level=0):
    root = name.split(".")[0]
    if root in _FLY_SB_BLOCKED_MODS:
        raise FlyRuntimeError("沙箱: 禁止导入危险模块 " + root)
    if root in _FLY_SB_ALLOWED_MODS or root in _fly_sb_sys.modules:
        return _fly_sb_builtins.__import__(name, globals, locals, fromlist, level)
    raise FlyRuntimeError("沙箱: 模块 " + root + " 不在白名单")


class _FlySandbox:
    def __getattr__(self, name):
        if name == "__import__":
            return _fly_sb_import
        if name in _FLY_SB_DANGEROUS:
            raise FlyRuntimeError("沙箱: 禁止访问内建 " + name)
        if name in ("FlyRuntimeError", "GuardError", "ResourceExhaustedError"):
            return _fly_sb_module_globals[name]
        return _fly_sb_builtins.getattr(_fly_sb_builtins, name)

    def __getitem__(self, name):
        return self.__getattr__(name)


__builtins__ = _FlySandbox()
