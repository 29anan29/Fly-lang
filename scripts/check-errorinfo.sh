#!/usr/bin/env bash
# 校验错误码双真源一致：internal/ast/errors.go（Go 源）→ src/errorinfo.rs（Rust 产物）。
# 改 errors.go 后运行 `go run ./tools/gen_errorinfo` 重新生成并提交两者。
# CI 中运行本脚本：生成后如有 diff 即失败（防止双源漂移）。
set -euo pipefail
cd "$(dirname "$0")/.."

cp src/errorinfo.rs /tmp/errorinfo.rs.bak
go run ./tools/gen_errorinfo
if ! diff -q /tmp/errorinfo.rs.bak src/errorinfo.rs >/dev/null; then
  git checkout -- src/errorinfo.rs 2>/dev/null || true
  echo "FAIL: src/errorinfo.rs 与 internal/ast/errors.go 不一致，请运行 'go run ./tools/gen_errorinfo' 并提交产物" >&2
  exit 1
fi
echo "OK: 错误码双真源一致"
