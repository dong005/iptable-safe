#!/bin/bash

# 远程服务器配置
REMOTE_HOST="1.1.1.1"
REMOTE_USER="root"
REMOTE_PASS="022018"
SSH_OPTS="-o StrictHostKeyChecking=no -o HostKeyAlgorithms=+ssh-rsa -o PubkeyAcceptedKeyTypes=+ssh-rsa -o ConnectTimeout=20"

# 颜色输出
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

echo -e "${GREEN}=== 验证远程服务器部署状态 ===${NC}\n"

# 测试连接
echo -e "${YELLOW}测试网络连接...${NC}"
if ! ping -c 2 -W 3 "$REMOTE_HOST" > /dev/null 2>&1; then
    echo -e "${RED}警告: 无法ping通服务器，但SSH可能仍然可用${NC}\n"
fi

# 验证部署
echo -e "${YELLOW}连接服务器验证部署...${NC}\n"
sshpass -p "$REMOTE_PASS" ssh $SSH_OPTS "$REMOTE_USER@$REMOTE_HOST" << 'EOF'
echo "=== 连接成功 ==="
echo ""

echo "=== 服务状态 ==="
/etc/init.d/iptables-safe status 2>&1
echo ""

echo "=== 进程信息 ==="
ps aux | grep iptables-safe-linux | grep -v grep
echo ""

echo "=== 端口监听 ==="
netstat -tlnp 2>/dev/null | grep 8888
echo ""

echo "=== 开机自启配置 ==="
chkconfig --list 2>&1 | grep iptables-safe
echo ""

echo "=== 程序文件 ==="
ls -lh /root/iptables-safe-linux /root/templates/*.html 2>/dev/null
echo ""

echo "=== 最近日志（最后30行）==="
if [ -f /root/iptables-safe.log ]; then
    tail -n 30 /root/iptables-safe.log
else
    echo "日志文件不存在"
fi
EOF

if [ $? -eq 0 ]; then
    echo -e "\n${GREEN}=== 验证完成 ===${NC}"
    echo -e "${GREEN}访问地址: http://1.1.1.1:8899${NC}"
else
    echo -e "\n${RED}=== 验证失败，请检查网络连接 ===${NC}"
    exit 1
fi
