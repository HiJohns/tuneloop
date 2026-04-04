# TuneLoop 系统重构与功能演进调查报告

## 1. 概述

本报告针对 Issue #199 中提出的"TuneLoop 系统重构与功能演进计划书"进行深度调研，分析当前系统架构与待实现功能之间的差距，并提出可行的实施路径。

### 1.1 目标与范围

计划书涵盖四大核心领域：
- **基础设施与鉴权**：统一 API 调用、登录态优化、UUID 校验
- **数据模型重构**：JSONB 非标属性、媒体矩阵、标签归一化
- **UI/UX 体验升级**：列表页扁平化、编辑模式转换、视觉优化
- **租赁安全闭环**：出库确认、归还审计、法律确权

### 1.2 核心原则

系统逻辑从"工业化标品"转向"高价值非标资产"管理模式，遵循：
- 一物一码
- 资产中心化
- 流程闭环化

---

## 2. 当前系统架构分析

### 2.1 前端 API 服务层 (`frontend-pc/src/services/api.js`)

#### 2.1.1 已实现功能

| 功能模块 | 状态 | 说明 |
|---------|------|------|
| JWT 自动注入 | ✅ 已实现 | 第 124-126 行，自动添加 `Authorization: Bearer` 头 |
| 401 全局拦截 | ✅ 已实现 | 第 135-145 行，处理 HTTP 401 状态码 |
| 40101 错误码处理 | ✅ 已实现 | 第 154-166 行，处理业务层 token 过期错误 |
| 滑动窗口续期 | ✅ 已实现 | 第 108-118 行，请求前检查并自动刷新 token |
| Token 刷新机制 | ✅ 已实现 | 第 80-103 行，refreshAccessToken 函数 |
| 基础 API 封装 | ✅ 已实现 | api.get/post/put/delete 方法 |
| 领域 API 模块 | ✅ 已实现 | instrumentsApi, ordersApi, sitesApi 等 |

#### 2.1.2 待增强功能

1. **微信 Webview 环境适配**：当前 redirectToIAM 函数未检测微信环境
2. **30 天长效登录**：当前 token 有效期未明确确认是否支持 30 天

### 2.2 后端数据模型 (`backend/models/models.go`)

#### 2.2.1 JSONB 字段使用情况

| 字段 | 表 | 类型 | 说明 |
|------|-----|------|------|
| Images | Instrument | jsonb | 已使用 |
| Specifications | Instrument | jsonb | 已使用 |
| Pricing | Instrument | jsonb | 已使用 |
| Images | MaintenanceTicket | jsonb | 已使用 |
| RepairPhotos | MaintenanceTicket | jsonb | 已使用 |
| CompletionPhotos | MaintenanceTicket | jsonb | 已使用 |
| RedirectURIs | Client | jsonb | 已使用 |

#### 2.2.2 待扩展字段

1. **metadata 字段**：用于存储制作师、产地、材质等非标属性（当前缺失）
2. **标签系统**：缺乏独立的标签表和归一化映射表

---

## 3. 阶段实施分析

### 3.1 第一阶段：地基稳固 (System Core)

#### Task 1.1: 重构 api.js 与拦截器逻辑

**现状评估**：api.js 已具备完善的基础架构，包括：
- 统一的请求拦截器
- JWT 自动注入
- 401/40101 错误处理
- 滑动窗口续期

**建议**：
- 评估当前 token 有效期配置
- 添加微信 Webview 环境检测
- 考虑将微信环境检测逻辑添加至 redirectToIAM

#### Task 1.2: 数据库 Schema 调整

**现状评估**：部分字段已使用 JSONB，但 metadata 字段缺失

**建议**：
- 为 Instrument 模型添加 metadata JSONB 字段
- 考虑创建独立的标签系统表（label_normalization）

#### Task 1.3: 全局 CSS 优化

**现状评估**：需评估现有 index.css 和组件样式

**建议**：
- 审查现有滚动条样式
- 评估是否引入方案三（悬浮式自动隐藏）

### 3.2 第二阶段：录入进化 (Asset Management)

#### Task 2.1: 全屏编辑页

**现状评估**：
- 当前 InstrumentEdit.jsx 使用 Modal 对话框
- 规格与价格联动计算逻辑待实现

**建议**：
- 开发全屏编辑路由页面
- 实现两行规格布局
- 添加价格自动倍率计算逻辑

#### Task 2.2: 上传锁逻辑

**现状评估**：需评估现有上传组件

**建议**：
- 确保图片/视频上传完成后再提交 API
- 添加上传状态管理

#### Task 2.3: 标签检定中心

**现状评估**：当前缺乏独立的标签管理模块

**建议**：
- 创建标签数据库表
- 实现归一化映射逻辑
- 开发后台标签检定界面

### 3.3 第三阶段：流程闭环 (Rental Workflow)

#### Task 3.1: 移动端出库确认

**现状评估**：需开发小程序端新页面

**建议**：
- 开发小程序"出库确认"页面
- 实现用户预览入库照片并勾选确认

#### Task 3.2: 比对定损界面

**现状评估**：需开发新的管理后台页面

**建议**：
- 开发"出库 vs 归还"双栏同屏比对 UI
- 集成电子签名组件

#### Task 3.3: PDF 生成服务

**现状评估**：后端当前无 PDF 生成能力

**建议**：
- 集成 PDF 生成库（如 gofpdf）
- 开发《归还鉴定报告》生成接口

---

## 4. 补充建议分析

### 4.1 资产 ID (QR Code)

**可行性**：高
- 后端已有唯一 UUID
- 可使用 UUID 生成二维码图片
- 前端已有图片展示组件

### 4.2 网点库存看板

**可行性**：中
- 需增加地图视图组件
- 需实现多网点库存聚合查询

### 4.3 缓存策略

**可行性**：中
- 需评估 Redis 集成成本
- 仪表盘统计数字需后端支持缓存

---

## 5. 实施依赖关系

```
Task 1.1 ──┬──> Task 1.2 ──> Task 2.1 ──> Task 3.1
           │              │
           │              └─> Task 2.2 ──> Task 3.2
           │                         │
           │                         └─> Task 3.3
           │
           └─> Task 1.3 ──> Task 2.3
```

---

## 6. 结论与建议

### 6.1 总体评估

- **基础设施**：基础功能已完成 80%，需微调
- **数据模型**：需扩展 metadata 字段和标签系统
- **UI/UX**：需较大改动，涉及多个页面
- **流程闭环**：需从零开发新功能

### 6.2 优先实施建议

1. **第一阶段优先**：API 微信适配 + metadata 字段 + CSS 优化
2. **第二阶段其次**：全屏编辑页 + 标签系统
3. **第三阶段最后**：小程序和 PDF 功能

---

## 附录

### A. 相关文件索引

| 文件 | 用途 |
|------|------|
| `frontend-pc/src/services/api.js` | 前端 API 服务层 |
| `backend/models/models.go` | 后端数据模型 |
| `backend/handlers/api.go` | 后端 API 处理器 |
| `frontend-pc/src/pages/admin/instrument/` | 乐器管理页面 |

### B. 数据库表清单

| 表名 | 说明 |
|------|------|
| instruments | 乐器主表 |
| categories | 分类表 |
| orders | 订单表 |
| leases | 租赁表 |
| maintenance_tickets | 维修工单表 |
| sites | 网点表 |

---

*Model: google/gemini-3-pro-preview*
