# IPTables Safe - IP白名单管理系统

一个基于Go开发的iptables防火墙管理应用，提供Web界面进行IP白名单管理。

## 功能特性

- 🔒 **默认安全策略**：默认只开放22（SSH）和8888（HTTP）端口
- 🔐 **密码认证**：用户通过密码认证后自动加入IP白名单
- 🛡️ **防暴力破解**：限制登录频率，防止密码暴力破解（15分钟内失败5次将被锁定）
- ⏰ **临时白名单**：用户认证后IP自动加入白名单24小时
- 👨‍💼 **管理后台**：管理员可管理永久IP白名单
- 📝 **CRUD功能**：完整的IP白名单增删改查功能
- 🔑 **密码管理**：支持修改用户密码和管理员密码
- 💾 **纯Go SQLite**：使用modernc.org/sqlite，无需CGO，支持交叉编译
- 🔄 **自动恢复**：服务器重启后自动从数据库加载白名单
- 🎨 **现代化UI**：美观的Web界面
- ✅ **IP验证增强**：防止无效IP（0.0.0.0、空字符串等）被添加

## 系统要求

- CentOS 6 或更高版本
- root权限（用于管理iptables）
- iptables

## 默认密码

- **用户密码**：`022018`
- **管理员密码**：`admin123`
- **Web端口**：`8888`

⚠️ **重要**：首次部署后请立即修改默认密码！

## 快速部署（推荐）

**三种部署方式对比**：
- **方式一**：全自动安装（会自动下载Go并编译）- 适合首次部署
- **方式二**：使用预编译文件（无需Go环境）- 适合快速部署 ⭐
- **方式三**：源码编译（需要Go环境）- 仅在需要修改代码时使用

### 方式一：使用自动安装脚本（全自动）

从GitHub下载完整部署包并自动安装（会自动下载Go 1.15.15并编译）：

```bash
# 1. 下载部署包
cd /tmp
wget https://github.com/dong005/iptable-safe/archive/refs/heads/main.zip
unzip main.zip
cd iptable-safe-main

# 2. 执行自动安装脚本
chmod +x auto-install.sh
./auto-install.sh
```

自动安装脚本会完成：
- ✅ 安装必要工具（wget, gcc, sqlite）
- ✅ 自动下载并安装Go 1.15.15
- ✅ 创建安装目录 `/opt/iptables-safe`
- ✅ 解压项目文件
- ✅ 自动编译程序
- ✅ 配置iptables防火墙（只开放22和8888端口）
- ✅ 配置init.d开机自启服务
- ✅ 初始化数据库和密码
- ✅ 启动服务

安装完成后访问：`http://your-server-ip:8888`

### 方式二：Git克隆 + 预编译二进制（推荐）

直接使用仓库中的预编译Linux二进制文件，无需安装Go。

**选项A：使用自动化脚本（最简单）**

```bash
# 一键部署
curl -fsSL https://raw.githubusercontent.com/dong005/iptable-safe/main/git-deploy.sh | bash
```

或者手动下载后执行：

```bash
wget https://raw.githubusercontent.com/dong005/iptable-safe/main/git-deploy.sh
chmod +x git-deploy.sh
./git-deploy.sh
```

自动化脚本会完成：
- ✅ 检查并安装Git
- ✅ 克隆/更新仓库
- ✅ 使用预编译二进制文件
- ✅ 配置防火墙规则
- ✅ 配置init.d服务
- ✅ 启动服务

**选项B：手动部署**

```bash
# 1. 克隆仓库到安装目录
cd /opt
git clone https://github.com/dong005/iptable-safe.git iptables-safe
cd iptables-safe

# 2. 使用预编译的二进制文件（无需编译）
chmod +x iptables-safe-linux
mv iptables-safe-linux iptables-safe

# 3. 创建init.d服务
cat > /etc/init.d/iptables-safe << 'EOF'
#!/bin/bash
# chkconfig: 2345 90 10
# description: IPTables Safe Service

DAEMON=/opt/iptables-safe/iptables-safe
PIDFILE=/var/run/iptables-safe.pid
LOGFILE=/var/log/iptables-safe.log

case "$1" in
    start)
        echo "Starting iptables-safe..."
        cd /opt/iptables-safe
        nohup $DAEMON >> $LOGFILE 2>&1 &
        echo $! > $PIDFILE
        echo "Started"
        ;;
    stop)
        echo "Stopping iptables-safe..."
        if [ -f $PIDFILE ]; then
            kill $(cat $PIDFILE)
            rm -f $PIDFILE
            echo "Stopped"
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
        fi
        ;;
    *)
        echo "Usage: $0 {start|stop|restart|status}"
        exit 1
        ;;
esac
EOF

chmod +x /etc/init.d/iptables-safe
chkconfig --add iptables-safe
chkconfig iptables-safe on

# 4. 启动服务
service iptables-safe start
```

### 方式三：从源码编译（仅在需要修改代码时使用）

如果需要修改源码后重新编译（需要Go 1.15+环境）：

```bash
# 1. 克隆仓库
cd /opt
git clone https://github.com/dong005/iptable-safe.git iptables-safe
cd iptables-safe

# 2. 编译（需要先安装Go环境）
go build -o iptables-safe main.go

# 3. 按照方式二的步骤3-4配置服务
```

**注意**：大多数情况下使用方式一或方式二即可，无需从源码编译。

## 手动安装步骤

### 1. 安装Go环境（CentOS 6）

```bash
# 下载Go 1.15（兼容CentOS 6）
cd /tmp
wget https://golang.org/dl/go1.15.15.linux-amd64.tar.gz

# 解压到/usr/local
sudo tar -C /usr/local -xzf go1.15.15.linux-amd64.tar.gz

# 设置环境变量
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
echo 'export GOPATH=$HOME/go' >> ~/.bashrc
source ~/.bashrc

# 验证安装
go version
```

### 2. 编译应用

```bash
# 进入项目目录
cd /path/to/iptables-safe

# 下载依赖
go mod download

# 编译
go build -o iptables-safe main.go
```

### 3. 配置防火墙

```bash
# 确保iptables服务已安装
sudo yum install iptables iptables-services -y

# 启动iptables服务
sudo service iptables start
sudo chkconfig iptables on
```

### 4. 运行应用

```bash
# 需要root权限运行
sudo ./iptables-safe
```

## 使用systemd服务（推荐）

创建服务文件：

```bash
sudo nano /etc/systemd/system/iptables-safe.service
```

添加以下内容：

```ini
[Unit]
Description=IPTables Safe Service
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=/path/to/iptables-safe
ExecStart=/path/to/iptables-safe/iptables-safe
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

启动服务：

```bash
sudo systemctl daemon-reload
sudo systemctl start iptables-safe
sudo systemctl enable iptables-safe
sudo systemctl status iptables-safe
```

## 使用说明

### 用户访问

1. 访问 `http://your-server-ip:8888/`
2. 输入密码：`022018`
3. 认证成功后，您的IP将被加入白名单24小时

### 管理员访问

1. 访问 `http://your-server-ip:8888/admin`
2. 输入管理员密码：`admin123`
3. 进入管理后台

### 管理后台功能

- **IP白名单管理**
  - 查看所有白名单IP
  - 添加永久或临时IP白名单
  - 删除白名单IP
  
- **密码管理**
  - 修改用户密码
  - 修改管理员密码

## 安全建议

1. ✅ 首次部署后立即修改默认密码
2. ✅ 使用强密码（至少8位，包含大小写字母、数字和特殊字符）
3. ✅ 定期更换密码
4. ✅ 限制管理后台访问IP
5. ✅ 定期检查白名单IP列表
6. ✅ 启用HTTPS（建议使用Nginx反向代理）

## 目录结构

```
iptables-safe/
├── main.go                 # 主程序入口
├── go.mod                  # Go模块依赖
├── models/                 # 数据模型
│   └── models.go
├── database/               # 数据库操作
│   └── database.go
├── iptables/               # iptables管理
│   └── iptables.go
├── handlers/               # HTTP处理器
│   └── handlers.go
├── templates/              # HTML模板
│   ├── login.html
│   ├── admin_login.html
│   └── admin_dashboard.html
└── README.md
```

## 日志查看

```bash
# 查看服务日志
sudo journalctl -u iptables-safe -f

# 查看iptables规则
sudo iptables -L -n -v
```

## 故障排除

### 端口8888被占用

```bash
# 查看占用端口的进程
sudo netstat -tlnp | grep :8888

# 或者修改main.go中的端口号
# router.Run(":8080")  // 改为其他端口
```

### iptables规则未生效

```bash
# 手动初始化防火墙规则
sudo iptables -F
sudo iptables -X
sudo iptables -P INPUT DROP
sudo iptables -P FORWARD DROP
sudo iptables -P OUTPUT ACCEPT
sudo iptables -A INPUT -i lo -j ACCEPT
sudo iptables -A INPUT -m state --state ESTABLISHED,RELATED -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 22 -j ACCEPT
sudo iptables -A INPUT -p tcp --dport 8888 -j ACCEPT

# 保存规则
sudo service iptables save
```

### 数据库权限问题

```bash
# 确保数据库文件有正确的权限
sudo chmod 644 iptables-safe.db
sudo chown root:root iptables-safe.db
```

## 备份与恢复

### 备份数据库

```bash
cp iptables-safe.db iptables-safe.db.backup
```

### 恢复数据库

```bash
cp iptables-safe.db.backup iptables-safe.db
sudo systemctl restart iptables-safe
```

## 更新应用

```bash
# 停止服务
sudo systemctl stop iptables-safe

# 备份数据库
cp iptables-safe.db iptables-safe.db.backup

# 重新编译
go build -o iptables-safe main.go

# 启动服务
sudo systemctl start iptables-safe
```

## API接口

### 用户认证
- `POST /api/login` - 用户登录认证

### 管理员接口（需要认证）
- `POST /api/admin/login` - 管理员登录
- `GET /api/admin/whitelist` - 获取白名单列表
- `POST /api/admin/whitelist` - 添加白名单IP
- `DELETE /api/admin/whitelist/:id` - 删除白名单IP
- `PUT /api/admin/password/user` - 修改用户密码
- `PUT /api/admin/password/admin` - 修改管理员密码

## 技术栈

- **后端**：Go 1.15+, Gin Web Framework
- **数据库**：SQLite (modernc.org/sqlite - 纯Go实现，无需CGO)
- **前端**：HTML5, CSS3, JavaScript (Vanilla)
- **安全**：bcrypt密码加密, 登录频率限制, IP验证增强
- **系统**：iptables防火墙管理
- **部署**：支持本地交叉编译（macOS → Linux）

## 许可证

MIT License

## 联系方式

如有问题或建议，请联系系统管理员。
