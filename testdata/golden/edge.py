"""P6 边界用例：嵌套作用域/闭包/递归/only 嵌套（全部应为正例）"""

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
            return getattr(_fly_builtins, name)
        if name.startswith("_fly_") or name in ("FlyRuntimeError", "GuardError"):
            return globals()[name]
        if name in self._mods:
            if name in _fly_sys.modules:
                return _fly_sys.modules[name]
            return _fly_builtins.__import__(name)
        if name in _FLY_SAFE_BUILTINS:
            return getattr(_fly_builtins, name)
        raise RuntimeError("only: 禁止访问未白名单名称 " + name)


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
    try:
        return getattr(_fly_op, op)(a, b)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 运算 %s 失败: %s" % (_fly_loc(line, col), op, e)
        ) from None


def _fly_unary(x, op, line, col):
    try:
        return getattr(_fly_op, op)(x)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 运算 %s 失败: %s" % (_fly_loc(line, col), op, e)
        ) from None


def _fly_get(x, k, line, col):
    try:
        return x[k]
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 下标访问失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_set(x, k, v, line, col):
    try:
        x[k] = v
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 下标赋值失败: %s" % (_fly_loc(line, col), e)
        ) from None


def _fly_attr(x, name, line, col):
    try:
        return getattr(x, name)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性访问 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_setattr(x, name, v, line, col):
    try:
        setattr(x, name, v)
    except _FLY_SAFE_ERRORS as e:
        raise FlyRuntimeError(
            "%s: 属性赋值 %s 失败: %s" % (_fly_loc(line, col), name, e)
        ) from None


def _fly_cmp(a, b, op, line, col):
    x, y = a(), b()
    try:
        return getattr(_fly_op, op)(x, y)
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

def outer(a):
    x = _fly_binop(a, 2, "mul", 4, 11)
    def inner(b):
        x = _fly_binop(b, 1, "add", 7, 15)
        return x
    return _fly_binop(inner(10), x, "add", 10, 22)
def counter():
    count = 0
    def inc():
        return _fly_binop(count, 1, "add", 17, 22)
    def setc(n):
        count = n
        return count
    return inc, setc
def fib(n):
    if _fly_cmp(lambda: n, lambda: 2, "lt", 27, 8):
        return n
    return _fly_binop(fib(_fly_binop(n, 1, "sub", 29, 18)), fib(_fly_binop(n, 2, "sub", 29, 31)), "add", 29, 23)
import json
_fly_ob_b = globals().get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('json'))
def parse(raw):
    return _fly_attr(json, "loads", 34, 21)(raw)
parse = _fly_patch_builtins(parse, ('json'))
import math
_fly_ob_c = globals().get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('math'))
def sq(x):
    return _fly_attr(math, "sqrt", 38, 25)(x)
sq = _fly_patch_builtins(sq, ('math'))
data = sq(4.0)
__builtins__ = _fly_ob_c
def double(raw):
    return _fly_attr(json, "dumps", 43, 21)(raw)
double = _fly_patch_builtins(double, ('json'))
__builtins__ = _fly_ob_b
uid = "42"
def use_taint():
    clean = _fly_cast(int, uid, line=51, col=16)
    return clean
def secret():
    password = "s3cr3t"
    hashed = hash(password)
    return hashed
print(outer(5))
inc, setc = counter()
print(inc(), setc(3))
print(fib(10))
print(parse('{"a": 1}'))
print(sq is None or sq(9.0))
print(use_taint())
print(secret())
