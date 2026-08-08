---
id: O-01
domain: organization
flow: 新建网点
steps:
  - seq: 1
    action: 进入网点管理
    frontend:
      - platform: [pc]
        page: /sites
        role: [merchant_admin, site_admin]
        gate: "拥有 organization:create 权限"
        reach: "组织管理 → 网点管理"
        controls: [网点树, 创建顶级网点按钮]
        displays: [网点树状列表]
        ops:
          - {type: navigate, target: /sites/new}
  - seq: 2
    action: 填写网点表单
    frontend:
      - platform: [pc]
        page: /sites/new
        role: [merchant_admin, site_admin]
        gate: "名称必填"
        reach: ""
        controls: [名称输入, 类型选择(加盟/直营/合作), 地址, 电话, 管理员搜索, 创建新用户按钮]
        displays: [选中管理员姓名/邮箱, 名称重复校验结果]
        ops:
          - {type: interact}
          - {type: api, method: POST, path: /merchant/sites}
    api: {method: POST, path: /merchant/sites, params: [name, type, address, phone, manager_id]}
  - seq: 3
    action: 提交（IAM 绑定）
    frontend:
      - platform: [pc]
        page: /sites/new
        role: [merchant_admin, site_admin]
        gate: ""
        reach: ""
        controls: [提交按钮]
        displays: [网点树更新, 新网点选中]
        ops:
          - {type: api, method: POST, path: /merchant/sites}
          - {type: navigate, target: /sites/:id}
---

# O-01 新建网点

## 前置条件
- 商户管理员或租户全局权限

## 流程
1. 网点管理 → 创建顶级网点
2. 填表单（名称/类型/地址/电话/管理员）
3. 提交 → 后端 IAM 三步绑定（org 创建 + bind + cus_perm + role 模板）
4. 网点树更新

## 关键规则
- IAM 下级组织 parent_id = 商户 org_id
- 管理员初始角色 site_admin
- 本地 site_members 同步

## 验收
- `go test -run TestOrgManagement ./handlers/ -v`

---
id: O-02
domain: organization
flow: 网点人员管理
steps:
  - seq: 1
    action: 查看人员列表
    frontend:
      - platform: [pc]
        page: /sites/:id/members
        role: [merchant_admin]
        gate: "商户管理员或租户全局权限"
        reach: "网点详情 → 人员"
        controls: [用户搜索框, 创建新用户按钮, 角色切换, 移除按钮]
        displays: [用户名, 角色类别, 操作]
        ops:
          - {type: api, method: GET, path: /sites/:id/members}
    api: {method: GET, path: /sites/:id/members, params: []}
  - seq: 2
    action: 添加成员
    frontend:
      - platform: [pc]
        page: /sites/:id/members
        role: [merchant_admin]
        gate: ""
        reach: ""
        controls: [多选用户, 确认添加按钮]
        displays: [已选用户列表]
        ops:
          - {type: api, method: POST, path: /sites/:id/members}
    api: {method: POST, path: /sites/:id/members, params: [user_ids, role]}
  - seq: 3
    action: 切换角色
    frontend:
      - platform: [pc]
        page: /sites/:id/members
        role: [merchant_admin]
        gate: "非最后一名管理员"
        reach: ""
        controls: [角色下拉]
        displays: []
        ops:
          - {type: api, method: PUT, path: /sites/:id/members/:uid}
    api: {method: PUT, path: /sites/:id/members/:uid, params: [role]}
---

# O-02 网点人员管理

## 前置条件
- 商户管理员

## 流程
1. 人员列表 → 搜索/新建用户
2. 添加成员（IAM 三步绑定，初始 site_member）
3. 切换角色（site_admin↔site_member，IAM 同步）
4. 移除成员（最后一名管理员保护）

## 关键规则
- 角色名映射：site_admin→ADMIN, site_member→STAFF, worker→WORKER
- 最后一名管理员不可切换/移除
- 下级组织绑定无需用户确认，即时生效

---
id: O-03
domain: organization
flow: 删除网点
steps:
  - seq: 1
    action: 删除网点
    frontend:
      - platform: [pc]
        page: /sites/:id
        role: [merchant_admin]
        gate: "通过前置检查"
        reach: "网点详情 → 删除"
        controls: [删除按钮]
        displays: [资产校验提示, 人员校验提示]
        ops:
          - {type: api, method: DELETE, path: /merchant/sites/:id}
    api: {method: DELETE, path: /merchant/sites/:id, params: []}
---

# O-03 删除网点

## 前置条件
- 商户管理员

## 流程
1. 删除前校验：无 available/rented 乐器 + 无成员
2. 通过后删除

## 关键规则
- 有在库乐器 → "请先转移资产"
- 有在租乐器 → "请先处理在租订单"
- 有成员 → "请先移除所有成员"

---

*Model: deepseek/deepseek-v4-flash*
