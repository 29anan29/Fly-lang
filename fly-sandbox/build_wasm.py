#!/usr/bin/env python3
"""
build_wasm.py — 编译 RustPython 为 wasm32-wasip1 目标

前置条件:
    rustup target add wasm32-wasip1
    cargo install wasm-opt  (可选，用于优化体积)

输出: runtime/rustpython.wasm
"""
import subprocess
import sys
from pathlib import Path

RUSTPYTHON_REPO = "https://github.com/RustPython/RustPython.git"
BUILD_DIR = Path("build/rustpython")
OUTPUT = Path("runtime/rustpython.wasm")


def run(cmd: list[str], cwd: Path | None = None):
    print(f"[run] {' '.join(cmd)}")
    result = subprocess.run(cmd, cwd=cwd, check=False)
    if result.returncode != 0:
        sys.exit(f"命令失败 (exit={result.returncode}): {' '.join(cmd)}")


def main():
    # 1. 克隆 RustPython（如果尚未克隆）
    if not BUILD_DIR.exists():
        run(["git", "clone", "--depth", "1", RUSTPYTHON_REPO, str(BUILD_DIR)])

    # 2. 编译 wasm 目标
    #    RustPython 的 wasm 入口在 wasm_lib crate
    wasm_crate = BUILD_DIR / "wasm_lib"
    run(
        [
            "cargo", "build",
            "--release",
            "--target", "wasm32-wasip1",
            "--features", "freeze-stdlib",
        ],
        cwd=wasm_crate,
    )

    # 3. 复制产物
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    wasm_path = wasm_crate / "target/wasm32-wasip1/release/wasm_lib.wasm"
    OUTPUT.write_bytes(wasm_path.read_bytes())
    print(f"[ok] 输出: {OUTPUT} ({OUTPUT.stat().st_size / 1024:.1f} KB)")

    # 4. 可选：wasm-opt 优化
    try:
        run(["wasm-opt", "-Oz", "-o", str(OUTPUT), str(OUTPUT)])
        print(f"[ok] wasm-opt 优化完成: {OUTPUT.stat().st_size / 1024:.1f} KB")
    except FileNotFoundError:
        print("[skip] wasm-opt 未安装，跳过优化")


if __name__ == "__main__":
    main()
