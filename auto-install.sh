#!/bin/bash

##############################################
# IPTables Safe 自动安装脚本
# 适用于 CentOS 6/7
# 使用方法: chmod +x auto-install.sh && ./auto-install.sh
##############################################

set -e

INSTALL_DIR="/opt/iptables-safe"
LOG_FILE="/var/log/iptables-safe-install.log"

echo "======================================"
echo "IPTables Safe 自动安装程序"
echo "======================================"
echo ""

# 检查是否为root用户
if [ "$EUID" -ne 0 ]; then 
    echo "错误: 请使用root权限运行此脚本"
    echo "使用方法: sudo ./auto-install.sh"
    exit 1
fi

# 记录日志
exec > >(tee -a "$LOG_FILE")
exec 2>&1

echo "[$(date '+%Y-%m-%d %H:%M:%S')] 开始安装..."
echo ""

# 步骤1: 安装必要工具
echo "[1/8] 安装必要工具..."
yum install -y wget gcc sqlite 2>/dev/null || {
    echo "警告: 部分工具安装失败，继续尝试..."
}
echo "✓ 工具安装完成"
echo ""

# 步骤2: 检查并安装Go环境
echo "[2/8] 检查Go环境..."
if ! command -v /usr/local/go/bin/go &> /dev/null; then
    echo "正在安装Go 1.15.15..."
    cd /tmp
    
    # 检查是否已下载
    if [ ! -f go1.15.15.linux-amd64.tar.gz ]; then
        echo "下载Go安装包（约120MB，请耐心等待）..."
        wget https://golang.org/dl/go1.15.15.linux-amd64.tar.gz || {
            echo "使用国内镜像下载..."
            wget https://golang.google.cn/dl/go1.15.15.linux-amd64.tar.gz
        }
    fi
    
    echo "解压安装Go..."
    tar -C /usr/local -xzf go1.15.15.linux-amd64.tar.gz
    echo "✓ Go安装完成"
else
    echo "✓ Go已安装"
fi

# 设置Go环境变量
export PATH=$PATH:/usr/local/go/bin
export GOPATH=/root/go
export GO111MODULE=on
export GOPROXY=https://goproxy.cn,direct

/usr/local/go/bin/go version
echo ""

# 步骤3: 创建安装目录
echo "[3/8] 创建安装目录..."
mkdir -p $INSTALL_DIR
cd $INSTALL_DIR
echo "✓ 目录创建完成: $INSTALL_DIR"
echo ""

# 步骤4: 解压项目文件（假设当前目录有tar包）
echo "[4/8] 解压项目文件..."
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# 查找tar包的位置
TAR_FILE=""
if [ -f "$SCRIPT_DIR/iptables-safe.tar.gz" ]; then
    TAR_FILE="$SCRIPT_DIR/iptables-safe.tar.gz"
elif [ -f "./iptables-safe.tar.gz" ]; then
    TAR_FILE="./iptables-safe.tar.gz"
elif [ -f "/tmp/iptables-safe.tar.gz" ]; then
    TAR_FILE="/tmp/iptables-safe.tar.gz"
fi

if [ -n "$TAR_FILE" ]; then
    tar xzf "$TAR_FILE" -C $INSTALL_DIR
    echo "✓ 项目文件解压完成"
elif [ -f "$INSTALL_DIR/main.go" ]; then
    echo "✓ 项目文件已存在"
else
    echo "错误: 找不到项目文件 iptables-safe.tar.gz"
    echo "请确保 auto-install.sh 和 iptables-safe.tar.gz 在同一目录"
    exit 1
fi
echo ""

# 步骤5: 编译程序
echo "[5/8] 编译程序..."
cd $INSTALL_DIR
/usr/local/go/bin/go build -o iptables-safe main.go
chmod +x iptables-safe

if [ -f iptables-safe ]; then
    SIZE=$(ls -lh iptables-safe | awk '{print $5}')
    echo "✓ 编译完成: iptables-safe ($SIZE)"
else
    echo "错误: 编译失败"
    exit 1
fi
echo ""

# 步骤6: 配置iptables防火墙
echo "[6/8] 配置iptables防火墙..."

# 安装iptables
yum install -y iptables iptables-services 2>/dev/null || true

# 备份现有规则
if command -v iptables-save &> /dev/null; then
    BACKUP_FILE="/root/iptables-backup-$(date +%Y%m%d-%H%M%S).rules"
    iptables-save > "$BACKUP_FILE" 2>/dev/null && echo "已备份现有规则到: $BACKUP_FILE"
fi

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
service iptables save 2>/dev/null || \
iptables-save > /etc/sysconfig/iptables 2>/dev/null || \
iptables-save > /etc/iptables/rules.v4 2>/dev/null || true

echo "✓ 防火墙配置完成"
iptables -L -n | head -10
echo ""

# 步骤7: 配置开机自启服务
echo "[7/8] 配置开机自启服务..."

# 检查系统使用systemd还是init.d
if [ -d /etc/systemd/system ]; then
    # 使用systemd (CentOS 7+)
    echo "检测到systemd，创建systemd服务..."
    cat > /etc/systemd/system/iptables-safe.service <<EOF
[Unit]
Description=IPTables Safe - IP Whitelist Management
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/iptables-safe
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
    
    systemctl daemon-reload
    systemctl enable iptables-safe
    echo "✓ systemd服务配置完成"
else
    # 使用init.d (CentOS 6)
    echo "检测到init.d，创建init.d服务..."
    cat > /etc/init.d/iptables-safe <<'EOF'
#!/bin/bash
# chkconfig: 2345 90 10
# description: IPTables Safe - IP Whitelist Management

DAEMON=/opt/iptables-safe/iptables-safe
PIDFILE=/var/run/iptables-safe.pid
LOGFILE=/var/log/iptables-safe.log

start() {
    echo "Starting iptables-safe..."
    if [ -f $PIDFILE ]; then
        echo "Service already running"
        return 1
    fi
    cd /opt/iptables-safe
    nohup $DAEMON > $LOGFILE 2>&1 &
    echo $! > $PIDFILE
    echo "Started"
}

stop() {
    echo "Stopping iptables-safe..."
    if [ -f $PIDFILE ]; then
        kill $(cat $PIDFILE) 2>/dev/null
        rm -f $PIDFILE
        echo "Stopped"
    else
        echo "Service not running"
    fi
}

status() {
    if [ -f $PIDFILE ]; then
        PID=$(cat $PIDFILE)
        if ps -p $PID > /dev/null 2>&1; then
            echo "iptables-safe is running (PID: $PID)"
            return 0
        else
            echo "iptables-safe is not running (stale PID file)"
            return 1
        fi
    else
        echo "iptables-safe is not running"
        return 3
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
esac
exit 0
EOF
    
    chmod +x /etc/init.d/iptables-safe
    chkconfig --add iptables-safe
    chkconfig iptables-safe on
    echo "✓ init.d服务配置完成"
fi
echo ""

# 步骤8: 初始化数据库密码
echo "[8/8] 初始化数据库密码..."
cat > $INSTALL_DIR/init_password.go <<'EOF'
package main

import (
	"database/sql"
	"fmt"
	"log"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	db, err := sql.Open("sqlite3", "./iptables-safe.db")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	var count int
	db.QueryRow("SELECT COUNT(*) FROM config").Scan(&count)
	
	if count == 0 {
		userHash, _ := bcrypt.GenerateFromPassword([]byte("022018"), bcrypt.DefaultCost)
		adminHash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		_, err = db.Exec("INSERT INTO config (user_password, admin_password) VALUES (?, ?)", string(userHash), string(adminHash))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("✓ 默认密码已初始化")
	} else {
		adminHash, _ := bcrypt.GenerateFromPassword([]byte("admin123"), bcrypt.DefaultCost)
		_, err = db.Exec("UPDATE config SET admin_password = ? WHERE id = 1", string(adminHash))
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println("✓ 管理员密码已重置")
	}
}
EOF

# 启动服务以创建数据库
echo "启动服务..."
if [ -d /etc/systemd/system ]; then
    systemctl start iptables-safe
    sleep 3
    systemctl stop iptables-safe
else
    service iptables-safe start
    sleep 3
    service iptables-safe stop
fi

# 初始化密码
cd $INSTALL_DIR
/usr/local/go/bin/go run init_password.go
rm -f init_password.go
echo ""

# 最终启动服务
echo "启动服务..."
if [ -d /etc/systemd/system ]; then
    systemctl start iptables-safe
    sleep 2
    systemctl status iptables-safe --no-pager || true
else
    service iptables-safe start
    sleep 2
    service iptables-safe status || true
fi
echo ""

# 获取服务器IP
SERVER_IP=$(hostname -I | awk '{print $1}')

echo "======================================"
echo "✓ 安装完成！"
echo "======================================"
echo ""
echo "服务信息:"
echo "  - 安装目录: $INSTALL_DIR"
echo "  - 日志文件: /var/log/iptables-safe.log"
echo "  - 配置文件: $INSTALL_DIR/iptables-safe.db"
echo ""
echo "访问地址:"
echo "  🌐 用户登录: http://$SERVER_IP/"
echo "  👨‍💼 管理后台: http://$SERVER_IP/admin"
echo ""
echo "默认密码:"
echo "  🔑 用户密码: 022018"
echo "  🔑 管理员密码: admin123"
echo ""
echo "⚠️  重要: 请立即登录修改默认密码！"
echo ""
echo "管理命令:"
if [ -d /etc/systemd/system ]; then
    echo "  systemctl status iptables-safe    # 查看状态"
    echo "  systemctl restart iptables-safe   # 重启服务"
    echo "  systemctl stop iptables-safe      # 停止服务"
    echo "  journalctl -u iptables-safe -f    # 查看日志"
else
    echo "  service iptables-safe status      # 查看状态"
    echo "  service iptables-safe restart     # 重启服务"
    echo "  service iptables-safe stop        # 停止服务"
    echo "  tail -f /var/log/iptables-safe.log # 查看日志"
fi
echo "  iptables -L -n -v                 # 查看防火墙"
echo ""
echo "安装日志已保存到: $LOG_FILE"
echo ""
