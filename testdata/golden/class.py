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

class Animal:
    legs = 4
    def __init__(self, name):
        _fly_setattr(self, "name", name, 5, 14)
    def speak(self):
        return f"{self.name} makes a sound"
a = Animal("dog")
print(_fly_attr(a, "legs", 12, 9), _fly_attr(a, "speak", 12, 17)())
