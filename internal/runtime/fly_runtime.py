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
