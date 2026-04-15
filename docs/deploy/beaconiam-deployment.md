# 服务器部署文档

## 1. 系统要求

### 1.1 硬件配置
- 服务器：阿里云 EC2 北京站
- 规格：ecs.g5.large（2核8GB）
- 操作系统：CentOS 6.8 64位

### 1.2 部署架构
```
                           ┌─────────────────────────────┐
                           │  NGINX (Port 80/443)  │
                           │                   │
                           │ wx.cadenzayueqi.com│→ 5556
                           │ www.cadenzayueqi.com→ 5557
                           │ iam.cadenzayueqi.com→ 5552
                           └─────────────────────────────┘
                   
        ┌─────────────────────────────────────────────────┐
        │              内部网络                     │
        ├─────────────┬─────────────┬─────────────────┤
        │ TuneLoop  │ TuneLoop  │  BeaconIAM    │
        │ Mobile  │ PC      │            │
        │ :5556   │ :5557   │  :5552     │
        └─────────┴─────────┴─────────────────┘
                      │
              ┌─────────┴─────────┐
              │  PostgreSQL     │
              │  :5432        │
              └───────────────┘
```

---

## 2. 环境准备

### 2.1 系统更新
```bash
# 更新系统包
sudo yum update -y

# 安装基础工具
sudo yum install -y wget curl git vim unzip
```

### 2.2 安装 Docker
```bash
# 检查 CentOS 版本
cat /etc/centos-release

# 安装 Docker (CentOS 6.x 需要指定版本)
# 添加 Docker 仓库
sudo tee /etc/yum.repos.d/docker.repo <<EOF
[dockerrepo]
name=Docker Repository
baseurl=https://yum.dockerproject.org/repo/main/centos/6
enabled=1
gpgcheck=1
gpgkey=https://yum.dockerproject.org/gpg
EOF

# 安装 Docker
sudo yum install -y docker-engine docker-compose

# 启动 Docker
sudo service docker start
sudo chkconfig docker on
```

### 2.3 配置 Docker 加速器
```bash
sudo mkdir -p /etc/docker
sudo tee /etc/docker/daemon.json <<EOF
{
  "registry-mirrors": [
    "https://docker.mirrors.ustc.edu.cn",
    "https://hub-mirror.c.163.com"
  ]
}
EOF

sudo service docker restart
```

### 2.4 防火墙配置
```bash
# 检查防火墙状态
sudo service iptables status

# 开放端口
sudo iptables -I INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5556 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5557 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5552 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5432 -j ACCEPT

# 保存规则
sudo service iptables save
```

---

## 3. 数据库部署

### 3.1 使用 Docker 部署 PostgreSQL
```bash
# 创建数据目录
sudo mkdir -p /data/postgres
sudo chmod 755 /data/postgres

# 启动 PostgreSQL
docker run -d \
  --name tuneloop-db \
  -e POSTGRES_PASSWORD=YourSecurePassword \
  -e POSTGRES_DB=tuneloop \
  -v /data/postgres:/var/lib/postgresql/data \
  -p 5432:5432 \
  postgres:15-alpine
```

### 3.2 验证数据库
```bash
# 测试连接
docker exec tuneloop-db psql -U postgres -c "SELECT version();"
```

---

## 4. 应用编译与部署

### 4.1 克隆项目
```bash
# 创建工作目录
mkdir -p ~/app
cd ~/app

# 克隆 TuneLoop
git clone https://github.com/HiJohns/tuneloop.git

# 克隆 BeaconIAM
git clone https://github.com/HiJohns/beaconiam.git
```

### 4.2 编译后端
```bash
cd ~/app/tuneloop/backend

# 编译 TuneLoop 后端
go build -o tuneloop-api .

# 或使用 Docker 编译
docker run --rm \
  -v $(pwd):/src \
  -w /src \
  golang:1.21 \
  go build -o tuneloop-api .
```

### 4.3 编译前端

#### 4.3.1 PC 前端
```bash
cd ~/app/tuneloop/frontend-pc

# 安装依赖
npm install

# 构建生产版本
npm run build

# 或使用 Docker
docker run --rm \
  -v $(pwd):/app \
  -w /app \
  node:18 \
  npm install && npm run build
```

#### 4.3.2 移动前端
```bash
cd ~/app/tuneloop/frontend-mobile

# 安装依赖
npm install

# 构建
npm run build

# 复制输出到目录
cp -r dist/* ../backend/uploads/mobile/
```

### 4.4 编译 BeaconIAM
```bash
cd ~/app/beaconiam

# 编译
go build -o lin-iam .

# 或使用 Docker
docker run --rm \
  -v $(pwd):/src \
  -w /src \
  golang:1.21 \
  go build -o lin-iam .
```

---

## 5. 服务启动

### 5.1 使用 Systemd 管理服务

#### 5.1.1 TuneLoop 后端服务
```bash
sudo tee /etc/systemd/system/tuneloop.service <<EOF
[Unit]
Description=TuneLoop Backend API
After=network.target postgresql.service

[Service]
Type=simple
User=coder
WorkingDirectory=/home/coder/app/tuneloop/backend
ExecStart=/home/coder/app/tuneloop/backend/tuneloop-api
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable tuneloop
sudo systemctl start tuneloop
```

#### 5.1.2 BeaconIAM 服务
```bash
sudo tee /etc/systemd/system/beaconiam.service <<EOF
[Unit]
Description=Beacon IAM Service
After=network.target

[Service]
Type=simple
User=coder
WorkingDirectory=/home/coder/app/beaconiam
ExecStart=/home/coder/app/beaconiam/lin-iam
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable beaconiam
sudo systemctl start beaconiam
```

### 5.2 验证服务状态
```bash
# 检查 TuneLoop
curl http://localhost:5557/api/health

# 检查 BeaconIAM
curl http://localhost:5552/health

# 检查端口
netstat -tlnp | grep -E '(5552|5556|5557|5432)'
```

---

## 6. NGINX 配置

### 6.1 安装 NGINX
```bash
# 安装 EPEL 仓库
sudo yum install -y epel-release

# 安装 NGINX
sudo yum install -y nginx
```

### 6.2 配置反向代理
```bash
sudo tee /etc/nginx/conf.d/tuneloop.conf <<'EOF'
upstream tuneloop-mobile {
    server 127.0.0.1:5556;
}

upstream tuneloop-pc {
    server 127.0.0.1:5557;
}

upstream beaconiam {
    server 127.0.0.1:5552;
}

server {
    listen 80;
    server_name wx.cadenzayueqi.com;

    location / {
        proxy_pass http://tuneloop-mobile;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

server {
    listen 80;
    server_name www.cadenzayueqi.com;

    location / {
        proxy_pass http://tuneloop-pc;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
    
    location /api/ {
        proxy_pass http://tuneloop-pc;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}

server {
    listen 80;
    server_name iam.cadenzayueqi.com;

    location / {
        proxy_pass http://beaconiam;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
EOF
```

### 6.3 启动 NGINX
```bash
sudo systemctl enable nginx
sudo systemctl start nginx

# 测试配置
sudo nginx -t
```

### 6.4 配置域名解析
```bash
# 在阿里云控制台添加域���解��记录
# A 记录: wx.cadenzayueqi.com → 服务器公网 IP
# A 记录: www.cadenzayueqi.com → 服务器公网 IP
# A 记录: iam.cadenzayueqi.com → 服务器公网 IP
```

---

## 7. SSL 配置（可选）

### 7.1 使用 Let's Encrypt
```bash
# 安装 Certbot
sudo yum install -y certbot python2-certbot-nginx

# 获取证书
sudo certbot --nginx -d wx.cadenzayueqi.com -d www.cadenzayueqi.com -d iam.cadenzayueqi.com

# 自动续期
sudo certbot renew --dry-run

# 添加计划任务
crontab -e
0 0,12 * * * certbot renew --quiet --deploy-hook "systemctl reload nginx"
```

---

## 8. 启动与验证

### 8.1 完整启动顺序
```bash
# 1. 启动数据库
docker start tuneloop-db

# 2. 启动后端服务
sudo systemctl start tuneloop
sudo systemctl start beaconiam

# 3. 启动 NGINX
sudo systemctl start nginx
```

### 8.2 验证检查清单
```bash
# 检查所有服务状态
sudo systemctl status docker
sudo systemctl status tuneloop
sudo systemctl status beaconiam
sudo systemctl status nginx

# 开放端口检查
netstat -tlnp | grep -E ':(80|443|5552|5556|5557|5432)'

# 外部访问测试
curl http://www.cadenzayueqi.com/api/health
curl http://iam.cadenzayueqi.com/health
```

---

## 9. 维护指南

### 9.1 日志查看
```bash
# NGINX 日志
sudo tail -f /var/log/nginx/access.log
sudo tail -f /var/log/nginx/error.log

# 应用日志
journalctl -u tuneloop -f
journalctl -u beaconiam -f
```

### 9.2 数据备份
```bash
# 数据库备份
docker exec tuneloop-db pg_dump -U postgres tuneloop > backup_$(date +%Y%m%d).sql

# 文件备份
tar -czvf backends_$(date +%Y%m%d).tar.gz ~/app/
```

### 9.3 回滚方案
```bash
# 停止服务
sudo systemctl stop tuneloop beaconiam nginx

# 恢复数据库
docker exec -i tuneloop-db psql -U postgres tuneloop < backup_YYYYMMDD.sql

# 重启服务
sudo systemctl start tuneloop beaconiam nginx
```

---

## 10. 环境变量参考

### 10.1 TuneLoop 后端环境变量
```
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
DB_USER=postgres
DB_PASSWORD=YourSecurePassword
DB_NAME=tuneloop
BEACONIAM_URL=http://localhost:5552
APP_ENV=production
```

### 10.2 BeaconIAM 环境变量
```
BEACONIAM_PORT=5552
DB_TYPE=postgres
DB_HOST=localhost
DB_PORT=5432
POSTGRES_USER=postgres
POSTGRES_PASSWORD=YourSecurePassword
JWT_SECRET=YourJWTSecretKey
```

---

*最后更新: 2026-04-15*