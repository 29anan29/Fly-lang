#!/usr/bin/env bash
# PyFly 一键发布：打 tag → 推远程 → 触发 CI → 可选轮询等待 Release 发布
# 用法（版本号自动按最新正式版 patch+1，如 v0.2.0 → v0.2.1）:
#   tools/release.sh            # 自动发正式版（0.2.0 → v0.2.1，release）
#   tools/release.sh dev        # 自动发 dev 版（0.2.0 → v0.2.1-dev，prerelease）
#   tools/release.sh 0.2.1      # 显式版本发正式版 → v0.2.1
#   tools/release.sh 0.2.1-dev  # 显式版本发 dev 版 → v0.2.1-dev
# 任意模式加 --wait 轮询等待发布完成（最多 10 分钟）
set -euo pipefail

ARG1="${1:-}"
WAIT=0
if [ "$ARG1" = "--wait" ] || [ "${2:-}" = "--wait" ]; then
	WAIT=1
fi
[ "$ARG1" = "--wait" ] && ARG1=""
REPO="29anan29/Fly-lang"
PUSH_URL="origin"
if [ -n "${GH_TOKEN:-}" ]; then
	PUSH_URL="https://x-access-token:${GH_TOKEN}@github.com/$REPO.git"
fi

push_tag() {
	git push "$PUSH_URL" "$TAG"
}

case "$ARG1" in
	"" | dev | release)
		if [ "$ARG1" = "dev" ]; then
			LATEST=$(curl -s "https://api.github.com/repos/$REPO/releases?per_page=100" | tr -d ' \n' | grep -o '"tag_name":"v[0-9]*\.[0-9]*\.[0-9]*-dev"[^}]*"prerelease":true' | grep -o 'v[0-9]*\.[0-9]*\.[0-9]*-dev' | head -1)
			[ -z "$LATEST" ] && LATEST=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)
			KIND="dev"
		else
			LATEST=$(curl -s "https://api.github.com/repos/$REPO/releases/latest" | grep -o '"tag_name": *"[^"]*"' | head -1 | cut -d'"' -f4)
			KIND="正式版"
		fi
		BASE=${LATEST#v}
		BASE=${BASE%-dev}
		if [[ "$BASE" != [0-9]*.[0-9]*.[0-9]* ]]; then
			echo "无法从最新 Release 解析版本号（latest=$LATEST）"; exit 1
		fi
		MAJ=${BASE%%.*}
		REST=${BASE#*.}
		MIN=${REST%%.*}
		PAT=${REST#*.}
		PAT=$((PAT + 1))
		VER="$MAJ.$MIN.$PAT"
		if [ "$KIND" = "dev" ]; then
			VER="$VER-dev"
			echo "最新 dev 版 $LATEST → 自动生成 dev 版 $VER"
		else
			echo "最新正式版 $LATEST → 自动生成正式版 v$VER"
		fi
		;;
	*)
		VER="$ARG1"
		;;
esac
TAG="v${VER#v}"
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
		push_tag
	else
		git tag "$TAG"
		push_tag
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

