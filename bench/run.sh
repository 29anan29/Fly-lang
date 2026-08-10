#!/usr/bin/env bash
# 性能基准：原生 CPython vs Fly-Lang 转译（同一源码，诚实对比护栏开销）
# 用法: bash bench/run.sh
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

echo "Fly: $FLY"
echo "Python: $(python3 -V 2>&1)"
echo ""

{
    echo "# Fly-Lang 转译性能基准"
    echo ""
    echo "> 同一份源码（bench/benchmarks/，纯 Python 语法），左列由 CPython 直接执行，右列经 \`fly build\` 转译后执行。"
    echo "> 每次取 3 轮最小耗时；转译产物含 \`_fly_*\` 运行时护栏（binop/下标/属性/比较兜底），开销即安全成本。"
    echo "> 环境：$(python3 -V 2>&1) / $(uname -sm) / $(date +%F)"
    echo ""
    echo "| 基准 | 原生 CPython (s) | Fly 转译 (s) | 开销倍数 |"
    echo "|------|-----------------:|-------------:|---------:|"
} > "$OUT"

for b in bench/benchmarks/*.fly; do
    name=$(basename "$b" .fly)

    native_out=$(python3 "$b")
    native_t=$(echo "$native_out" | awk '{print $1}')

    $FLY build "$b" -o /tmp/fly_bench_$name.py >/dev/null 2>&1
    if [ ! -f /tmp/fly_bench_$name.py ]; then
        echo "⚠️ $name: fly build 失败，跳过"
        continue
    fi
    trans_out=$(python3 /tmp/fly_bench_$name.py)
    trans_t=$(echo "$trans_out" | awk '{print $1}')

    ratio=$(python3 -c "print(f'{(float('$trans_t')/float('$native_t')):.2f}')")
    echo "| $name | $native_t | $trans_t | ${ratio}× |" >> "$OUT"
    echo "$name: native=$native_t trans=$trans_t ratio=${ratio}×"
done

echo ""
echo "报告已写入 $OUT"
