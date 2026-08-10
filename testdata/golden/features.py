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
from math import sqrt as s, pi
def process(items):
    total = 0
    for x in _fly_iter(items, 6, 5):
        total += _fly_binop(x, 2, "mul", 7, 20)
    while _fly_cmp(lambda: total, lambda: 100, "lt", 8, 11):
        total *= 2
        if _fly_cmp(lambda: total, lambda: 500, "gt", 10, 12):
            break
    else:
        print("never")
    try:
        data = _fly_attr(json, "loads", 15, 21)('{"a": [1, 2, 3]}')
        return _fly_get(_fly_get(data, "a", 16, 20), slice(1, 3), 16, 25), total
    except (ValueError, TypeError) as e:
        print(f"err: {e}")
    finally:
        print("done")
a, b = process([1, 2, 3])
print(a, b)
sq = [_fly_binop(i, i, "mul", 24, 9) for i in _fly_iter(range(10), 24, 6) if _fly_binop(i, 2, "mod", 24, 37) == 0]
d = {"x": 1, "y": s(9)}
print(sq, d, pi)
