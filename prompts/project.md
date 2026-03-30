# 环境变量规范 / Environment Variables Specification

## 命名约定

本项目遵循以下环境变量命名约定：

### 数据库配置 (PostgreSQL)
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `POSTGRES_HOST` | 数据库主机地址 | `localhost` |
| `POSTGRES_PORT` | 数据库端口 | `5432` |
| `POSTGRES_USER` | 数据库用户名 | `tuneloop` |
| `POSTGRES_PASSWORD` | 数据库密码 | - |
| `TUNELOOP_DB` | 数据库名称 | `tuneloop` |
| `DB_SSLMODE` | SSL 模式 | `disable` |

### Beacon IAM 配置
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `BEACONIAM_EXTERNAL_URL` | 前端 IAM 跳转地址 | - |
| `BEACONIAM_INTERNAL_URL` | 内部 IAM 调用地址 | - |
| `IAM_CLIENT_ID` | IAM 客户端 ID | - |
| `IAM_CLIENT_SECRET` | IAM 客户端密钥 | - |

### 服务地址配置
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `TUNELOOP_WWW_URL` | PC Web 服务地址 | `http://localhost:5554` |
| `TUNELOOP_WX_URL` | 微信小程序服务地址 | `http://localhost:5553` |

### 文件上传配置
| 变量名 | 说明 | 默认值 |
|--------|------|--------|
| `UPLOAD_BASE_URL` | 上传文件的访问基准URL | `http://localhost:5554/uploads` |
| `UPLOAD_MAX_SIZE` | 最大文件大小（MB） | `10` |

## 配置优先级

1. 新变量名优先：`POSTGRES_*`, `BEACONIAM_*`, `TUNELOOP_WWW_URL`
2. 旧变量名兼容：`DB_HOST`, `IAM_URL` (保留向后兼容)

## 12-Factor App 合规

所有配置均通过环境变量注入，代码中无硬编码配置。