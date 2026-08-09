#!/usr/bin/env bash
# Fly-Lang 一键发布：打 tag → 推远程 → 触发 CI → 可选轮询等待 Release 发布
# 用法:
#   tools/release.sh 0.3.0          # 正式版 → v0.3.0，发 release
#   tools/release.sh 0.3.0-dev      # dev 版 → v0.3.0-dev，发 prerelease
#   tools/release.sh 0.3.0 --wait   # 推送后轮询 Actions 结果（最多 10 分钟）
set -euo pipefail

VER="${1:?用法: release.sh <版本> [--wait]，如 release.sh 0.3.0 或 0.3.0-dev}"
WAIT=0
if [ "${2:-}" = "--wait" ]; then
	WAIT=1
fi
TAG="v${VER#v}"
REPO="29anan29/Fly-lang"
case "$TAG" in
	v[0-9]*.[0-9]*.[0-9]*-dev*) ;;
	v[0-9]*.[0-9]*.[0-9]*) ;;
	*) echo "版本号格式不正确: $TAG（应为 X.Y.Z 或 X.Y.Z-dev）"; exit 1 ;;
esac

if [ -n "$(git status --porcelain)" ]; then
	echo "工作区不干净，请先提交再发布"; exit 1
fi
git fetch origin --tags

if git ls-remote --tags origin "refs/tags/$TAG" | grep -q "refs/tags/$TAG"; then
	echo "tag $TAG 已在远程，复用"
else
	if git rev-parse -q --verify "refs/tags/$TAG" >/dev/null; then
		git push origin "$TAG"
	else
		git tag "$TAG"
		git push origin "$TAG"
	fi
	echo "tag $TAG 已推送，CI 已触发"
fi
if [[ "$TAG" == *-dev* ]]; then
	echo "渠道: dev（prerelease）— 发布后可用 \`fly update --channel dev\` 获取"
else
	echo "渠道: release — 发布后可用 \`fly update\` 获取"
fi
echo "Actions 进度: https://github.com/$REPO/actions"
echo "Release 页面: https://github.com/$REPO/releases"

if [ "$WAIT" = 1 ]; then
	for i in $(seq 1 60); do
		REL=$(curl -s "https://api.github.com/repos/$REPO/releases/tags/$TAG")
		if echo "$REL" | grep -q '"id"'; then
			URL=$(echo "$REL" | grep -o '"html_url": *"[^"]*"' | head -1 | cut -d'"' -f4)
			echo "已发布: $URL"
			exit 0
		fi
		sleep 10
	done
	echo "等待超时（10 分钟），请到 Actions 查看结果"; exit 1
fi
