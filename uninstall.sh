#!/bin/bash

set -e

echo "======================================"
echo "IPTables Safe 卸载脚本"
echo "======================================"
echo ""

if [ "$EUID" -ne 0 ]; then 
    echo "错误: 请使用root权限运行此脚本"
    echo "使用方法: sudo bash uninstall.sh"
    exit 1
fi

read -p "确定要卸载IPTables Safe吗？(y/N) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "取消卸载"
    exit 0
fi

echo ""
echo "[1/3] 停止并禁用服务..."
systemctl stop iptables-safe 2>/dev/null || true
systemctl disable iptables-safe 2>/dev/null || true
rm -f /etc/systemd/system/iptables-safe.service
systemctl daemon-reload
echo "服务已停止"

echo ""
echo "[2/3] 清理防火墙规则..."
read -p "是否重置iptables规则到默认状态？(y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    iptables -F
    iptables -X
    iptables -P INPUT ACCEPT
    iptables -P FORWARD ACCEPT
    iptables -P OUTPUT ACCEPT
    service iptables save 2>/dev/null || true
    echo "防火墙规则已重置"
else
    echo "保留现有防火墙规则"
fi

echo ""
echo "[3/3] 清理文件..."
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

read -p "是否删除数据库文件？(y/N) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    rm -f "$SCRIPT_DIR/iptables-safe.db"
    echo "数据库文件已删除"
else
    echo "保留数据库文件: $SCRIPT_DIR/iptables-safe.db"
fi

rm -f "$SCRIPT_DIR/iptables-safe"
echo "程序文件已删除"

echo ""
echo "======================================"
echo "卸载完成！"
echo "======================================"
echo ""
