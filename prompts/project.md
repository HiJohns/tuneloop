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
| `UPLOAD_MAX_SIZE` | 最大文件大小（MB） | `10` |

## 配置优先级

1. 新变量名优先：`POSTGRES_*`, `BEACONIAM_*`, `TUNELOOP_WWW_URL`
2. 旧变量名兼容：`DB_HOST`, `IAM_URL` (保留向后兼容)

## 12-Factor App 合规

所有配置均通过环境变量注入，代码中无硬编码配置。

## 任务完成后验证清单 (Post-Task Verification)

每个任务标记为 `status:ready` 前，必须完成以下验证：

### 通用验证
- [ ] 代码可通过 `go build` (Golang) 或 `npm run build` (JavaScript) 编译
- [ ] 如涉及 API 变更，使用 `curl` 或 Postman 测试端点返回预期结果
- [ ] 对于 Bug 修复，复现原始问题并验证已解决

### 认证/授权相关任务（关键）
- [ ] **完整登录流程测试**：从 IAM OAuth 登录到所有受保护页面访问
- [ ] 验证 token 解析：检查 `[DEBUG Callback] Setting cookies for tenant: <实际tenant_id>` 日志
- [ ] 验证 cookie 设置：浏览器 developer tools 中可见 `token` 和 `refresh_token`
- [ ] **JWT token 完整性**：确保前端能正确读取包含多个 `=` padding 字符的 token
- [ ] 测试 token 过期场景：登录后等待 token 过期，验证自动刷新或重定向
- [ ] 后端 middleware 日志应显示 tenant_id 已正确设置（无空值）

### 前端相关任务
- [ ] 运行构建命令（`npm run build`）无语法错误
- [ ] 在隐身模式/清空缓存后测试页面加载和功能
- [ ] 验证所有修改的文件中的类似代码模式已同步修复

### 文档要求
- [ ] 新增环境变量已记录在本文件的「环境变量规范」章节
- [ ] 接口变更已更新到 `docs/api.md` (如存在)
- [ ] 复杂功能添加简短说明到本文件的「功能说明」章节

**特别警告**：OAuth、token 处理、权限相关的任务，必须在真实登录场景下完整测试后才能标记完成。