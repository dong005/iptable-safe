# IPTables Safe - IP白名单管理系统

一个基于Go开发的iptables防火墙管理应用，提供Web界面进行IP白名单管理。

## 功能特性

- 🔒 **默认安全策略**：默认只开放22（SSH）和80（HTTP）端口
- 🔐 **密码认证**：用户通过密码认证后自动加入IP白名单
- 🛡️ **防暴力破解**：限制登录频率，防止密码暴力破解（15分钟内失败5次将被锁定）
- ⏰ **临时白名单**：用户认证后IP自动加入白名单24小时
- 👨‍💼 **管理后台**：管理员可管理永久IP白名单
- 📝 **CRUD功能**：完整的IP白名单增删改查功能
- 🔑 **密码管理**：支持修改用户密码和管理员密码
- 💾 **SQLite数据库**：轻量级数据库存储
- 🎨 **现代化UI**：美观的Web界面

## 系统要求

- CentOS 6 或更高版本
- Go 1.15 或更高版本
- root权限（用于管理iptables）
- iptables

## 默认密码

- **用户密码**：`022018`
- **管理员密码**：`admin123`

⚠️ **重要**：首次部署后请立即修改默认密码！

## 安装步骤

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

1. 访问 `http://your-server-ip/`
2. 输入密码：`022018`
3. 认证成功后，您的IP将被加入白名单24小时

### 管理员访问

1. 访问 `http://your-server-ip/admin`
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

### 端口80被占用

```bash
# 查看占用端口的进程
sudo netstat -tlnp | grep :80

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
sudo iptables -A INPUT -p tcp --dport 80 -j ACCEPT

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

- **后端**：Go 1.15, Gin Web Framework
- **数据库**：SQLite3
- **前端**：HTML5, CSS3, JavaScript (Vanilla)
- **安全**：bcrypt密码加密, 登录频率限制
- **系统**：iptables防火墙管理

## 许可证

MIT License

## 联系方式

如有问题或建议，请联系系统管理员。
