#!/usr/bin/env bash
# 把门户部署到本机或 40.247。先在本机跑通，再推远端。
#
#   ./deploy/deploy.sh local      # 本机 9.134.129.162:18094
#   ./deploy/deploy.sh 40.247     # 远端 192.168.40.247:18090
#
# 两种目标都用同一个二进制与同一份 config/sites.yaml，差别只在监听地址与 systemd unit。
set -euo pipefail

TARGET="${1:-}"
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INSTALL_DIR="/opt/chainweaver-studio"
BIN="chainweaver-studio"

case "$TARGET" in
  local)
    HOST_SSH=""
    LISTEN="9.134.129.162:18094"
    UNIT_SRC="$PROJECT_DIR/deploy/chainweaver-studio.local.service"
    UNIT_DST="/etc/systemd/system/chainweaver-studio-local.service"
    ;;
  40.247)
    HOST_SSH="192.168.40.247"
    LISTEN="192.168.40.247:18090"
    UNIT_SRC="$PROJECT_DIR/deploy/chainweaver-studio.service"
    UNIT_DST="/etc/systemd/system/chainweaver-studio.service"
    ;;
  *)
    echo "用法: $0 {local|40.247}" >&2
    exit 2
    ;;
esac

echo "==> 目标: $TARGET  安装目录: $INSTALL_DIR  监听: $LISTEN"

# 1) 编译前先过测试，避免把坏版本推上去。
echo "==> go vet + go test"
cd "$PROJECT_DIR"
GOFLAGS=-mod=mod go vet ./...
GOFLAGS=-mod=mod go test ./...

# 2) 交叉编译静态二进制，不依赖目标机 glibc。
echo "==> 编译"
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GOFLAGS=-mod=mod \
  go build -trimpath -ldflags "-s -w" -o "/tmp/$BIN" .

run() {
  if [ -z "$HOST_SSH" ]; then
    bash -lc "$1"
  else
    ssh -o BatchMode=yes "$HOST_SSH" "$1"
  fi
}

copy() {
  if [ -z "$HOST_SSH" ]; then
    cp "$1" "$2"
  else
    scp -o BatchMode=yes -q "$1" "$HOST_SSH:$2"
  fi
}

# 3) 铺文件：二进制、配置、unit。
# 二进制先落到 .new 再 mv 覆盖：直接 cp 到正在运行的二进制会报 "Text file busy"，
# 而 mv 是换 inode，运行中的进程继续用旧 inode，不会被打断。
echo "==> 拷贝产物"
run "mkdir -p $INSTALL_DIR/config $INSTALL_DIR/run/robot"
copy "/tmp/$BIN"             "$INSTALL_DIR/$BIN.new"
run "chmod 755 $INSTALL_DIR/$BIN.new && mv -f $INSTALL_DIR/$BIN.new $INSTALL_DIR/$BIN"
copy "$PROJECT_DIR/config/sites.yaml" "$INSTALL_DIR/config/sites.yaml"
copy "$UNIT_SRC"             "$UNIT_DST"

# 4) 启动与验证。配置缺失或非法时进程会直接退出，这里用 is-active 兜住。
SERVICE_NAME="$(basename "$UNIT_DST" .service)"
echo "==> 启动服务 (${SERVICE_NAME})"
run "chmod 600 $INSTALL_DIR/config/sites.yaml
     systemctl daemon-reload
     systemctl enable $SERVICE_NAME >/dev/null 2>&1
     systemctl restart $SERVICE_NAME
     systemctl is-active $SERVICE_NAME"

# 等端口真正可连再继续：固定 sleep 在机器忙时不够，会让后面的冒烟误判失败。
# 探测从执行机发出，本机与远端两种情况都适用。
echo "==> 等待服务就绪"
ready=0
for _ in 1 2 3 4 5 6 7 8 9 10 11 12 13 14 15 16 17 18 19 20 21 22 23 24 25 26 27 28 29 30; do
  if curl -sf -o /dev/null -m 2 "http://$LISTEN/"; then ready=1; break; fi
  sleep 1
done
if [ "$ready" -ne 1 ]; then
  echo "!! 服务在 30 秒内未就绪" >&2
  exit 1
fi

echo "==> 冒烟验证 $LISTEN"
BASE="http://$LISTEN"
for path in / /api/config /api/sites /assets/app.js /assets/styles.css /favicon.svg; do
  code="$(curl -s -o /dev/null -w '%{http_code}' -m 5 "$BASE$path" || echo 000)"
  printf '  %-20s %s\n' "$path" "$code"
  [ "$code" = "200" ] || { echo "!! $path 返回 $code" >&2; exit 1; }
done

echo "==> 完成: $BASE/"
