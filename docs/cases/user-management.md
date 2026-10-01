---
id: P-02
domain: administration
flow: 平台管理员用户管理
steps:
  - seq: 1
    action: 查看用户列表
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [namespace_admin]
        gate: "拥有 sys_perm bit[16]（平台管理员）"
        reach: "系统管理 → 用户管理"
        controls: [搜索框, 导出CSV按钮, 列表表格, 昵称列（可点击）, 操作列（详情/标记删除）]
        displays: [昵称, 微信绑定, 电话, 当前等级, 当前积分（点）, 注册时间, 最新活动, 状态]
        ops:
          - {type: api, method: GET, path: /admin/user-management}
          - {type: interact}
    api: {method: GET, path: /admin/user-management, params: [page, pageSize, search]}
    rule: "#1807 修订：第一列为昵称（nickname，空则回退 username/phone），点击昵称打开详情/编辑 Modal；**#2108 设计变更（覆盖 #1807 的「不再有独立操作列」约束）**：恢复列表「操作」列（详情 + 标记删除危险按钮）——用户台账走查反馈「找不到删除入口」而删除原本仅埋于详情弹窗；当前积分按点显示（1点=1元，后端存分 promo_points/100）；**状态列四态中文：active=可用（绿）/disabled=已禁用（红）/deleted=已删除（橙）/init=待激活（金）**；列表查询豁免 tenant scoping（平台级全量用户，含空租户顾客）"
  - seq: 2
    action: 查看用户详情并编辑
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [namespace_admin]
        gate: ""
        reach: "列表 → 点击昵称 → Modal 弹窗"
        controls: [等级输入, 积分输入（点）, 禁用/可用开关, 身份证正反面预览, 其他证件预览（含类型）, 保存按钮]
        displays: [当前等级, 当前积分, 当前状态, 身份证正面图, 身份证反面图, 其他证件图（类型标签）]
        ops:
          - {type: api, method: GET, path: /admin/user-management/:id}
          - {type: api, method: PUT, path: /admin/user-management/:id}
    api: {method: PUT, path: /admin/user-management/:id, params: [membership_level_id, promo_points, status]}
    rule: "积分编辑按点（保存时 ×100 转分）；证件照 URL 防双前缀（历史数据存完整 URL /uploads/media/... 直接返回）；其他证件 id_photo_other + 类型 id_photo_other_type（readOnly 展示；**缺省时明示「类型未定：由审核员在实名审核时指定」**）；**#2108**：`personnel_type=staff` 时隐藏顾客专属区块（身份证照片/其他证件照片/介绍信/实名核身）；介绍信 `intro_letter_url` 有值时以图片预览展示（学生作为第二证件需上传）"
  - seq: 3
    action: 导出用户 CSV
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [namespace_admin]
        gate: ""
        reach: "列表 → 导出CSV按钮"
        controls: [导出CSV按钮]
        displays: []
        ops:
          - {type: api, method: GET, path: /admin/user-management/export}
    api: {method: GET, path: /admin/user-management/export, params: [search]}
  - seq: 4
    action: 禁用用户登录
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [namespace_admin]
        gate: ""
        reach: "列表 → 详情 → 禁用开关"
        controls: [禁用/可用开关, 保存按钮]
        displays: [当前状态]
        ops:
          - {type: api, method: PUT, path: /admin/user-management/:id}
    api: {method: PUT, path: /admin/user-management/:id, params: [status]}
---

# P-02 平台管理员用户管理

## 前置条件
- 平台管理员（namespace_admin）已登录 PC 端
- 拥有 sys_perm bit[16]（系统管理权限）

## 流程
1. 系统管理 → 用户管理 → 列表（搜索/分页）
2. 点击昵称 → Modal 弹窗（等级/积分/禁用开关/证件照）
3. 保存 → PUT /admin/user-management/:id
4. 导出 CSV → GET /admin/user-management/export
5. 禁用用户 → 下次登录返回 403"该账户已被禁用"（后端 EnsureLocalUser + WxLogin 双重拦截）

## 关键规则
- 禁用用户对所有认证接口生效（EnsureLocalUser 统一入口）
- 列表字段：nickname（第一列，可点击）, 微信绑定（`wx_user_bindings`，已废弃 `users.wx_openid` —— #2019）, phone, level(会员等级名), points(显示点=分/100), registered_at, last_active(取 UpdatedAt), status
- 搜索覆盖 nickname/name/username/phone/微信绑定 openid（经 `wx_user_bindings` 关联；不再直查 `users.wx_openid`）
- **用户列表（#2025 D4）**：平台级全量用户；提供**标记删除**（解除全部关联，用户记录保留、可重新加回）；既有「顾客列表（不可删）」保留
- **tenant scoping 豁免**：List/Get/Update/Export/AdminUploadIDPhoto/AdminDeleteIdPhoto 使用清空 TenantIDKey 的 context（platformDB）——用户管理是平台级功能，必须显示全部注册用户（含空租户顾客 tenant_id=00000000，否则新注册会员不显示）
- 证件照 URL：resolveStorageKey 防双前缀（key 已含 /uploads/media/ 或 http(s):// 直接返回；历史数据 front/back 误存完整 URL）
- 详情 Modal 展示：身份证正反面（可替换/删除）+ 其他证件照（readOnly + 类型标签 id_photo_other_type）
- **#2108 员工门控**：`personnel_type=staff` → 详情 Modal 隐藏顾客专属资料区块（身份证照/其他证件照/介绍信/实名核身概不展示）；禁用/乐币/标记删除保留
- CSV 导出复用列表查询参数（支持搜索过滤），列含 nickname
- 等级输入：membership_level_id（整数），前端默认配置的 level ID
- 积分输入：按点（1点=1元），保存时 Math.round(points*100) 存分（promo_points 为 Cents）

## 验收
- `go test ./handlers/ -count=1` 回归通过
- PC `npm run build` 通过

---

# P-07 邀请与直属成员管理（#2114）

> 阶段 2 业务层（各级管各级）。领域契约见 [organization.md](./organization.md) O-02/O-04/O-05；UI 见 [ui.md §3.28](../spec/ui/ui.md)。

## 前置条件
- 商户管理员（`member:invite`）或系统管理员（system_admin）已登录 PC
- 待邀请对象已在本平台注册（手机号/邮箱可定位到账户）

## 流程
1. **邀请管理**（人员管理 → 邀请管理，`/staff/invites`）
   - 网点管理员为非本商户成员（P2 已注册 / P3 未注册）提交加入申请 → `POST /api/sites/:id/members/apply`
   - 商户管理员在列表看到 `pending_approval` → **同意**（`POST /api/admin/invites/:id/approve`，转 `pending` 并发邀请）或 **拒绝**（`POST /api/admin/invites/:id/reject`，带原因）
   - 邀请有效期 72h；被邀请人接受时（移动端「我的 → 系统消息」）自注册/绑定 → `accepted`
2. **直属成员管理**（人员管理 → 直属成员管理，`/staff/direct`）
   - 商户管理员：`GET /api/admin/my-merchant` 取本商户 → 「邀请直属员工」`POST /api/admin/merchants/:id/invites {kind:'merchant_staff'}`；「创建员工」跳 `/staff`
   - 平台管理员：「邀请平台员工」`POST /api/admin/platform-staff/invites`
3. **消息中心**（PC 顶栏铃铛）：未读徽标 + 列表 + 动作直达（`invite_manage` → `/staff/invites`）

## 关键规则
- **各级管各级**：网点管理员只能直接添加「已是本商户成员」的用户（P1）；其余一律走申请审批（P2/P3 返回 `40311 need_apply`）。商户管理员 / 系统管理员 / 平台员工不受限；中转网点（`type=transit`）豁免（#1938）。
- **审批 = 邀请**：同一枚权限 `member:invite`；`merchant_admin` 模板默认含。
- **`member` 为平凡角色**：仅用于枚举，不参与门禁；判定「是否 B 成员」以 IAM 上 B 关系 active 为准（member 或 staff 均算）。
- **邀请有效期 72h**；先支持单人（一人一行）。
- **P3 自注册**：被邀请人接受时自注册建号（见 [account-lifecycle.md](../ops/account-lifecycle.md)）。
- 前端入口：人员管理「邀请管理」(待审批徽标) + 「直属成员管理」，仅 `merchant_admin`/`system_admin` 可见。

## 验收
- `go build .` + `go test ./handlers/ -run TestMemberInvite2114` 通过
- PC `npm run build`（vite build）通过；ESLint 无新增 `no-undef`
- 端到端：网点管理员申请 → 商户管理员审批 → 被邀请人接受（自注册）→ 关系正确（A:member / B:member / C:role）
