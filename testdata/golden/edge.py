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

def outer(a):
    x = a * 2
    def inner(b):
        x = b + 1
        return x
    return inner(10) + x
def counter():
    count = 0
    def inc():
        return count + 1
    def setc(n):
        count = n
        return count
    return inc, setc
def fib(n):
    if n < 2:
        return n
    return fib(n - 1) + fib(n - 2)
import json
_fly_ob_b = globals().get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('json'))
def parse(raw):
    return json.loads(raw)
parse = _fly_patch_builtins(parse, ('json'))
import math
_fly_ob_c = globals().get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('math'))
def sq(x):
    return math.sqrt(x)
sq = _fly_patch_builtins(sq, ('math'))
data = sq(4.0)
__builtins__ = _fly_ob_c
def double(raw):
    return json.dumps(raw)
double = _fly_patch_builtins(double, ('json'))
__builtins__ = _fly_ob_b
uid = "42"
def use_taint():
    clean = int(uid)
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
