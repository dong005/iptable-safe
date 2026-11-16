#!/bin/bash
##############################################
# iptables-safe 本机部署脚本（SRC加固版）
# 默认端口 8899；自动安装缺失依赖并编译部署
##############################################
set -e

INSTALL_DIR="/opt/iptables-safe"
SRC_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PORT="${IPTABLES_SAFE_PORT:-8899}"
GO_BIN=""

echo "======================================"
echo " iptables-safe 部署 (port=$PORT)"
echo "======================================"

if [ "$EUID" -ne 0 ]; then
  echo "请使用 root 运行"
  exit 1
fi

need_cmd() {
  command -v "$1" >/dev/null 2>&1
}

echo "[1/6] 安装系统依赖..."
PKGS=""
need_cmd wget || PKGS="$PKGS wget"
need_cmd curl || PKGS="$PKGS curl"
need_cmd unzip || PKGS="$PKGS unzip"
need_cmd gcc || PKGS="$PKGS gcc"
need_cmd iptables || PKGS="$PKGS iptables"
if [ -n "$PKGS" ]; then
  if need_cmd yum; then
    yum install -y $PKGS || true
  elif need_cmd apt-get; then
    apt-get update -y && apt-get install -y $PKGS || true
  fi
fi
echo "依赖检查完成"

echo "[2/6] 准备 Go 环境..."
pick_go() {
  for c in /usr/local/go1.24/bin/go /usr/local/go1.22/bin/go /usr/local/go/bin/go "$(command -v go 2>/dev/null)"; do
    if [ -x "$c" ]; then
      ver=$("$c" version 2>/dev/null || true)
      if echo "$ver" | grep -qE 'go1\.(2[2-9]|[3-9][0-9])'; then
        GO_BIN="$c"
        return 0
      fi
    fi
  done
  return 1
}

if ! pick_go; then
  echo "未找到可用 Go，自动下载 Go 1.22.10..."
  cd /tmp
  GO_TGZ=go1.22.10.linux-amd64.tar.gz
  if [ ! -f "$GO_TGZ" ]; then
    wget -O "$GO_TGZ" https://golang.google.cn/dl/$GO_TGZ \
      || wget -O "$GO_TGZ" https://go.dev/dl/$GO_TGZ \
      || curl -L -o "$GO_TGZ" https://mirrors.aliyun.com/golang/$GO_TGZ
  fi
  rm -rf /usr/local/go1.22
  mkdir -p /usr/local/go1.22
  tar -C /usr/local/go1.22 --strip-components=1 -xzf "$GO_TGZ"
  GO_BIN=/usr/local/go1.22/bin/go
fi
export PATH="$(dirname "$GO_BIN"):$PATH"
export GOPROXY=https://goproxy.cn,direct
export GO111MODULE=on
export GOTOOLCHAIN=auto
"$GO_BIN" version

echo "[3/6] 编译..."
cd "$SRC_DIR"
"$GO_BIN" mod tidy
"$GO_BIN" build -o "$SRC_DIR/iptables-safe" .
echo "编译完成: $SRC_DIR/iptables-safe"

echo "[4/6] 停止旧服务..."
systemctl stop iptables-safe 2>/dev/null || true
pkill -f '/opt/iptables-safe/iptables-safe' 2>/dev/null || true
fuser -k ${PORT}/tcp 2>/dev/null || true
fuser -k 8888/tcp 2>/dev/null || true
sleep 1

echo "[5/6] 安装到 $INSTALL_DIR ..."
mkdir -p "$INSTALL_DIR"
# 保留已有数据库
if [ -f "$INSTALL_DIR/iptables-safe.db" ]; then
  cp -a "$INSTALL_DIR/iptables-safe.db" /tmp/iptables-safe.db.bak 2>/dev/null || true
fi
cp -a "$SRC_DIR/iptables-safe" "$INSTALL_DIR/iptables-safe"
cp -a "$SRC_DIR/templates" "$INSTALL_DIR/"
chmod +x "$INSTALL_DIR/iptables-safe"
if [ -f /tmp/iptables-safe.db.bak ]; then
  cp -a /tmp/iptables-safe.db.bak "$INSTALL_DIR/iptables-safe.db"
fi

# 迁移/设置端口到 DB（若库已存在）
if [ -f "$INSTALL_DIR/iptables-safe.db" ]; then
  "$INSTALL_DIR/iptables-safe" set-port "$PORT" || true
fi

cat > /etc/systemd/system/iptables-safe.service << EOF
[Unit]
Description=IPTables Safe - IP Whitelist Manager
After=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
Environment=IPTABLES_SAFE_PORT=$PORT
ExecStart=$INSTALL_DIR/iptables-safe
Restart=on-failure
RestartSec=3
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable iptables-safe >/dev/null 2>&1 || true

echo "[6/6] 启动服务..."
systemctl restart iptables-safe
sleep 2
systemctl --no-pager status iptables-safe || true
ss -tlnp | grep -E ":$PORT\\b" || netstat -tlnp | grep -E ":$PORT\\b" || true

echo ""
echo "部署完成"
echo "  用户页:   http://<IP>:$PORT/"
echo "  管理页:   http://<IP>:$PORT/admin"
echo "  默认密码: 用户 022018 / 管理员 admin123"
echo "  改密码:   $INSTALL_DIR/iptables-safe passwd user|admin <新密码>"
echo "  改端口:   管理后台「服务端口」或 $INSTALL_DIR/iptables-safe set-port <端口>"
