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

import logging as _fly_log

import json
_fly_ob_b = globals().get("__builtins__", _fly_builtins)
__builtins__ = _FlyOnly(('json'))
def parse(raw):
    return json.loads(raw)
parse = _fly_patch_builtins(parse, ('json'))
__builtins__ = _fly_ob_b
class Admin:
    role = "admin"
    def __init__(self, name):
        object.__setattr__(self, '_fly_seal_initializing', True)
        self.name = name
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
print(parse('{"x": 9}'), admin.name)
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
    return uid * 2
print(twice(3))
