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
        role: [tenant_admin]
        gate: "拥有 sys_perm bit[5]（租户管理员）"
        reach: "系统管理 → 用户管理"
        controls: [搜索框, 导出CSV按钮, 列表表格, 详情/编辑按钮]
        displays: [注册名, 微信号, 电话, 当前等级, 当前积分, 注册时间, 最新活动, 状态]
        ops:
          - {type: api, method: GET, path: /admin/user-management}
          - {type: interact}
    api: {method: GET, path: /admin/user-management, params: [page, pageSize, search]}
  - seq: 2
    action: 查看用户详情并编辑
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [tenant_admin]
        gate: ""
        reach: "列表 → 详情/编辑 → Modal 弹窗"
        controls: [等级输入, 积分输入, 禁用/可用开关, 保存按钮]
        displays: [当前等级, 当前积分, 当前状态]
        ops:
          - {type: api, method: GET, path: /admin/user-management/:id}
          - {type: api, method: PUT, path: /admin/user-management/:id}
    api: {method: PUT, path: /admin/user-management/:id, params: [membership_level_id, promo_points, status]}
  - seq: 3
    action: 导出用户 CSV
    frontend:
      - platform: [pc]
        page: /system/user-management
        role: [tenant_admin]
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
        role: [tenant_admin]
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
- 平台管理员（租户管理员）已登录 PC 端
- 拥有 sys_perm bit[5]（系统管理权限）

## 流程
1. 系统管理 → 用户管理 → 列表（搜索/分页）
2. 点击"详情/编辑" → Modal 弹窗（等级/积分/禁用开关）
3. 保存 → PUT /admin/user-management/:id
4. 导出 CSV → GET /admin/user-management/export
5. 禁用用户 → 下次登录返回 403"该账户已被禁用"（后端 EnsureLocalUser + WxLogin 双重拦截）

## 关键规则
- 禁用用户对所有认证接口生效（EnsureLocalUser 统一入口）
- 列表字段：username, wx_openid, phone, level(会员等级名), points(promo_points), registered_at, last_active(取 UpdatedAt), status
- CSV 导出复用列表查询参数（支持搜索过滤）
- 等级输入：membership_level_id（整数），前端默认配置的 level ID

## 验收
- `go test ./handlers/ -count=1` 回归通过
- PC `npm run build` 通过

---
*Model: deepseek/deepseek-v4-flash*
