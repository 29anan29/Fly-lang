"""P6 最终回归：方案.md 全部 8 个示例（正例行，全部应通过）"""

class GuardError(Exception):
    """Fly guard 断言失败"""

    pass

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

import logging as _fly_log

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
    _fly_sb_check_modattr(x, name, line, col)
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
    _fly_sb_check_modattr(x, name, line, col)
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
    return isinstance(x, _fly_sb_types.ModuleType)


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
            return _fly_sb_module_globals[name]
        return _fly_sb_builtins.getattr(_fly_sb_builtins, name)

    def __getitem__(self, name):
        return self.__getattr__(name)


__builtins__ = _FlySandbox()

class _Request:
    def get(self, key, default=None):
        return "42"
request = _Request()
def handle():
    uid = _fly_attr(request, "get", 11, 19)('id')
    clean = _fly_cast(int, uid, line=13, col=16)
    return clean
import json
import math
_fly_ob_b = _fly_sb_module_globals.get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('json', 'math'))
def parse(raw):
    data = _fly_attr(json, "loads", 19, 21)(raw)
    return _fly_attr(math, "sqrt", 20, 21)(_fly_get(data, "x", 20, 30))
parse = _fly_patch_builtins(parse, ('json', 'math'))
__builtins__ = _fly_ob_b
SECRET_KEY = "abc-123"
hashed = hash(SECRET_KEY)
def login(password):
    hashed = hash(password)
    return hashed
@_fly_cage(2, 52428800)
def heavy():
    data = _fly_binop([0], 1000, "mul", 36, 20)
    return len(data)
def create_user(username, age):
    if not (isinstance(username, str) and (len(username) > 0) and (len(username) <= 20)):
        raise GuardError("guard username: str, len(username) > 0, len(username) <= 20")
    if not (isinstance(age, int) and (_fly_cmp(lambda: 0, lambda: age, "lt", 42, 21) and _fly_cmp(lambda: age, lambda: 150, "lt", 42, 21))):
        raise GuardError("guard age: int, 0 < age < 150")
    return (username, age)
class Admin:
    role = "admin"
    def __init__(self, name):
        object.__setattr__(self, '_fly_seal_initializing', True)
        _fly_setattr(self, "name", name, 50, 14)
        object.__setattr__(self, '_fly_seal_initializing', False)
    def __setattr__(self, name, value):
        if _fly_sb_builtins.getattr(self, "_fly_seal_initializing", False):
            object.__setattr__(self, name, value)
        else:
            raise AttributeError("seal 类 %s 的属性 %s 不可修改" % (type(self).__name__, name))
    def __delattr__(self, name):
        if _fly_sb_builtins.getattr(self, "_fly_seal_initializing", False):
            object.__delattr__(self, name)
        else:
            raise AttributeError("seal 类 %s 的属性 %s 不可删除" % (type(self).__name__, name))
admin = Admin("Alice")
def delete_user(uid):
    _fly_log.log(_fly_log.WARNING, "enter delete_user, uid=%r", uid)
    try:
        _fly_ret_c = _fly_trace_impl_delete_user(uid)
    except BaseException as _fly_err_c:
        _fly_log.log(_fly_log.WARNING, "exit delete_user: raise %r", _fly_err_c)
        raise
    _fly_log.log(_fly_log.WARNING, "exit delete_user: ret=%r", _fly_ret_c)
    return _fly_ret_c
def _fly_trace_impl_delete_user(uid):
    _fly_attr(db, "append", 57, 12)(uid)
    return uid
db = []
print(parse('{"x": 9}'))
print(hashed is not None)
print(login("pw"))
print(heavy())
print(create_user("alice", 30))
print(_fly_attr(admin, "name", 66, 13), _fly_attr(admin, "role", 66, 25))
print(delete_user(7))
