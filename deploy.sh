#!/bin/bash
# 停止所有旧进程
killall -9 iptables-safe-linux 2>/dev/null
killall -9 iptables-safe-new 2>/dev/null
fuser -k 8888/tcp 2>/dev/null

# 等待端口释放
sleep 3

# 备份旧程序
if [ -f /root/iptables-safe-linux ]; then
    mv /root/iptables-safe-linux /root/iptables-safe-linux.bak
fi

# 使用新程序
mv /root/iptables-safe-new /root/iptables-safe-linux
chmod +x /root/iptables-safe-linux

# 启动新程序
cd /root
nohup ./iptables-safe-linux > iptables-safe.log 2>&1 &

echo "程序已重启"
