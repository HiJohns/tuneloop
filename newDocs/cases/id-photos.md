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
        controls: [正面上传区域, 反面上传区域, 第三证件上传区域, 证件类型选择（学生证/教师证/工作证/其他）]
        displays: [已选图片预览]
        ops:
          - {type: api, method: POST, path: /auth/registration-sessions/:id/id-photo}
    api:
      - {method: POST, path: /auth/registration-sessions/:id/id-photo, params: [file, side]}
      - {method: POST, path: /auth/registration-sessions, params: [id_photo_other_type]}
    rule: "#1807: 布局正反面一行 + 第三证件单独一行（含证件类型选择）；证件照延迟上传到注册 session（会话端点），注册完成回调时转移至用户记录；证件类型随 session form_data 一并转移（users.id_photo_other_type）"

  # ── 查看/替换入口（顾客自助） ──
  - seq: 4
    action: 移动端编辑资料查看和替换身份证
    frontend:
      - platform: [weapp, h5]
        page: /profile/edit
        role: [customer]
        gate: ""
        reach: "个人中心 → 编辑资料 → 身份证区域"
        controls: [当前正面预览, 当前反面预览, 第三证件预览, 证件类型选择, 替换按钮, 删除按钮]
        displays: [已上传的正面图, 已上传的反面图, 未上传时的占位]
        ops:
          - {type: api, method: GET, path: /user/id-photos}
          - {type: api, method: POST, path: /user/id-photo}
          - {type: api, method: DELETE, path: /user/id-photo}
    api:
      - {method: GET, path: /user/id-photos}
      - {method: POST, path: /user/id-photo, params: [file, side]}
      - {method: DELETE, path: /user/id-photo, params: [side]}
    rule: "#1807: 布局正反面一行（各 ~48%）+ 第三证件单独一行（含证件类型选择 users.id_photo_other_type）；实名认证区块隐藏「姓名/身份证号输入框」——实名信息由员工在审核流程根据身份证照核对填写；「发起人脸识别」入口按 id_verify_status 状态显示（none 引导上传 / uploaded·rejected 进入人脸识别页，见 #1811）"

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

  # ── 核身状态消费场景（#1787 补充设计） ──
  - seq: 8
    action: 顾客下单确认页检查核身状态（警告 + 跳转，不阻断提交）
    frontend:
      - platform: [weapp, h5]
        page: /checkout
        role: [customer]
        gate: ""
        reach: "购物车 → 确认订单 → 核身状态区"
        controls: [警告条, 跳转编辑资料按钮]
        displays: [未上传提示（去上传身份证）, 已上传未提交自拍提示（去编辑资料）]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
    rule: "仅警告不阻断：未认证仍可提交订单；状态派生：none/uploaded/verified（#1807：跳转目标为编辑资料提交自拍，非人脸核身）"
  - seq: 9
    action: 顾客订单详情页检查核身状态（警告 + 跳转）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [customer]
        gate: "订单未发货"
        reach: "我的订单 → 订单详情 → 核身状态区"
        controls: [警告条, 跳转编辑资料按钮]
        displays: [未上传提示, 已上传未提交自拍提示]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
    rule: "订单已发货后不再展示警告（履约已开始）；#1807：跳转目标为编辑资料提交自拍，非人脸核身"
  - seq: 10
    action: 员工待发货订单核身拦截（前端置灰 + 后端强制）
    frontend:
      - platform: [weapp, h5]
        page: /staff/shipping, /orders/:id
        role: [site_admin, site_member]
        gate: "order.status=paid"
        reach: "待发货列表 → 订单详情 → 发货按钮"
        controls: [发货按钮（未核身置灰）, 未核身角标]
        displays: [未核身标记, 置灰发货按钮]
        ops:
          - {type: api, method: GET, path: /orders/:id, params: []}
    api:
      - {method: GET, path: /orders/:id, params: []}
      - {method: PUT, path: /warehouse/orders/:id/shipping, params: [], gate: "id_verify_status=verified，无豁免"}
    rule: "后端强制校验：非 verified 拒绝发货（40002，message=user id verification required）；前端置灰仅为 UX"
  - seq: 11
    action: 核身超时订单自动取消退款（H8，用户决策 2026-08-29：A 超时自动取消，3 天）
    actor: 系统定时任务
    gate: "order.status ∈ {paid, pending_shipment} 且 id_verify_status != verified 且 支付完成 > FACE_VERIFY_TIMEOUT_HOURS(默认72)"
    steps:
      - "扫描满足条件订单"
      - "订单 status → cancelled"
      - "按实付金额发起微信退款（order_refund_records，reason=核身超时自动取消退款）"
      - "乐器 stock_status → available"
      - "⚠️ 必须生成审计日志（强制，禁止静默取消）：order_logs Event=核身超时自动取消 + audit_logs action=auto_cancel_verify_timeout（details 含 trigger/timeout_hours/退款单号）+ 顾客站内通知"
    rule: "幂等：已取消/已退款订单跳过；任何自动取消必须可追溯"
  - seq: 12
    action: 平台员工审核队列 — 根据身份证照填写实名信息（5 项）并通过/驳回（#1807 扩充）
    frontend:
      - platform: [pc]
        page: /face-review
        role: [platform_staff, system_admin]
        gate: "SysPermUserUpdate"
        reach: "平台管理 → 实名审核队列 → 待审核批次"
        controls: [证件照三张预览, 自拍图/视频预览, 真实姓名输入框, 身份证号输入框, 有效期输入框, 签发机关输入框, 住址输入框, 通过按钮, 驳回按钮（必填原因）]
        displays: [用户姓名, 提交时间]
        ops:
          - {type: api, method: GET, path: /admin/face-review/queue}
          - {type: api, method: POST, path: /admin/face-review/:batchId, params: [action, real_name, id_card_no, id_card_expire, id_card_authority, id_card_address, reason]}
    api:
      - {method: GET, path: /admin/face-review/queue, params: []}
      - {method: POST, path: /admin/face-review/:batchId, params: [action], gate: "approve 必填 real_name+id_card_no+id_card_expire+id_card_authority+id_card_address（宽松校验：非空即可）；reject 必填 reason"}
    rule: "#1807: 实名信息（真实姓名/身份证号/有效期/签发机关/住址 5 项）由员工根据身份证照核对填写，approve 时一并落库（face_verified=true, method=manual, id_card_expire 支持 YYYY-MM-DD 或「长期」）；字段边界：审核队列不返回身份证号明文，仅证件照供核对；驳回附原因顾客重新采集"
---

# P-04 身份证照片全流程管理

## 前置条件
- 系统已运行，media storage 可用
- `users` 表需添加 `id_photo_front`、`id_photo_back`、`id_photo_other` 列
- 腾讯云慧眼 FaceID 已配置（可选，未配置时降级为纯照片模式）

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
ALTER TABLE users ADD COLUMN id_photo_front  VARCHAR(500);
ALTER TABLE users ADD COLUMN id_photo_back   VARCHAR(500);
ALTER TABLE users ADD COLUMN id_photo_other  VARCHAR(500);  -- #1787 第三张照片
ALTER TABLE users ADD COLUMN id_photo_other_type VARCHAR(50); -- #1807 第三证件类型（student/teacher/work/other）
ALTER TABLE users ADD COLUMN real_name       VARCHAR(100);  -- #1787 实名姓名（#1807 起由员工审核填写）
ALTER TABLE users ADD COLUMN id_card_no      VARCHAR(20);   -- #1787 身份证号（明文，#1807 起由员工审核填写）
ALTER TABLE users ADD COLUMN id_card_expire  VARCHAR(20);   -- #1807 身份证有效期（YYYY-MM-DD 或「长期」，员工审核填写）
ALTER TABLE users ADD COLUMN id_card_authority VARCHAR(100); -- #1807 签发机关（员工审核填写）
ALTER TABLE users ADD COLUMN id_card_address VARCHAR(200);  -- #1807 证件住址（员工审核填写）
ALTER TABLE users ADD COLUMN face_verified   BOOLEAN DEFAULT FALSE;  -- #1787 人脸识别是否通过
ALTER TABLE users ADD COLUMN face_verified_at TIMESTAMPTZ;  -- #1787 人脸识别通过时间
```

### API 端点

| 方法 | 路径 | 用途 | 权限 |
|------|------|------|------|
| POST | `/api/user/id-photo` | 上传（FormData: file + side=front\|back\|other） | 已登录用户 |
| GET | `/api/user/id-photos` | 获取自己的三张照片 URL | 已登录用户 |
| DELETE | `/api/user/id-photo?side=front\|back\|other` | 删除自己的一面 | 已登录用户 |
| GET | `/api/user/:userId/id-photos` | 员工查看用户的身份证 | STAFF + tenant 隔离 |
| POST | `/api/admin/user-management/:id/id-photo` | 管理员替用户上传 | namespace_admin |
| DELETE | `/api/admin/user-management/:id/id-photo?side=front\|back\|other` | 管理员替用户删除 | namespace_admin |
| POST | `/api/auth/registration-sessions/:id/id-photo` | 注册阶段上传（会话级匿名端点） | 无需认证 |
| POST | `/api/user/face-verify/token` | 获取腾讯云慧眼核身 Token | 已登录用户 |
| POST | `/api/user/face-verify/result` | 轮询核身结果 | 已登录用户 |

### 存储路径
- 文件存储: `id_photos/{userID}_{timestamp}.{ext}`（media storage）
- 列值: storage_key（如 `id_photos/abc123_1691234567890.jpg`）
- 前端访问: 通过 `MediaStorage.GetURL()` 获取签名/公开 URL

## 关键规则

### 上传规则
- 仅接受 JPEG、PNG、WebP
- 单张 ≤ 5MB
- 三张独立上传（front/back/other），不强制同时具备
- 上传覆盖：新上传自动替换对应面（覆盖旧文件 + 更新列值）
- 上传后立即持久化 URL 到对应 DB 列

### 查看规则
- 顾客只能查看/替换/删除自己的身份证
- 员工（site_admin/site_member/repair_technician）可查看所辖用户身份证（tenant 隔离）
- PC 管理员（namespace_admin）可查看和替换任意用户身份证
- 无身份证时显示「未上传」占位，不显示空白或错误

### 人脸识别核身规则（#1787，#1807 修订）

> #1807 调整：**实名信息（真实姓名/身份证号）不由顾客输入**，改为员工在人工审核流程根据身份证照核对填写。#1811 调整：编辑资料页按 `id_verify_status` 细分文案，「发起人脸识别」链接始终可见（按状态显示不同引导）。

- **实名信息填写方**：平台员工（人工审核时填写，见人工审核流程）——顾客端不再提供 real_name/id_card_no 输入框
- **通过后**：`users.face_verified = true, face_verified_at = now(), real_name, id_card_no`（员工填写值），编辑资料页显示「已实名」绿标（姓名 + 掩码身份证）
- **身份证号存储**：明文（腾讯云 API 需原文）；GET /users/me 返回掩码 `110***********1234`
- **人脸识别入口**（#1811）：编辑资料页实名认证模块按状态显示文案与链接：`none`→「请先在上方上传身份证照片」（无链接）；`uploaded`/`rejected`→「发起人脸识别」链接进入人脸识别页；`pending_review`→黄色审核中提示；`verified`→绿色已认证

### 配置降级与人工审核兜底（#1787 补充设计）

> **#1807 阶段1 决策（2026-08-31）**：自动核身采用 **腾讯云 E证通**（跳转官方「e证通」小程序核身，公安权威库比对）。E证通商户 ID `00EI2608281323113956` + CA 已通过；服务端调用需 API 密钥（CAM `TENCENTCLOUD_SECRET_ID/KEY`，同一密钥复用 OCR）。启用开关：`FACE_VERIFY_PROVIDER=eid`（或 `faceid` 慧眼站内插件模式），**默认 `manual`（人工审核）**。身份信息来源：**OCR 预填 + 顾客确认**（复用 idcard_ocr_real.go，需开通腾讯云「文字识别」产品）。E证通 client 手写（腾讯云未发布 eid 拆分 SDK 包），复用 common.Client TC3 签名（services/tencentcloud/eid_real.go）。

**原则：腾讯云配置未就位不阻塞注册/测试；采集与存储始终可用。**

| 能力 | 有配置（腾讯云） | 无配置（降级） |
|------|-----------------|----------------|
| 证件照上传（三张） | ✅ | ✅ |
| 自拍采集界面（图+视频） | ✅ | ✅（小程序相机组件，采集照常） |
| 自拍数据存储 `face_captures/{userID}/` | ✅ 长期保存 | ✅ 长期保存（GC 豁免） |
| 身份证信息提取（OCR，#1782） | ✅（腾讯云 OCR） | ❌ 跳过（人工看照片） |
| 生物特征比对（人脸） | ✅（E证通跳转 / 慧眼 FaceID） | ❌ 跳过 → **人工审核** |
| 核身确认 | 自动（method=tencent） | 人工（method=manual） |

**人工审核流程**（#1807 修订：员工填写 5 项实名信息）：
1. 顾客提交自拍 → `uploaded → pending_review`（新增待审核态）
2. PC「实名审核队列」（**平台员工/系统管理员**，SysPerm user 类权限，非 org 隔离）：查看证件照三张 + 自拍图/视频
3. **员工根据身份证照核对填写 真实姓名 + 身份证号 + 身份证有效期 + 签发机关 + 住址**（宽松校验：非空即可；有效期 `YYYY-MM-DD` 或「长期」；OCR 可用时预填辅助）→ 通过（approve 必填 5 项）；驳回（reject 必填原因）→ 顾客重新采集
4. 通过 → `verified`（method=manual）+ 5 项实名信息一并落库；驳回 → `rejected` + 原因
5. 自动比对（tencent）失败**不自动降级**人工——慧眼失败 ≠ 自拍无效，由顾客重新发起

**客户（商户）需完成的外部配置**（未完成时功能自动降级，不阻塞）：
1. 腾讯云控制台：注册账号 → 实名认证（企业）→ 开通「人脸核身」服务（按次计费）→ 创建 API 密钥（SecretId/SecretKey）
2. 开通「文字识别 OCR」（#1782 身份证信息提取用，可选）
3. 微信小程序后台：添加「慧眼人脸核身」插件（需企业主体，审核 1-3 工作日）
4. `.env` 配置 `TENCENTCLOUD_SECRET_ID` / `TENCENTCLOUD_SECRET_KEY` / `TENCENTCLOUD_FACEID_REGION`（三环境各一份）

### 核身状态派生与消费（#1787 补充设计）

**五态状态机**（后端提供 `id_verify_status`，前端禁止自行推导）：

```
none（未上传证件照）
  → uploaded（证件照已传，未提交自拍）
  → pending_review（已提交自拍，待人工审核）← 人工兜底引入的状态
  → verified（核身通过）
  → rejected（审核驳回，可重新采集提交）→ 回到 pending_review
```

| 状态 | 判定 | 顾客端展示 | 员工端 |
|------|------|-----------|--------|
| `none` | 三张照片均未上传 | 编辑资料页黄字「请先在上方上传身份证照片」 | 待发货列表角标「未核身」 |
| `uploaded` | 有照片未提交自拍 | 编辑资料页黄字「已上传身份证照片，请完成人脸识别」+ 链接「发起人脸识别」→ 人脸识别页（weapp 全屏 Camera 引导采集 / H5 卡片式上传） | 角标「未核身」+ 发货按钮置灰 |
| `pending_review` | 自拍已提交，审核中 | 提示「核身审核中，预计 1-2 个工作日」 | 角标「审核中」（发货按钮置灰） |
| `verified` | 审核/比对通过 | 无警告 | 正常发货 |
| `rejected` | 审核驳回 | 编辑资料页黄字「审核未通过，请重新发起人脸识别」+ 链接「发起人脸识别」 | 角标「未核身」+ 发货按钮置灰 |

**审核角色**：平台员工 + 系统管理员（SysPerm user 类权限，如 `SysPermUserUpdate`）——商户侧员工无权审核（用户对商户不可见，架构约束）。平台员工由 `PLATFORM_ROOT_ORG_ID`（env）标识，`GetBusinessRole` 在 merchant_admin 判定前分支识别。

**核身来源标记**：`face_verified=true` 时同时记录 `face_verify_method`（`tencent`=自动比对 / `manual`=人工审核）与 `face_verified_at`；信息变更（real_name/id_card_no）重置验证并清除来源标记，**同时作废该用户所有 pending 批次**（rejected, reason="身份信息已变更，请重新采集"）——防止审核基于旧身份信息提交的批次。

**信息变更判定（#1807 修订）**：仅当提交值与现值**实际不同**才视为身份变更（`identityChanged` 做值比较）——顾客编辑资料（EditProfile）会原样重提交已上传的 `id_photo_front/back`，若按"字段存在"判定会误作废 pending 批次（真实事故：顾客提交自拍后再次保存资料 → 批次被作废 → 审核队列空）。同值重提交：保留 face_verified、保留 pending 批次。

**核身采集入口**（#1811 修订）：
- 「发起人脸识别」：**始终可用**——编辑资料页按 `id_verify_status` 显示链接（uploaded/rejected 态可点击），进入人脸识别页
- 人脸识别页（weapp）：全屏 Camera 前摄预览 + 快门（拍照帧 → 过渡界面[确认照片 + 重新拍摄/继续录视频] → 录像 5s → 最后 2s 随机动作引导 → 停止）→ 分离上传 → 自动返回编辑资料页
- 人脸识别页（H5）：卡片式上传（FaceCaptureUploader，自拍照片 + 动态视频，下期统一为前摄引导）

**隐私声明**（注册/核身页明示）：自拍数据用途（实名核身）+ 存储期限（长期保存）+ 隐私政策链接；生物特征数据合规留存。

**采集界面一致性**：无论腾讯云是否配置，顾客端核身页均出现自拍采集界面（#1811：weapp 全屏 Camera 前摄引导 + 快门拍照录像；H5 卡片式上传，自拍照片 + 可选视频）；weapp 在拍照与录像之间新增过渡界面（确认照片 + 点击继续），录像阶段随机提示 1 个动作（眨眨眼/左右转头/张嘴/微笑）；自拍数据上传至 `face_captures/{userID}/`（长期保存，GC 豁免），配置缺失时采集照常进行，仅跳过自动比对。

**动作池与随机规则**（#1815）：
- weapp 人脸核身采集使用随机动作提示替代固定眨眼引导，用于活体防伪（防录制固定眨眼视频绕过检测）
- 动作池：`请眨眨眼` / `请左右转头` / `请张嘴` / `请微笑`
- 随机逻辑：用户点击「继续录视频」开始录像时，从前端动作池随机抽取 1 项，存入当前采集会话状态
- 提示时机：录像最后 2s 显示随机抽中的动作文案（替代原固定「请眨眨眼」）
- 范围：仅 weapp 全屏模式；H5 卡片式上传不涉及（自带用户控制节点）

**人工审核流程**（腾讯云未配置或自动比对兜底，#1807 修订）：
1. 顾客提交自拍 → 状态 `uploaded → pending_review`
2. **平台员工/系统管理员**（PC「实名审核队列」，SysPerm user 类权限）查看证件照三张 + 自拍图/视频
3. **员工根据身份证照填写 5 项实名信息**（real_name + id_card_no + id_card_expire + id_card_authority + id_card_address，approve 必填，宽松校验）→ 通过 → `verified`（method=manual）+ 实名信息落库；驳回 → `rejected` + 原因，顾客重新采集
4. 自动比对失败不自动降级人工（慧眼失败 ≠ 自拍无效，由顾客重新发起）
5. 审核动作双写留痕：`face_capture_batches.reviewed_by/at` + `audit_logs`（action=face_review_approve/reject）

**订单接口买家核身状态边界**：`GET /orders/:id` / 员工订单列表仅返回买家 `id_verify_status`（聚合状态，角标/校验数据源）；**禁止返回** real_name/id_card_no/证件照/自拍素材——用户对商户不可见原则。

**消费点规则**：
1. **订单确认页（/checkout）**：`none/uploaded/rejected` → 警告条 + 跳转按钮，**不阻断提交**（核身不设支付门槛）；`pending_review/verified` 无警告
2. **订单详情页（/order/:id）**：订单未发货时同警告；已发货后不再展示（履约已开始）
3. **员工发货（PUT /warehouse/orders/:id/shipping）**：**后端强制校验** `id_verify_status=verified` → 否则拒绝（错误码 40002，message=`user id verification required`，前端映射「用户未完成实名核身，请联系平台运营完成审核」）；前端按钮置灰仅为 UX，不能替代后端校验；**无豁免**（无存量真实用户，全量强制）
4. **待发货列表**：非 verified 订单显示角标（未核身/审核中），便于员工识别
5. **错误展示**：员工 weapp/H5 发货页（ShippingInterface）点击发货被拒 → toast/弹窗展示后端 40002 错误信息（**非静默**）；员工端不展示买家敏感字段（仅聚合状态）

### 注册阶段照片上传（#1787）
- weapp 注册页使用会话级匿名端点 `POST /auth/registration-sessions/:id/id-photo`
- H5 注册页使用已登录端点 `POST /user/id-photo`
- 注册完成时 `completeRegistrationFromSession` 自动将 form_data.id_photos 转移到 users 表

## 全局审计：所有需修改的页面

| # | 平台 | 页面 | 角色 | 动作 | 当前状态 |
|---|------|------|------|------|:---:|
| 1 | H5 | `/register` | guest | 注册时上传（3 张） | **已实现** |
| 2 | H5 | `/onboarding` | customer | 登录后上传 | 上传可用但**不持久化** |
| 3 | weapp | `/pages-weapp/profile-complete/index` | guest | 注册时上传（3 张 + 会话端点） | **已实现** |
| 4 | H5 | `/profile/edit` | customer | 查看+替换+删除（3 张 + 证件类型）；实名信息员工审核填写 | **已实现** |
| 5 | weapp | `/pages-weapp/profile/edit/index` | customer | 查看+替换+删除（3 张 + 证件类型）；实名信息员工审核填写 | **已实现** |
| 6 | PC | `/system/user-management` | namespace_admin | 查看+替换+删除 | **缺失**（P-02 已建页但无此区域） |
| 7 | PC | `/orders/:id` | site_admin/member | 发货/收货时查看 | **缺失** |
| 8 | H5 | `/repair-request` | staff | 操作时查看 | **缺失** |
| 9 | weapp | `/repair-request` | staff | 操作时查看 | **缺失** |
| 10 | weapp+h5 | `/checkout` | customer | 核身状态警告 + 跳转（不阻断） | **已实现**（#1787/#1807） |
| 11 | weapp+h5 | `/order/:id` | customer | 订单详情核身警告 + 跳转 | **已实现**（#1787/#1807） |
| 12 | weapp+h5 | `/staff/shipping` | site_admin/member | 发货按钮置灰 + 后端强制校验 | **已实现**（#1787） |
| 13 | PC | `/face-review` | 平台员工/系统管理员 | 审核队列：查看证件照+自拍 → **填写 real_name/id_card_no** → 通过/驳回 | **已实现**（#1793/#1807） |

## 验收
- Go test: POST id-photo → DB 列值非空，GET id-photos → 返回 URL，DELETE → 列值清空
- Go test: face-verify token/result → 40012 (未配置) / 20000 (通过/失败)
- checklist-verify.py: P-04 displays 与 GET /users/me、GET /user/:userId/id-photos 响应交叉验证
- 端到端: 注册上传 → 编辑资料可见 → 替换 → 编辑资料更新 → 删除 → 编辑资料显示未上传
- weapp 人脸核身采集流程：拍照后出现过渡界面（确认照片 + 重新拍摄/继续录视频）；「继续录视频」后录像 5s + 最后 2s 随机动作提示
- weapp 连续多次采集应观察到不同动作提示（随机性可被观察）
- weapp 重拍路径：照片不满意可重拍，不残留旧照片（photoPathRef 清空）
- weapp 「继续录视频」前无录像启动（无后台录音）
- weapp 提交后审核队列（PC「实名审核队列」）应能看到**自拍图 + 视频**两项素材（非仅图片）——分离上传两步均成功：image 建批次 + video 按 batch_id 追加（#1815 后曾因 `stopRecord` 读错字段 `tempFilePath`→`tempVideoPath` 导致视频静默丢失，仅图片上传，人工测试才暴露）

## 关联 Issue
- 后端数据层 (DB + API)：[tuneloop#1598](https://github.com/HiJohns/tuneloop/issues/1598)
- 前端全平台接入：[tuneloop#1599](https://github.com/HiJohns/tuneloop/issues/1599)
- H5 注册页（含身份证上传入口）：[tuneloop#1597](https://github.com/HiJohns/tuneloop/issues/1597)
- 身份证三处上传 + 人脸识别核身：[tuneloop#1787](https://github.com/HiJohns/tuneloop/issues/1787)

## 已知缺口
- 未定义身份证过期/重新认证机制（首次上传即永久有效）——#1807 起记录有效期 id_card_expire（员工填写），到期提醒/重认证机制未定义
- 未定义 OCR 自动识别（身份信息全靠人工查看）→ #1782 已 accepted
- 未定义存储清理策略（旧文件被替换后残留于 media storage）

---

*Last updated: 2026-08-27*
