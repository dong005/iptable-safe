#!/bin/bash
#
# IPTables Safe - Git部署自动化脚本
# 使用预编译二进制文件，无需安装Go环境
#

set -e

INSTALL_DIR="/opt/iptables-safe"
SERVICE_NAME="iptables-safe"
REPO_URL="https://github.com/dong005/iptable-safe.git"
LOG_FILE="/var/log/iptables-safe-install.log"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo "======================================"
echo "  IPTables Safe - Git部署脚本"
echo "======================================"
echo ""

# 检查root权限
if [ "$EUID" -ne 0 ]; then 
    echo -e "${RED}错误: 请使用root权限运行此脚本${NC}"
    echo "使用方法: sudo $0"
    exit 1
fi

# 记录日志
exec > >(tee -a "$LOG_FILE")
exec 2>&1

echo "[1/7] 检查系统环境..."

# 检查git是否安装
if ! command -v git &> /dev/null; then
    echo "Git未安装，正在安装..."
    yum install -y git || {
        echo -e "${RED}错误: Git安装失败${NC}"
        exit 1
    }
fi

# 检查是否已安装
if [ -d "$INSTALL_DIR" ]; then
    echo -e "${YELLOW}警告: 安装目录已存在${NC}"
    read -p "是否覆盖安装？(y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        echo "取消安装"
        exit 0
    fi
    
    # 停止服务
    if [ -f /etc/init.d/$SERVICE_NAME ]; then
        echo "停止现有服务..."
        service $SERVICE_NAME stop 2>/dev/null || true
    fi
    
    # 备份数据库
    if [ -f "$INSTALL_DIR/iptables-safe.db" ]; then
        echo "备份数据库..."
        cp "$INSTALL_DIR/iptables-safe.db" "$INSTALL_DIR/iptables-safe.db.backup.$(date +%Y%m%d_%H%M%S)"
    fi
    
    # 删除旧文件（保留数据库）
    find "$INSTALL_DIR" -type f ! -name "*.db*" -delete 2>/dev/null || true
fi

echo "✓ 系统环境检查完成"
echo ""

echo "[2/7] 克隆Git仓库..."
if [ -d "$INSTALL_DIR/.git" ]; then
    echo "更新现有仓库..."
    cd "$INSTALL_DIR"
    git pull origin main || {
        echo -e "${RED}错误: Git更新失败${NC}"
        exit 1
    }
else
    echo "克隆仓库..."
    rm -rf "$INSTALL_DIR"
    git clone "$REPO_URL" "$INSTALL_DIR" || {
        echo -e "${RED}错误: Git克隆失败${NC}"
        exit 1
    }
    cd "$INSTALL_DIR"
fi
echo "✓ 仓库克隆/更新完成"
echo ""

echo "[3/7] 配置程序..."
# 使用预编译的二进制文件
if [ ! -f "$INSTALL_DIR/iptables-safe-linux" ]; then
    echo -e "${RED}错误: 找不到预编译文件 iptables-safe-linux${NC}"
    exit 1
fi

chmod +x "$INSTALL_DIR/iptables-safe-linux"
cp "$INSTALL_DIR/iptables-safe-linux" "$INSTALL_DIR/iptables-safe"
chmod +x "$INSTALL_DIR/iptables-safe"

echo "✓ 程序配置完成"
echo ""

echo "[4/7] 配置防火墙规则..."
# 配置防火墙规则（只开放22和8888端口）
echo "配置防火墙规则（只开放22和8888端口）..."
iptables -F
iptables -X
iptables -P INPUT DROP
iptables -P FORWARD DROP
iptables -P OUTPUT ACCEPT
iptables -A INPUT -i lo -j ACCEPT
iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
iptables -A INPUT -p tcp --dport 22 -j ACCEPT
iptables -A INPUT -p tcp --dport 8888 -j ACCEPT

# 保存规则
if command -v iptables-save &> /dev/null; then
    iptables-save > /etc/sysconfig/iptables 2>/dev/null || \
    iptables-save > /etc/iptables/rules.v4 2>/dev/null || true
fi

echo "✓ 防火墙规则配置完成"
echo ""

echo "[5/7] 配置系统服务..."

# 创建init.d服务脚本
cat > /etc/init.d/$SERVICE_NAME << 'EOF'
#!/bin/bash
# chkconfig: 2345 90 10
# description: IPTables Safe Service

DAEMON=/opt/iptables-safe/iptables-safe
PIDFILE=/var/run/iptables-safe.pid
LOGFILE=/var/log/iptables-safe.log

case "$1" in
    start)
        echo "Starting iptables-safe..."
        if [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null; then
            echo "iptables-safe is already running"
            exit 0
        fi
        cd /opt/iptables-safe
        nohup $DAEMON >> $LOGFILE 2>&1 &
        echo $! > $PIDFILE
        echo "Started"
        ;;
    stop)
        echo "Stopping iptables-safe..."
        if [ -f $PIDFILE ]; then
            kill $(cat $PIDFILE) 2>/dev/null || true
            rm -f $PIDFILE
            echo "Stopped"
        else
            echo "iptables-safe is not running"
        fi
        ;;
    restart)
        $0 stop
        sleep 2
        $0 start
        ;;
    status)
        if [ -f $PIDFILE ] && kill -0 $(cat $PIDFILE) 2>/dev/null; then
            echo "iptables-safe is running (PID: $(cat $PIDFILE))"
        else
            echo "iptables-safe is not running"
            if [ -f $PIDFILE ]; then
                echo "(stale PID file)"
            fi
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
exit 0
EOF

chmod +x /etc/init.d/$SERVICE_NAME

# 配置开机自启
if command -v chkconfig &> /dev/null; then
    chkconfig --del $SERVICE_NAME 2>/dev/null || true
    chkconfig --add $SERVICE_NAME
    chkconfig $SERVICE_NAME on
elif command -v update-rc.d &> /dev/null; then
    update-rc.d $SERVICE_NAME defaults
fi

echo "✓ 系统服务配置完成"
echo ""

echo "[6/7] 初始化数据库..."
# 如果没有数据库文件，程序启动时会自动创建
if [ ! -f "$INSTALL_DIR/iptables-safe.db" ]; then
    echo "数据库将在首次启动时自动创建"
else
    echo "使用现有数据库文件"
fi
echo "✓ 数据库初始化完成"
echo ""

echo "[7/7] 启动服务..."
service $SERVICE_NAME start

# 等待服务启动
sleep 3

# 检查服务状态
if service $SERVICE_NAME status | grep -q "running"; then
    echo "✓ 服务启动成功"
else
    echo -e "${YELLOW}警告: 服务可能未正常启动，请检查日志${NC}"
fi

echo ""
echo "======================================"
echo "  安装完成！"
echo "======================================"
echo ""
echo "访问地址: http://$(hostname -I | awk '{print $1}'):8888"
echo ""
echo "默认密码:"
echo "  - 用户密码: 022018"
echo "  - 管理员密码: admin123"
echo ""
echo "⚠️  重要: 请立即修改默认密码！"
echo ""
echo "管理命令:"
echo "  - 启动服务: service $SERVICE_NAME start"
echo "  - 停止服务: service $SERVICE_NAME stop"
echo "  - 重启服务: service $SERVICE_NAME restart"
echo "  - 查看状态: service $SERVICE_NAME status"
echo "  - 查看日志: tail -f /var/log/iptables-safe.log"
echo ""
echo "安装日志: $LOG_FILE"
echo ""
