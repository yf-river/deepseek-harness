#!/usr/bin/env bash
# 发布 ChainWeaver 门户到 40.247。
#
# 流程固定为：本地开发 → push 到 origin/dev → 远端 git pull → 远端构建并部署。
# 远端只拉代码，不在远端直接改代码；任何改动必须先在本地提交并推送。
#
#   ./deploy/release.sh            # 完整发布
#   ./deploy/release.sh --check    # 只检查本地是否已全部推送，不发布
set -euo pipefail

TARGET_HOST="192.168.40.247"
TARGET_DIR="/opt/chainweaver-studio-remote"
REMOTE_GIT="https://github.com/yf-river/deepseek-harness.git"
BRANCH="dev"
SERVICE="chainweaver-portal"
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
REMOTE_SHA="$(git ls-remote origin "refs/heads/$BRANCH" | cut -f1)"
if [ "$LOCAL_SHA" != "$REMOTE_SHA" ]; then
  echo "!! 本地 HEAD ($LOCAL_SHA) 与 origin/$BRANCH ($REMOTE_SHA) 不一致，先 push：" >&2
  echo "   git push origin $BRANCH" >&2
  exit 1
fi
echo "    本地 = origin/$BRANCH = $LOCAL_SHA"

echo "==> 2/5 本地测试"
GOFLAGS=-mod=mod go vet ./...
GOFLAGS=-mod=mod go test ./... -timeout 120s

if [ "$CHECK_ONLY" -eq 1 ]; then
  echo "==> 仅检查模式，到此为止"
  exit 0
fi

echo "==> 3/5 远端拉代码"
ssh -o BatchMode=yes "$TARGET_HOST" "
  set -euo pipefail
  if [ ! -d $TARGET_DIR/.git ]; then
    git clone --branch $BRANCH $REMOTE_GIT $TARGET_DIR
  fi
  cd $TARGET_DIR
  git fetch --prune origin
  git checkout $BRANCH
  git reset --hard origin/$BRANCH
  echo -n '    远端 HEAD: '; git rev-parse HEAD
"

echo "==> 4/5 远端构建"
ssh -o BatchMode=yes "$TARGET_HOST" "
  set -euo pipefail
  cd $TARGET_DIR
  GOFLAGS=-mod=mod CGO_ENABLED=0 go build -trimpath -ldflags '-s -w' -o /tmp/chainweaver-studio.build .
  chmod 755 /tmp/chainweaver-studio.build
"

echo "==> 5/5 安装并重启"
ssh -o BatchMode=yes "$TARGET_HOST" "
  set -euo pipefail
  # 二进制换 inode 覆盖，避免 Text file busy
  cp /opt/chainweaver-portal/chainweaver-portal /opt/chainweaver-portal/chainweaver-portal.bak-\$(date +%Y%m%d%H%M%S) 2>/dev/null || true
  mv /tmp/chainweaver-studio.build /opt/chainweaver-portal/chainweaver-portal
  # 40.247 专用配置：cwe 同机直读本地 state.json
  cp $TARGET_DIR/deploy/40.247/sites.yaml /opt/chainweaver-portal/config/sites.yaml
  mkdir -p /opt/chainweaver-portal/run/robot
  systemctl restart $SERVICE
  sleep 3
  systemctl is-active $SERVICE
"

echo "==> 冒烟验证"
for path in / /api/config /api/sites /api/env /api/robot; do
  code="$(curl -s -o /dev/null -w '%{http_code}' -m 10 "http://$TARGET_HOST:18090$path" || echo 000)"
  printf '  %-16s %s\n' "$path" "$code"
  [ "$code" = "200" ] || { echo "!! $path 返回 $code" >&2; exit 1; }
done

echo "==> 完成：http://$TARGET_HOST:18090/  ($LOCAL_SHA)"
