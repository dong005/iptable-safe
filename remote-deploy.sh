#!/bin/bash

# 远程服务器配置
REMOTE_HOST="192.168.11.4"
REMOTE_USER="root"
REMOTE_DIR="/root"
SERVICE_NAME="iptables-safe"
SSH_OPTS="-o StrictHostKeyChecking=no -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedKeyTypes=+ssh-rsa -i ~/.ssh/id_rsa"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${GREEN}=== 开始部署 iptables-safe 到远程服务器 ===${NC}"

# 1. 测试SSH连接
echo -e "\n${GREEN}[1/6] 测试SSH连接...${NC}"
ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" "echo '连接成功'" || {
    echo -e "${RED}SSH连接失败，请检查服务器地址和密钥配置${NC}"
    exit 1
}

# 2. 停止旧服务
echo -e "\n${GREEN}[2/6] 停止旧服务...${NC}"
ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" << 'EOF'
systemctl stop iptables-safe 2>/dev/null || true
killall -9 iptables-safe-linux 2>/dev/null || true
killall -9 iptables-safe-new 2>/dev/null || true
fuser -k 8888/tcp 2>/dev/null || true
sleep 2
EOF

# 3. 创建远程目录并上传项目文件
echo -e "\n${GREEN}[3/6] 创建远程目录...${NC}"
ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" << 'EOF'
mkdir -p /root/templates
EOF

echo -e "\n${GREEN}[3/6] 上传项目文件...${NC}"
# 上传主程序
scp $SSH_OPTS \
    iptables-safe-linux \
    "$REMOTE_USER@$REMOTE_HOST:$REMOTE_DIR/" || {
    echo -e "${RED}主程序上传失败${NC}"
    exit 1
}

# 上传模板文件
scp $SSH_OPTS \
    templates/*.html \
    "$REMOTE_USER@$REMOTE_HOST:$REMOTE_DIR/templates/" || {
    echo -e "${RED}模板文件上传失败${NC}"
    exit 1
}

# 4. 设置权限
echo -e "\n${GREEN}[4/6] 设置文件权限...${NC}"
ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" << 'EOF'
chmod +x /root/iptables-safe-linux
EOF

# 5. 创建init.d服务脚本（开机自启）
echo -e "\n${GREEN}[5/6] 创建init.d服务脚本（开机自启）...${NC}"
ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" << 'EOF'
cat > /etc/init.d/iptables-safe << 'INITSCRIPT'
#!/bin/bash
# chkconfig: 2345 90 10
# description: IPTables Safe Management Service

PROG_NAME="iptables-safe-linux"
PROG_PATH="/root/iptables-safe-linux"
LOG_FILE="/root/iptables-safe.log"
PID_FILE="/var/run/iptables-safe.pid"

start() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Service is already running"
        return 1
    fi
    echo "Starting $PROG_NAME..."
    cd /root
    nohup "$PROG_PATH" >> "$LOG_FILE" 2>&1 &
    echo $! > "$PID_FILE"
    sleep 2
    if kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Service started successfully"
        return 0
    else
        echo "Service failed to start"
        return 1
    fi
}

stop() {
    if [ ! -f "$PID_FILE" ]; then
        echo "Service is not running"
        return 1
    fi
    echo "Stopping $PROG_NAME..."
    PID=$(cat "$PID_FILE")
    kill -15 "$PID" 2>/dev/null
    sleep 2
    if kill -0 "$PID" 2>/dev/null; then
        kill -9 "$PID" 2>/dev/null
    fi
    rm -f "$PID_FILE"
    killall -9 "$PROG_NAME" 2>/dev/null
    echo "Service stopped"
    return 0
}

status() {
    if [ -f "$PID_FILE" ] && kill -0 $(cat "$PID_FILE") 2>/dev/null; then
        echo "Service is running (PID: $(cat $PID_FILE))"
        return 0
    else
        echo "Service is not running"
        return 1
    fi
}

case "$1" in
    start)
        start
        ;;
    stop)
        stop
        ;;
    restart)
        stop
        sleep 2
        start
        ;;
    status)
        status
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
exit $?
INITSCRIPT

# 设置脚本权限
chmod +x /etc/init.d/iptables-safe

# 添加到开机自启动
chkconfig --add iptables-safe
chkconfig iptables-safe on

# 启动服务
/etc/init.d/iptables-safe start

# 等待服务启动
sleep 3

# 检查服务状态
/etc/init.d/iptables-safe status
EOF

# 6. 验证部署
echo -e "\n${GREEN}[6/6] 验证部署...${NC}"
ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" << 'EOF'
echo "=== 服务状态 ==="
/etc/init.d/iptables-safe status
echo ""
chkconfig --list | grep iptables-safe

echo -e "\n=== 进程信息 ==="
ps aux | grep iptables-safe-linux | grep -v grep

echo -e "\n=== 端口监听 ==="
netstat -tlnp | grep 8888 2>/dev/null

echo -e "\n=== 最近日志 ==="
tail -n 20 /root/iptables-safe.log 2>/dev/null || echo "日志文件尚未生成"
EOF

echo -e "\n${GREEN}=== 部署完成 ===${NC}"
echo -e "${GREEN}服务已设置为开机自动启动${NC}"
echo -e "${GREEN}访问地址: http://103.254.148.168:8888${NC}"
