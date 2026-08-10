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

a, b, c = 3, 2, 1
x = _fly_binop(_fly_binop(a, b, "add", 2, 8), c, "mul", 2, 13)
y = _fly_unary(_fly_binop(a, 2, "pow", 3, 8), "neg", 3, 5)
z = not a == b
w = _fly_cmp(lambda: a, lambda: b, "lt", 5, 5) and _fly_cmp(lambda: b, lambda: c, "lt", 5, 5)
p = a if _fly_cmp(lambda: b, lambda: c, "gt", 6, 10) else b
q = _fly_binop(a, b, "add", 7, 8) if _fly_cmp(lambda: a, lambda: c, "gt", 7, 17) and _fly_cmp(lambda: b, lambda: a, "lt", 7, 29) else _fly_binop(a, b, "sub", 7, 44)
s = _fly_binop(_fly_binop(_fly_binop(a, b, "mul", 8, 7), _fly_binop(c, a, "truediv", 8, 15), "add", 8, 11), _fly_binop(_fly_binop(a, b, "sub", 8, 24), 2, "pow", 8, 29), "sub", 8, 19)
t = _fly_binop(_fly_binop(_fly_unary(a, "invert", 9, 5), b, "and_", 9, 8), _fly_binop(c, a, "xor", 9, 16), "or_", 9, 12)
u = _fly_binop(_fly_binop(a, 2, "lshift", 10, 7), 1, "rshift", 10, 12)
v = _fly_binop(_fly_binop(a, b, "floordiv", 11, 7), c, "mod", 11, 12)
lst = [x, y, z, w, p, q, s, t, u, v]
print(lst)
print([(i, j) for i in _fly_iter(range(3), 14, 7) for j in _fly_iter(range(3), 14, 7) if _fly_binop(i, j, "add", 14, 56) == 2])
print({1, 2, 2, 3}, {})
print((a,), a, a)
print(1_000_000, 0xFF, 0b1010, 0o17, 1.5e3, 1.5, .5)
print('单引号', "双引号", '转义\'引号', "a\tb", "多行\
拼接")
print("""三引号
跨行""")
