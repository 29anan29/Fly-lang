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

import logging as _fly_log

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

import json
_fly_ob_b = globals().get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('json'))
def parse(raw):
    return _fly_attr(json, "loads", 3, 21)(raw)
parse = _fly_patch_builtins(parse, ('json'))
__builtins__ = _fly_ob_b
class Admin:
    role = "admin"
    def __init__(self, name):
        object.__setattr__(self, '_fly_seal_initializing', True)
        _fly_setattr(self, "name", name, 9, 14)
        object.__setattr__(self, '_fly_seal_initializing', False)
    def __setattr__(self, name, value):
        if getattr(self, "_fly_seal_initializing", False):
            object.__setattr__(self, name, value)
        else:
            raise AttributeError("seal 类 %s 的属性 %s 不可修改" % (type(self).__name__, name))
    def __delattr__(self, name):
        if getattr(self, "_fly_seal_initializing", False):
            object.__delattr__(self, name)
        else:
            raise AttributeError("seal 类 %s 的属性 %s 不可删除" % (type(self).__name__, name))
admin = Admin("Alice")
print(parse('{"x": 9}'), _fly_attr(admin, "name", 12, 32))
def twice(uid):
    _fly_log.log(_fly_log.INFO, "enter twice, uid=%r", uid)
    try:
        _fly_ret_c = _fly_trace_impl_twice(uid)
    except BaseException as _fly_err_c:
        _fly_log.log(_fly_log.INFO, "exit twice: raise %r", _fly_err_c)
        raise
    _fly_log.log(_fly_log.INFO, "exit twice: ret=%r", _fly_ret_c)
    return _fly_ret_c
def _fly_trace_impl_twice(uid):
    return _fly_binop(uid, 2, "mul", 16, 20)
print(twice(3))
