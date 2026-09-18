# Server Deployment Guide

## 1. System Requirements

### 1.1 Hardware
- Server: Alibaba Cloud EC2 Beijing Region
- Instance: ecs.g5.large (2 vCPU, 8GB RAM)
- OS: CentOS 6.8 64-bit

### 1.2 Deployment Architecture
```
                           ┌─────────────────────────────┐
                           │  NGINX (Port 80/443)  │
                           │                   │
                           │ wx.cadenzayueqi.com│→ 5556
                           │ www.cadenzayueqi.com→ 5557
                           │ iam.cadenzayueqi.com→ 5552
                           └─────────────────────────────┘
                   
        ┌─────────────────────────────────────────────────┐
        │              Internal Network              │
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

## 2. Environment Setup

### 2.1 System Update
```bash
# Update system packages
sudo yum update -y

# Install basic tools
sudo yum install -y wget curl git vim unzip
```

### 2.2 Install Docker
```bash
# Check CentOS version
cat /etc/centos-release

# Install Docker (CentOS 6.x requires specific version)
# Add Docker repository
sudo tee /etc/yum.repos.d/docker.repo <<EOF
[dockerrepo]
name=Docker Repository
baseurl=https://yum.dockerproject.org/repo/main/centos/6
enabled=1
gpgcheck=1
gpgkey=https://yum.dockerproject.org/gpg
EOF

# Install Docker
sudo yum install -y docker-engine docker-compose

# Start Docker
sudo service docker start
sudo chkconfig docker on
```

### 2.3 Configure Docker Accelerator
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

### 2.4 Firewall Configuration
```bash
# Check firewall status
sudo service iptables status

# Open ports
sudo iptables -I INPUT -p tcp --dport 80 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 443 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5556 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5557 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5552 -j ACCEPT
sudo iptables -I INPUT -p tcp --dport 5432 -j ACCEPT

# Save rules
sudo service iptables save
```

---

## 3. Database Deployment

### 3.1 Deploy PostgreSQL Using Docker
```bash
# Create data directory
sudo mkdir -p /data/postgres
sudo chmod 755 /data/postgres

# Start PostgreSQL
docker run -d \
  --name tuneloop-db \
  -e POSTGRES_PASSWORD=YourSecurePassword \
  -e POSTGRES_DB=tuneloop \
  -v /data/postgres:/var/lib/postgresql/data \
  -p 5432:5432 \
  postgres:15-alpine
```

### 3.2 Verify Database
```bash
# Test connection
docker exec tuneloop-db psql -U postgres -c "SELECT version();"
```

---

## 4. Application Build and Deployment

### 4.1 Clone Projects
```bash
# Create working directory
mkdir -p ~/app
cd ~/app

# Clone TuneLoop
git clone https://github.com/HiJohns/tuneloop.git

# Clone BeaconIAM
git clone https://github.com/HiJohns/beaconiam.git
```

### 4.2 Build Backend
```bash
cd ~/app/tuneloop/backend

# Build TuneLoop backend
go build -o tuneloop-api .

# Or use Docker to build
docker run --rm \
  -v $(pwd):/src \
  -w /src \
  golang:1.21 \
  go build -o tuneloop-api .
```

### 4.3 Build Frontend

#### 4.3.1 PC Frontend
```bash
cd ~/app/tuneloop/frontend-pc

# Install dependencies
npm install

# Build production version
npm run build

# Or use Docker
docker run --rm \
  -v $(pwd):/app \
  -w /app \
  node:18 \
  npm install && npm run build
```

#### 4.3.2 Mobile Frontend
```bash
cd ~/app/tuneloop/frontend-mobile

# Install dependencies
npm install

# Build
npm run build

# Copy output to directory
cp -r dist/* ../backend/uploads/mobile/
```

### 4.4 Build BeaconIAM
```bash
cd ~/app/beaconiam

# Build
go build -o lin-iam .

# Or use Docker
docker run --rm \
  -v $(pwd):/src \
  -w /src \
  golang:1.21 \
  go build -o lin-iam .
```

---

## 5. Service Startup

### 5.1 Manage Services Using Systemd

#### 5.1.1 TuneLoop Backend Service
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

#### 5.1.2 BeaconIAM Service
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

### 5.2 Verify Service Status
```bash
# Check TuneLoop
curl http://localhost:5557/api/health

# Check BeaconIAM
curl http://localhost:5552/health

# Check ports
netstat -tlnp | grep -E '(5552|5556|5557|5432)'
```

---

## 6. NGINX Configuration

### 6.1 Install NGINX
```bash
# Install EPEL repository
sudo yum install -y epel-release

# Install NGINX
sudo yum install -y nginx
```

### 6.2 Configure Reverse Proxy
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

### 6.3 Start NGINX
```bash
sudo systemctl enable nginx
sudo systemctl start nginx

# Test configuration
sudo nginx -t
```

### 6.4 Configure DNS Records
```bash
# Add DNS A records in Alibaba Cloud console:
# A record: wx.cadenzayueqi.com → Server Public IP
# A record: www.cadenzayueqi.com → Server Public IP
# A record: iam.cadenzayueqi.com → Server Public IP
```

---

## 7. SSL Configuration (Optional)

### 7.1 Using Let's Encrypt
```bash
# Install Certbot
sudo yum install -y certbot python2-certbot-nginx

# Get certificate
sudo certbot --nginx -d wx.cadenzayueqi.com -d www.cadenzayueqi.com -d iam.cadenzayueqi.com

# Auto-renew test
sudo certbot renew --dry-run

# Add cron job
crontab -e
0 0,12 * * * certbot renew --quiet --deploy-hook "systemctl reload nginx"
```

---

## 8. Startup and Verification

### 8.1 Complete Startup Sequence
```bash
# 1. Start database
docker start tuneloop-db

# 2. Start backend services
sudo systemctl start tuneloop
sudo systemctl start beaconiam

# 3. Start NGINX
sudo systemctl start nginx
```

### 8.2 Verification Checklist
```bash
# Check all service status
sudo systemctl status docker
sudo systemctl status tuneloop
sudo systemctl status beaconiam
sudo systemctl status nginx

# Open ports check
netstat -tlnp | grep -E ':(80|443|5552|5556|5557|5432)'

# External access test
curl http://www.cadenzayueqi.com/api/health
curl http://iam.cadenzayueqi.com/health
```

---

## 9. Maintenance Guide

### 9.1 Log Viewing
```bash
# NGINX logs
sudo tail -f /var/log/nginx/access.log
sudo tail -f /var/log/nginx/error.log

# Application logs
journalctl -u tuneloop -f
journalctl -u beaconiam -f
```

### 9.2 Data Backup
```bash
# Database backup
docker exec tuneloop-db pg_dump -U postgres tuneloop > backup_$(date +%Y%m%d).sql

# File backup
tar -czvf backends_$(date +%Y%m%d).tar.gz ~/app/
```

### 9.3 Rollback Procedure
```bash
# Stop services
sudo systemctl stop tuneloop beaconiam nginx

# Restore database
docker exec -i tuneloop-db psql -U postgres tuneloop < backup_YYYYMMDD.sql

# Restart services
sudo systemctl start tuneloop beaconiam nginx
```

---

## 10. Environment Variables Reference

### 10.1 TuneLoop Backend Environment Variables
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

### 10.2 BeaconIAM Environment Variables
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

*Last Updated: 2026-04-15*