# IPTables Safe - IP白名单管理系统

基于 Go 的 iptables 防火墙管理应用：默认只开放认证端口，密码认证后将该来源 IP 加入白名单（全端口放行）。

## 功能特性

- **默认安全策略**：只开放认证端口 `8899`；认证成功后对该 IP `ACCEPT`（不默认对全世界开放 22）
- **密码认证**：用户登录后自动加入临时白名单（24 小时）
- **防暴力破解**：用户/管理员登录均限流（15 分钟内失败 5 次锁定）
- **会话鉴权**：管理后台使用服务端会话，伪造 Cookie 无效
- **防 IP 伪造**：不信任 `X-Forwarded-For` / `X-Real-IP`，仅使用直连地址
- **管理后台**：白名单 CRUD、改密、改端口（实时生效）、VOS 同步
- **命令行改密/改端口**：无需进后台也可维护
- **纯 Go SQLite**：`modernc.org/sqlite`，无需 CGO
- **自动恢复**：重启后从数据库加载白名单并重建规则

## 默认账号

| 项目 | 默认值 |
|------|--------|
| 用户密码 | `022018` |
| 管理员密码 | `admin123` |
| Web 端口 | `8899` |

⚠️ **首次部署后请立即修改默认密码。**

端口可用以下方式修改（任选）：

- 环境变量：`IPTABLES_SAFE_PORT=8899`
- 管理后台「服务端口」：保存后实时生效
- 命令行：`./iptables-safe set-port 8899`（需重启或后台热切换）

## 推荐部署（本机一键）

适用 CentOS 7+，会自动安装缺失依赖、编译并安装到 `/opt/iptables-safe`：

```bash
cd /tmp
wget https://github.com/dong005/iptable-safe/archive/refs/heads/main.zip
unzip -o main.zip
cd iptable-safe-main

chmod +x deploy-local.sh
./deploy-local.sh
```

也可指定端口：

```bash
IPTABLES_SAFE_PORT=8899 ./deploy-local.sh
```

部署完成后：

- 用户页：`http://<服务器IP>:8899/`
- 管理页：`http://<服务器IP>:8899/admin`
- 二进制/数据库：`/opt/iptables-safe/`
- systemd 服务名：`iptables-safe`

常用运维：

```bash
systemctl status iptables-safe
systemctl restart iptables-safe
journalctl -u iptables-safe -f
```

## 命令行工具

在安装目录执行（不要无参启动第二个实例）：

```bash
cd /opt/iptables-safe

# 修改用户/管理员密码（立即生效）
./iptables-safe passwd user '新密码'
./iptables-safe passwd admin '新密码'

# 保存监听端口到数据库（建议再用后台热切换，或 restart 服务）
./iptables-safe set-port 8899

# 查看帮助
./iptables-safe help
```

## 其他部署方式

### Git 克隆后部署

```bash
cd /tmp
git clone https://github.com/dong005/iptable-safe.git
cd iptable-safe
chmod +x deploy-local.sh
./deploy-local.sh
```

> 旧脚本 `auto-install.sh` / `git-deploy.sh` 仍保留，但推荐优先使用 `deploy-local.sh`（端口 8899、systemd、SRC 加固后的二进制）。

### 源码编译

需要 Go 1.22+（脚本会尝试自动下载）：

```bash
export GOPROXY=https://goproxy.cn,direct
go mod tidy
go build -o iptables-safe .
```

## 使用说明

### 用户访问

1. 打开 `http://<IP>:8899/`
2. 输入用户密码
3. 成功后，你的公网出口 IP 加入白名单 24 小时（可访问本机全部端口）

### 管理员访问

1. 打开 `http://<IP>:8899/admin`
2. 输入管理员密码
3. 可管理白名单、密码、服务端口、VOS 同步

### 防火墙模型

启动时大致规则：

- `INPUT` 默认 `DROP`
- 放行 `lo`、`ESTABLISHED,RELATED`
- 放行 TCP 认证端口（默认 8899）
- 白名单 IP：`-s <IP> -j ACCEPT`（入站全放行）
- `OUTPUT` 保持 `ACCEPT`（避免误杀出站）

## 安全建议

1. 立即修改默认用户/管理员密码
2. 使用强密码（建议 ≥ 8 位）
3. 不要把管理端口暴露给不必要的网段；可按需再收紧
4. 定期检查白名单
5. 生产环境建议前置 HTTPS（Nginx/Caddy 反代）
6. Token、密码不要提交到仓库或发到公开聊天

## 目录结构

```
iptables-safe/
├── main.go                 # 入口 / CLI
├── deploy-local.sh         # 推荐一键部署
├── serverctl/              # HTTP 热切换端口
├── handlers/               # HTTP + 会话鉴权
├── database/               # SQLite
├── iptables/               # 防火墙规则
├── models/
├── vos/                    # VOS 同步
├── templates/              # Web 页面
└── README.md
```

## API

### 公开

- `POST /api/login` — 用户认证并加白
- `POST /api/admin/login` — 管理员登录（写入会话 Cookie）
- `POST /api/admin/logout` — 管理员退出

### 需管理员会话

- `GET/POST /api/admin/whitelist`
- `DELETE /api/admin/whitelist/:id`
- `PUT /api/admin/password/user`
- `PUT /api/admin/password/admin`
- `GET/PUT /api/admin/port` — 查询/修改监听端口（热生效）
- `GET/PUT /api/admin/vos/config`
- `POST /api/admin/vos/sync`
- `GET /api/admin/vos/status`

## 故障排除

### 看端口与进程

```bash
ss -tlnp | grep 8899
systemctl status iptables-safe
```

### 看防火墙

```bash
iptables -L INPUT -n -v --line-numbers
```

### 改端口后打不开

确认防火墙已放行新端口，或在后台改端口（会同步改 iptables）。也可：

```bash
/opt/iptables-safe/iptables-safe set-port 8899
systemctl restart iptables-safe
```

### SSH 担心被锁

程序会保留 `ESTABLISHED,RELATED`。部署前建议确认当前 SSH 来源 IP，必要时先手动加白：

```bash
iptables -I INPUT 1 -s <你的IP> -j ACCEPT
```

## 备份

```bash
cp /opt/iptables-safe/iptables-safe.db /root/iptables-safe.db.backup
```

恢复后重启服务：

```bash
cp /root/iptables-safe.db.backup /opt/iptables-safe/iptables-safe.db
systemctl restart iptables-safe
```

## 技术栈

- Go + Gin
- SQLite（modernc.org/sqlite）
- bcrypt
- iptables
- systemd

## 许可证

MIT License
