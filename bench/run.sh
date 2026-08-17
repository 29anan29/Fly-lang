#!/usr/bin/env bash
# 性能基准：CPython / PyPy / PyFly 转译三列对比（同一源码，诚实对比护栏开销）
# 用法: bash bench/run.sh
#   PYPY 环境变量可指定 pypy3 路径（默认自动探测：pypy3 / uv python find pypy）
set -euo pipefail
cd "$(dirname "$0")/.."

FLY=./fly
OUT=docs/bench/bench.md
mkdir -p docs/bench

if [ ! -x "$FLY" ]; then
    FLY=$(command -v fly || echo "")
fi
if [ -z "$FLY" ]; then
    echo "未找到 fly 编译器"; exit 1
fi

PYPY="${PYPY:-}"
if [ -z "$PYPY" ]; then
    for c in "$(command -v pypy3 || true)" "$(command -v pypy || true)"; do
        if [ -n "$c" ]; then PYPY="$c"; break; fi
    done
fi
if [ -z "$PYPY" ] && command -v uv >/dev/null 2>&1; then
    PYPY=$(uv python find pypy 2>/dev/null || echo "")
fi

echo "Fly: $FLY"
echo "CPython: $(python3 -V 2>&1)"
if [ -n "$PYPY" ]; then
    echo "PyPy: $($PYPY -V 2>&1)"
else
    echo "PyPy: 未找到（跳过）"
fi
echo ""

{
    echo "# PyFly 转译性能基准"
    echo ""
    echo "> 同一份源码（bench/benchmarks/，纯 Python 语法）：左列为 CPython 直接执行，"
    if [ -n "$PYPY" ]; then
        echo "> 中列为 PyPy JIT 直接执行，右列为经 \`fly build\` 转译后由 CPython 执行。"
    else
        echo "> 右列为经 \`fly build\` 转译后由 CPython 执行（未检测到 PyPy）。"
    fi
    echo "> 每次取 3 轮最小耗时；转译产物含 \`_fly_*\` 运行时护栏（binop/下标/属性/比较兜底），开销即安全成本。"
    echo "> 环境：$(python3 -V 2>&1) / $(uname -sm) / $(date +%F)"
    if [ -n "$PYPY" ]; then
        echo "> $($PYPY -V 2>&1)"
    fi
    echo ""
    if [ -n "$PYPY" ]; then
        echo "| 基准 | CPython (s) | PyPy (s) | Fly 转译 (s) | vs CPython | vs PyPy |"
        echo "|------|------------:|---------:|-------------:|-----------:|--------:|"
    else
        echo "| 基准 | CPython (s) | Fly 转译 (s) | 开销倍数 |"
        echo "|------|------------:|-------------:|---------:|"
    fi
} > "$OUT"

for b in bench/benchmarks/*.fly; do
    name=$(basename "$b" .fly)

    native_t=$(python3 "$b" | awk '{print $1}')
    pypy_t=""
    if [ -n "$PYPY" ]; then
        pypy_t=$("$PYPY" "$b" | awk '{print $1}')
    fi

    $FLY build "$b" -o /tmp/fly_bench_$name.py >/dev/null 2>&1
    if [ ! -f /tmp/fly_bench_$name.py ]; then
        echo "⚠️ $name: fly build 失败，跳过"
        continue
    fi
    trans_t=$(python3 /tmp/fly_bench_$name.py | awk '{print $1}')

    if [ -n "$PYPY" ]; then
        r_nat=$(python3 -c "print(f'{(float('$trans_t')/float('$native_t')):.2f}')")
        r_pypy=$(python3 -c "print(f'{(float('$trans_t')/float('$pypy_t')):.2f}')")
        echo "| $name | $native_t | $pypy_t | $trans_t | ${r_nat}× | ${r_pypy}× |" >> "$OUT"
        echo "$name: cp=$native_t pypy=$pypy_t fly=$trans_t vs-cp=${r_nat}× vs-pypy=${r_pypy}×"
    else
        ratio=$(python3 -c "print(f'{(float('$trans_t')/float('$native_t')):.2f}')")
        echo "| $name | $native_t | $trans_t | ${ratio}× |" >> "$OUT"
        echo "$name: native=$native_t trans=$trans_t ratio=${ratio}×"
    fi
done

echo ""
echo "报告已写入 $OUT"
