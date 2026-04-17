#!/bin/bash

SERVER="103.254.148.165"
CHECK_INTERVAL=30

echo "开始监控服务器 $SERVER 的连接状态..."
echo "检查间隔: ${CHECK_INTERVAL}秒"
echo "按 Ctrl+C 停止监控"
echo ""

while true; do
    TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
    
    # 尝试ping
    if ping -c 1 -W 3 "$SERVER" > /dev/null 2>&1; then
        echo "[$TIMESTAMP] ✅ PING成功 - 服务器已恢复！"
        
        # 尝试SSH连接
        if timeout 5 bash -c "echo > /dev/tcp/$SERVER/22" 2>/dev/null; then
            echo "[$TIMESTAMP] ✅ SSH端口(22)可访问！"
            echo ""
            echo "服务器已恢复，可以运行验证脚本："
            echo "  cd /Users/a1/projects/iptables-safe"
            echo "  ./verify-deployment.sh"
            break
        else
            echo "[$TIMESTAMP] ⚠️  PING成功但SSH端口不可访问"
        fi
    else
        echo "[$TIMESTAMP] ❌ 服务器无响应（100%丢包）"
    fi
    
    sleep $CHECK_INTERVAL
done
