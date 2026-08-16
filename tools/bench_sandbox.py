#!/usr/bin/env python3
"""沙箱注入开销基准：Fly 产物（注入 runtime+sandbox）vs 等价纯 Python。

测量对象：__builtins__ 被 _FlySandbox 代理后每次内建名访问的开销。
用法：python3 tools/bench_sandbox.py   （产物由脚本自动 build）
输出：docs/性能.md 数据来源（表格格式）。
"""
import subprocess
import sys
import timeit
import pathlib
import tempfile

REPO = pathlib.Path(__file__).resolve().parent.parent
FLY = REPO / "fly"
CASES = [
    # 全部包进函数体内——顶层 f_builtins 缓存真 builtins（无代理税），
    # 函数/类体内的内建名访问才走 _FlySandbox.__getattr__（真实税路径）。
    ("fib_recursive", """
def fib(n):
    return n if n < 2 else fib(n - 1) + fib(n - 2)
def run():
    print(fib(24))
run()
"""),
    ("arith_loop", """
def run():
    total = 0
    for i in range(1000000):
        total = total + i * 3 - (i % 7)
    print(total)
run()
"""),
    ("str_concat", """
def run():
    s = ""
    for i in range(200000):
        s = s + str(i % 100)
    print(len(s))
run()
"""),
    ("list_comp", """
def run():
    data = [i * 2 for i in range(200000)]
    print(sum(data))
run()
"""),
]


def build_fly(src: str) -> str:
    with tempfile.NamedTemporaryFile("w", suffix=".fly", delete=False) as f:
        f.write(src)
        path = f.name
    out = path + ".py"
    r = subprocess.run([str(FLY), "build", "-o", out, path],
                       capture_output=True, text=True)
    if r.returncode != 0:
        print("build 失败:", r.stderr)
        sys.exit(1)
    return out


def bench(code: str) -> float:
    devnull = open("/dev/null", "w")

    def run() -> None:
        g: dict = {}
        old = sys.stdout
        sys.stdout = devnull
        try:
            exec(compile(open(code).read(), code, "exec"), g)
        finally:
            sys.stdout = old

    t = timeit.Timer(run)
    return min(t.repeat(repeat=7, number=1)) * 1000


def main() -> None:
    rows = []
    for name, src in CASES:
        with tempfile.NamedTemporaryFile("w", suffix=".py", delete=False) as f:
            f.write(src)
            pure = f.name
        injected = build_fly(src)
        t_pure = bench(pure)
        t_fly = bench(injected)
        t_fly_top = bench(injected)
        rows.append((name, t_pure, t_fly, t_fly_top,
                     t_fly / t_pure, t_fly_top / t_pure))
        print(f"{name:14s} pure={t_pure:8.2f}ms  fly={t_fly:8.2f}ms  "
              f"fly(顶层)={t_fly_top:8.2f}ms  ratio={t_fly/t_pure:.2f}x")
    print()
    print("| 场景 | 纯 Python (ms) | Fly 注入 (ms) | 开销比 | 顶层 (ms) | 顶层比 |")
    print("|------|---------------:|--------------:|-------:|----------:|-------:|")
    for n, p, f, ft, r, rt in rows:
        print(f"| {n} | {p:.2f} | {f:.2f} | {r:.2f}x | {ft:.2f} | {rt:.2f}x |")


if __name__ == "__main__":
    main()
