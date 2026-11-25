#!/usr/bin/env bash
# 发布 ChainWeaver 门户到 40.247。
#
# 流程：本地开发 → push 到 origin/dev → 把已推送的源码打包传过去 → 远端构建并部署。
# 远端只接收源码包并构建，不在远端直接改代码；任何改动都必须先在本地提交并推送。
#
# 为什么不用远端 git clone：这个仓库 fork 自 DSH，完整历史 380MB+，
# 且 40.247 访问 GitHub 频繁 Connection reset，全量与浅克隆都会中断。
# 改成传源码包后只需 300KB 左右，稳定且更快。
#
#   ./deploy/release.sh            # 完整发布
#   ./deploy/release.sh --check    # 只检查本地是否已全部推送，不发布
set -euo pipefail

TARGET_HOST="192.168.40.247"
REMOTE_SRC="/opt/chainweaver-portal/src"
INSTALL_DIR="/opt/chainweaver-portal"
SERVICE="chainweaver-portal"
BRANCH="dev"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
CHECK_ONLY=0
[ "${1:-}" = "--check" ] && CHECK_ONLY=1

cd "$PROJECT_DIR"

echo "==> 1/5 本地检查"
if [ -n "$(git status --porcelain)" ]; then
  echo "!! 本地有未提交改动，先 commit 再发布：" >&2
  git status --short >&2
  exit 1
fi
LOCAL_SHA="$(git rev-parse HEAD)"
# 用本地记录的 origin/<branch> 引用做校验，不实时查远端：
# 这台机器访问 GitHub 会间歇性 403/reset，实时查询不可靠。
# push 成功后该引用会更新，所以它仍能反映最近一次推送的结果。
TRACK_SHA="$(git rev-parse "origin/$BRANCH" 2>/dev/null || true)"
if [ "$LOCAL_SHA" != "$TRACK_SHA" ]; then
  echo "!! 本地 HEAD ($LOCAL_SHA) 与本地记录的 origin/$BRANCH ($TRACK_SHA) 不一致，先 push：" >&2
  echo "   git push origin $BRANCH" >&2
  exit 1
fi
echo "    本地 HEAD = origin/$BRANCH = $LOCAL_SHA"

echo "==> 2/5 本地测试"
GOFLAGS=-mod=mod go vet ./...
GOFLAGS=-mod=mod go test ./... -timeout 120s

if [ "$CHECK_ONLY" -eq 1 ]; then
  echo "==> 仅检查模式，到此为止"
  exit 0
fi

echo "==> 3/5 传源码（已推送的 HEAD，不含 .git）"
TARBALL="$(mktemp /tmp/studio-src.XXXXXX.tar.gz)"
trap 'rm -f "$TARBALL"' EXIT
# 只打包被 git 跟踪的文件，保证远端拿到的一定是已提交、已推送的内容。
git archive --format=tar.gz -o "$TARBALL" HEAD
echo "    包大小: $(du -h "$TARBALL" | cut -f1)"
scp -q -o BatchMode=yes "$TARBALL" "$TARGET_HOST:/tmp/studio-src.tar.gz"

echo "==> 4/5 远端解包并构建"
ssh -o BatchMode=yes "$TARGET_HOST" "
  set -euo pipefail
  rm -rf $REMOTE_SRC
  mkdir -p $REMOTE_SRC
  tar -xzf /tmp/studio-src.tar.gz -C $REMOTE_SRC
  rm -f /tmp/studio-src.tar.gz
  cd $REMOTE_SRC
  GOFLAGS=-mod=mod CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /tmp/chainweaver-studio.build .
  chmod 755 /tmp/chainweaver-studio.build
  echo -n '    远端构建完成: '; ls -lh /tmp/chainweaver-studio.build | awk '{print \$5}'
"

echo "==> 5/5 安装并重启"
ssh -o BatchMode=yes "$TARGET_HOST" "
  set -euo pipefail
  mkdir -p $INSTALL_DIR/config $INSTALL_DIR/run/robot
  # 备份当前二进制后换 inode 覆盖，避免 Text file busy
  cp $INSTALL_DIR/chainweaver-portal $INSTALL_DIR/chainweaver-portal.bak-\$(date +%Y%m%d%H%M%S) 2>/dev/null || true
  mv /tmp/chainweaver-studio.build $INSTALL_DIR/chainweaver-portal
  # 40.247 专用配置：cwe 同机直读本地 state.json
  cp $REMOTE_SRC/deploy/40.247/sites.yaml $INSTALL_DIR/config/sites.yaml
  echo $LOCAL_SHA > $INSTALL_DIR/REVISION
  systemctl restart $SERVICE
  sleep 3
  systemctl is-active $SERVICE
"

echo "==> 冒烟验证"
for path in / /api/config /api/sites /api/env /api/robot; do
  code="$(curl -s -o /dev/null -w '%{http_code}' -m 15 "http://$TARGET_HOST:18090$path" || echo 000)"
  printf '  %-16s %s\n' "$path" "$code"
  [ "$code" = "200" ] || { echo "!! $path 返回 $code" >&2; exit 1; }
done

echo "==> 完成：http://$TARGET_HOST:18090/  ($LOCAL_SHA)"
