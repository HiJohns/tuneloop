---
id: P-04
domain: profile
flow: 身份证照片全流程管理
steps:
  # ── 上传入口 ──
  - seq: 1
    action: H5 注册时上传身份证
    frontend:
      - platform: [h5]
        page: /register
        role: [guest]
        gate: ""
        reach: "注册页 → 身份证区域（可选）"
        controls: [正面上传区域, 反面上传区域, 预览及删除按钮]
        displays: [已选图片预览]
        ops:
          - {type: api, method: POST, path: /user/id-photo}
    api: {method: POST, path: /user/id-photo, params: [file, side]}
  - seq: 2
    action: H5 引导页上传身份证
    frontend:
      - platform: [h5]
        page: /onboarding
        role: [customer]
        gate: "onboarding_completed=false"
        reach: "登录后 → /onboarding → 身份证区域（可选）"
        controls: [正面上传区域, 反面上传区域, 预览及删除按钮]
        displays: [已选图片预览]
        ops:
          - {type: api, method: POST, path: /user/id-photo}
    api: {method: POST, path: /user/id-photo, params: [file, side]}
  - seq: 3
    action: weapp 注册时上传身份证
    frontend:
      - platform: [weapp]
        page: /pages-weapp/profile-complete/index
        role: [guest]
        gate: ""
        reach: "注册页 → 身份证区域（可选）"
        controls: [正面上传区域, 反面上传区域, 预览及删除按钮]
        displays: [已选图片预览]
        ops:
          - {type: api, method: POST, path: /user/id-photo}
    api: {method: POST, path: /user/id-photo, params: [file, side]}

  # ── 查看/替换入口（顾客自助） ──
  - seq: 4
    action: 移动端编辑资料查看和替换身份证
    frontend:
      - platform: [weapp, h5]
        page: /profile/edit
        role: [customer]
        gate: ""
        reach: "个人中心 → 编辑资料 → 身份证区域"
        controls: [当前正面预览, 当前反面预览, 替换按钮, 删除按钮]
        displays: [已上传的正面图, 已上传的反面图, 未上传时的占位]
        ops:
          - {type: api, method: GET, path: /user/id-photos}
          - {type: api, method: POST, path: /user/id-photo}
          - {type: api, method: DELETE, path: /user/id-photo}
    api:
      - {method: GET, path: /user/id-photos}
      - {method: POST, path: /user/id-photo, params: [file, side]}
      - {method: DELETE, path: /user/id-photo, params: [side]}

  # ── 查看/替换入口（PC 管理员） ──
  - seq: 5
    action: PC 用户管理页查看和替换身份证
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [namespace_admin]
        gate: "sys_perm bit[16]"
        reach: "系统管理 → 用户管理 → 详情 Modal → 身份证区域"
        controls: [正面预览, 反面预览, 替换按钮, 删除按钮]
        displays: [已上传的正面图, 已上传的反面图, 未上传时的占位]
        ops:
          - {type: api, method: GET, path: /admin/user-management/:id}
          - {type: api, method: POST, path: /admin/user-management/:id/id-photo}
          - {type: api, method: DELETE, path: /admin/user-management/:id/id-photo}
    api:
      - {method: GET, path: /admin/user-management/:id, params: []}
      - {method: POST, path: /admin/user-management/:id/id-photo, params: [file, side]}
      - {method: DELETE, path: /admin/user-management/:id/id-photo, params: [side]}

  # ── 业务场景查看入口 ──
  - seq: 6
    action: 发货/收货时查看用户身份证
    frontend:
      - platform: [pc]
        page: /orders/:id
        role: [site_admin, site_member]
        gate: ""
        reach: "订单详情 → 出库/入库环节 → 查看用户身份证"
        controls: [正面缩略图, 反面缩略图, 点击放大查看]
        displays: [身份证正面图, 身份证反面图]
        ops:
          - {type: api, method: GET, path: /user/:userId/id-photos}
    api: {method: GET, path: /user/:userId/id-photos, params: [userId]}

  # ── 移动端业务场景 ──
  - seq: 7
    action: 移动端员工发货/收货时查看身份证
    frontend:
      - platform: [weapp, h5]
        page: /repair-request
        role: [site_admin, site_member, repair_technician]
        gate: ""
        reach: "报修/租赁详情 → 员工操作面板 → 查看用户身份证"
        controls: [正面缩略图, 反面缩略图, 点击放大查看]
        displays: [身份证正面图, 身份证反面图]
        ops:
          - {type: api, method: GET, path: /user/:userId/id-photos}
    api: {method: GET, path: /user/:userId/id-photos, params: [userId]}
---

# P-04 身份证照片全流程管理

## 前置条件
- 系统已运行，media storage 可用
- `users` 表需添加 `id_photo_front` 和 `id_photo_back` 列

## 流程概览

```
上传端                              消费端
─────                              ─────
注册页 (H5/weapp)                   编辑资料 (H5/weapp)
引导页 (H5 onboarding)          ←   编辑资料 (PC 管理员)
编辑资料 (H5/weapp)                 发货/收货 (PC 员工)
用户管理 (PC 管理员)                 报修详情 (移动端员工)
```

## 数据模型变更

### users 表新增列
```sql
ALTER TABLE users ADD COLUMN id_photo_front VARCHAR(500);
ALTER TABLE users ADD COLUMN id_photo_back  VARCHAR(500);
```

### API 端点

| 方法 | 路径 | 用途 | 权限 |
|------|------|------|------|
| POST | `/api/user/id-photo` | 上传（FormData: file + side=front\|back） | 已登录用户 |
| GET | `/api/user/id-photos` | 获取自己的正反面 URL | 已登录用户 |
| DELETE | `/api/user/id-photo?side=front\|back` | 删除自己的一面 | 已登录用户 |
| GET | `/api/user/:userId/id-photos` | 员工查看用户的身份证 | STAFF + tenant 隔离 |
| POST | `/api/admin/user-management/:id/id-photo` | 管理员替用户上传 | namespace_admin |
| DELETE | `/api/admin/user-management/:id/id-photo?side=front\|back` | 管理员替用户删除 | namespace_admin |

### 存储路径
- 文件存储: `id_photos/{userID}_{timestamp}.{ext}`（media storage）
- 列值: storage_key（如 `id_photos/abc123_1691234567890.jpg`）
- 前端访问: 通过 `MediaStorage.GetURL()` 获取签名/公开 URL

## 关键规则

### 上传规则
- 仅接受 JPEG、PNG、WebP
- 单张 ≤ 5MB
- 正反面独立上传，不强制同时具备
- 上传覆盖：新上传自动替换对应面（覆盖旧文件 + 更新列值）
- 上传后立即持久化 URL 到 `users.id_photo_front` / `users.id_photo_back`

### 查看规则
- 顾客只能查看/替换/删除自己的身份证
- 员工（site_admin/site_member/repair_technician）可查看所辖用户身份证（tenant 隔离）
- PC 管理员（namespace_admin）可查看和替换任意用户身份证
- 无身份证时显示「未上传」占位，不显示空白或错误

### 修复项（Onboarding 现有 Bug）
- 当前 `POST /api/user/id-photo` 上传文件但不保存 URL 到 DB
- 当前 `PUT /user/onboarding` 不接受 id_photo 字段
- 修复方案：上传接口内联保存（上传成功后直接写入对应的 DB 列）

## 全局审计：所有需修改的页面

| # | 平台 | 页面 | 角色 | 动作 | 当前状态 |
|---|------|------|------|------|:---:|
| 1 | H5 | `/register` | guest | 注册时上传 | **新建**（P-03） |
| 2 | H5 | `/onboarding` | customer | 登录后上传 | 上传可用但**不持久化** |
| 3 | weapp | `/pages-weapp/profile-complete/index` | guest | 注册时上传 | **缺失** |
| 4 | H5 | `/profile/edit` | customer | 查看+替换+删除 | **缺失**（页尚不存在） |
| 5 | weapp | `/pages-weapp/profile/edit/index` | customer | 查看+替换+删除 | **缺失** |
| 6 | PC | `/system/user-management` | namespace_admin | 查看+替换+删除 | **缺失**（P-02 已建页但无此区域） |
| 7 | PC | `/orders/:id` | site_admin/member | 发货/收货时查看 | **缺失** |
| 8 | H5 | `/repair-request` | staff | 操作时查看 | **缺失** |
| 9 | weapp | `/repair-request` | staff | 操作时查看 | **缺失** |

## 验收
- Go test: POST id-photo → DB 列值非空，GET id-photos → 返回 URL，DELETE → 列值清空
- checklist-verify.py: P-04 displays 与 GET /users/me、GET /user/:userId/id-photos 响应交叉验证
- 端到端: 注册上传 → 编辑资料可见 → 替换 → 编辑资料更新 → 删除 → 编辑资料显示未上传

## 关联 Issue
- 后端数据层 (DB + API)：[tuneloop#1598](https://github.com/HiJohns/tuneloop/issues/1598)
- 前端全平台接入：[tuneloop#1599](https://github.com/HiJohns/tuneloop/issues/1599)
- H5 注册页（含身份证上传入口）：[tuneloop#1597](https://github.com/HiJohns/tuneloop/issues/1597)

## 已知缺口
- 未定义身份证过期/重新认证机制（首次上传即永久有效）
- 未定义 OCR 自动识别（身份信息全靠人工查看）
- 未定义存储清理策略（旧文件被替换后残留于 media storage）

---

*Last updated: 2026-08-09*
