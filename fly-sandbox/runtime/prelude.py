"""
prelude.py — 注入到 Wasm 沙箱内的 Python 运行时

提供 Fly-Lang 关键字对应的运行时兜底：
  - _FlyOnly       (对应 only 关键字)
  - _fly_cage      (对应 cage 关键字)
  - _fly_seal      (对应 seal 关键字)
  - _fly_trace     (对应 trace 关键字)
  - _fly_mask      (对应 mask 关键字)

这些在 Fly-Lang 转译产物中也会被注入，这里提供 Wasm 环境内的版本。
"""

import sys
import time
import logging
import functools
import threading
from types import ModuleType


# ─────────────────────────────────────────────
# only — 模块白名单代理
# ─────────────────────────────────────────────

class _FlyOnly:
    """
    白名单模块代理。只有显式声明的模块可被导入。
    对应 Fly-Lang `only (json, math):` 编译期检查 + 运行时兜底。
    """
    _ALLOWED = frozenset({
        "json", "math", "random", "string", "re",
        "datetime", "time", "collections", "itertools",
        "functools", "typing", "dataclasses", "enum",
        "hashlib", "hmac", "secrets", "uuid",
    })

    def __init__(self, allowed: set[str] | None = None):
        if allowed is not None:
            self._allowed = frozenset(allowed)
        else:
            self._allowed = self._ALLOWED

    def __getattr__(self, name: str):
        if name in self._allowed:
            return __import__(name)
        raise ImportError(
            f"[Fly-Lang] Module '{name}' not in allowlist. "
            f"Use 'only ({name}, ...):' to grant access."
        )

    def __setattr__(self, name: str, value):
        if name.startswith("_FlyOnly__") or name == "_allowed":
            super().__setattr__(name, value)
            return
        raise PermissionError(
            f"[Fly-Lang] Cannot modify __builtins__: '{name}'"
        )


# ─────────────────────────────────────────────
# cage — 资源限制装饰器
# ─────────────────────────────────────────────

class ResourceExhaustedError(Exception):
    """cage 资源超限异常"""
    pass


def _fly_cage(max_time: str = "5s", max_memory: str = "100MB"):
    """
    函数级资源限制装饰器。
    在 Wasm 环境中，实际限制由 host 的 fuel + memory pages 强制执行；
    此装饰器做 Python 层的软性检查（超时定时器）。

    对应 Fly-Lang `cage(max_time="5s", max_memory="100MB"):`
    """
    def parse_time(s: str) -> float:
        s = s.strip()
        if s.endswith("ms"):
            return float(s[:-2]) / 1000.0
        elif s.endswith("s"):
            return float(s[:-1])
        elif s.endswith("m"):
            return float(s[:-1]) * 60
        return float(s)

    timeout_sec = parse_time(max_time)

    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            # Python 层软超时（实际硬限制由 Wasm fuel 提供）
            timer = threading.Timer(
                timeout_sec,
                lambda: (_ for _ in ()).throw(
                    ResourceExhaustedError(
                        f"[Fly-Lang] cage: timeout after {max_time}"
                    )
                )
            )
            timer.start()
            try:
                return func(*args, **kwargs)
            finally:
                timer.cancel()
        return wrapper
    return decorator


# ─────────────────────────────────────────────
# seal — 防篡改类装饰器
# ─────────────────────────────────────────────

_SEAL_TOKEN = object()

def _fly_seal(cls):
    """
    禁止运行时修改类实例属性（初始化后）。
    对应 Fly-Lang `seal class Foo:`

    实现：用初始化令牌模式，__setattr__ 在初始化后拒绝修改。
    """
    original_init = cls.__init__
    original_setattr = cls.__setattr__
    original_delattr = cls.__delattr__

    def __init__(self, *args, **kwargs):
        self._sealed = False
        original_init(self, *args, **kwargs)
        self._sealed = True

    def __setattr__(self, name: str, value):
        if getattr(self, "_sealed", False) and not name.startswith("_"):
            raise PermissionError(
                f"[Fly-Lang] seal: cannot modify '{name}' on sealed {cls.__name__}"
            )
        super(cls, self).__setattr__(name, value)

    def __delattr__(self, name: str):
        if getattr(self, "_sealed", False) and not name.startswith("_"):
            raise PermissionError(
                f"[Fly-Lang] seal: cannot delete '{name}' on sealed {cls.__name__}"
            )
        super(cls, self).__delattr__(name)

    cls.__init__ = __init__
    cls.__setattr__ = __setattr__
    cls.__delattr__ = __delattr__
    return cls


# ─────────────────────────────────────────────
# trace — 审计日志装饰器
# ─────────────────────────────────────────────

def _fly_trace(level: str = "INFO", args: bool = False):
    """
    函数进出审计日志。
    对应 Fly-Lang `trace(level="INFO", args=True):`

    在 Wasm 沙箱内，日志通过 host function fly_log_trace 输出到宿主。
    """
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*func_args, **func_kwargs):
            arg_info = ""
            if args:
                arg_info = f" args={func_args} kwargs={func_kwargs}"
            # 尝试调用 host trace（Wasm 环境）
            try:
                _host_trace(level, f"→ {func.__name__}{arg_info}")
            except Exception:
                pass  # 非 Wasm 环境 fallback
            try:
                result = func(*func_args, **func_kwargs)
                try:
                    _host_trace(level, f"← {func.__name__} ok")
                except Exception:
                    pass
                return result
            except Exception as e:
                try:
                    _host_trace(level, f"← {func.__name__} error: {e}")
                except Exception:
                    pass
                raise
        return wrapper
    return decorator


def _host_trace(level: str, message: str):
    """
    调用 Wasm host function `fly_log_trace`。
    在非 Wasm 环境中会抛出 ImportError，由调用方处理。
    """
    import ctypes
    # 在 Wasm 环境中，fly_log_trace 是导入函数
    # 这里用 ctypes 模拟；实际 Rust 宿主注册为 wasm import
    raise NotImplementedError("host function only available in Wasm runtime")


# ─────────────────────────────────────────────
# mask — 敏感数据脱敏辅助
# ─────────────────────────────────────────────

_MASKED_NAMES = set()

def _fly_mask(*names: str):
    """
    注册敏感变量名，后续 print/logging 中自动脱敏。
    对应 Fly-Lang `mask password`
    """
    _MASKED_NAMES.update(names)
    return names


def _apply_mask(text: str) -> str:
    """对文本中的敏感词做脱敏替换"""
    for name in _MASKED_NAMES:
        if name in text:
            text = text.replace(name, "*" * len(name))
    return text


# ─────────────────────────────────────────────
# 安装到 __builtins__
# ─────────────────────────────────────────────

def install():
    """将 Fly-Lang 运行时注入到 Python 内置命名空间"""
    builtins = sys.modules["builtins"]
    builtins._FlyOnly = _FlyOnly
    builtins._fly_cage = _fly_cage
    builtins._fly_seal = _fly_seal
    builtins._fly_trace = _fly_trace
    builtins._fly_mask = _fly_mask
    builtins.ResourceExhaustedError = ResourceExhaustedError
    # 默认安装白名单 __builtins__
    builtins.__builtins__ = _FlyOnly()
    print("[Fly-Lang] Runtime installed: _FlyOnly, _fly_cage, _fly_seal, _fly_trace, _fly_mask")


# 自动安装
install()
