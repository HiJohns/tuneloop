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
        controls: [搜索框, 导出CSV按钮, 列表表格, 昵称列（可点击）]
        displays: [昵称, 微信号, 电话, 当前等级, 当前积分（点）, 注册时间, 最新活动, 状态]
        ops:
          - {type: api, method: GET, path: /admin/user-management}
          - {type: interact}
    api: {method: GET, path: /admin/user-management, params: [page, pageSize, search]}
    rule: "#1807 修订：第一列为昵称（nickname，空则回退 username/phone），点击昵称打开详情/编辑 Modal（不再有独立操作列）；当前积分按点显示（1点=1元，后端存分 promo_points/100）；列表查询豁免 tenant scoping（平台级全量用户，含空租户顾客）"
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
    rule: "积分编辑按点（保存时 ×100 转分）；证件照 URL 防双前缀（历史数据存完整 URL /uploads/media/... 直接返回）；其他证件 id_photo_other + 类型 id_photo_other_type（readOnly 展示）"
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
- 列表字段：nickname（第一列，可点击）, wx_openid, phone, level(会员等级名), points(显示点=分/100), registered_at, last_active(取 UpdatedAt), status
- 搜索覆盖 nickname/name/username/phone/wx_openid
- **tenant scoping 豁免**：List/Get/Update/Export/AdminUploadIDPhoto/AdminDeleteIdPhoto 使用清空 TenantIDKey 的 context（platformDB）——用户管理是平台级功能，必须显示全部注册用户（含空租户顾客 tenant_id=00000000，否则新注册会员不显示）
- 证件照 URL：resolveStorageKey 防双前缀（key 已含 /uploads/media/ 或 http(s):// 直接返回；历史数据 front/back 误存完整 URL）
- 详情 Modal 展示：身份证正反面（可替换/删除）+ 其他证件照（readOnly + 类型标签 id_photo_other_type）
- CSV 导出复用列表查询参数（支持搜索过滤），列含 nickname
- 等级输入：membership_level_id（整数），前端默认配置的 level ID
- 积分输入：按点（1点=1元），保存时 Math.round(points*100) 存分（promo_points 为 Cents）

## 验收
- `go test ./handlers/ -count=1` 回归通过
- PC `npm run build` 通过

---
*Model: deepseek/deepseek-v4-flash*
