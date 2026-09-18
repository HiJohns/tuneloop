---
id: C-01
domain: category
flow: 分类管理（PC 维护 → 小程序首页展示）
steps:
  - seq: 1
    action: 进入分类管理页
    frontend:
      - platform: [pc]
        page: /admin/categories
        role: [namespace_admin, tenant_admin]
        gate: "拥有 category:manage 权限"
        reach: "系统管理 → 分类管理"
        controls: [分类树, 新建按钮]
        displays: [分类名称, 排序值, 隐藏状态]
        ops:
          - {type: api, method: GET, path: /categories}
  - seq: 2
    action: 新建/编辑/删除分类
    frontend:
      - platform: [pc]
        page: /admin/categories
        role: [namespace_admin, tenant_admin]
        gate: "category:manage 权限"
        reach: ""
        controls: [名称输入, 父级选择, 排序值输入, 可见开关, 保存/删除按钮]
        displays: []
        ops:
          - {type: api, method: POST, path: /categories}
          - {type: api, method: PUT, path: /categories/:id}
          - {type: api, method: DELETE, path: /categories/:id}
    api: {method: POST, path: /categories, params: [name, parent_id, visible, sort]}
  - seq: 3
    action: 排序（首页菜单顺序）
    frontend:
      - platform: [pc]
        page: /admin/categories
        role: [namespace_admin, tenant_admin]
        gate: "category:manage 权限"
        reach: "分类列表 ↑↓ 按钮"
        controls: [上移, 下移]
        displays: []
        ops:
          - {type: api, method: PUT, path: /categories/sort, params: [items]}
    api: {method: PUT, path: /categories/sort, params: [items: [{id, sort}]]}
  - seq: 4
    action: 隐藏分类
    frontend:
      - platform: [pc]
        page: /admin/categories
        role: [namespace_admin, tenant_admin]
        gate: "category:manage 权限"
        reach: "分类列表 👁 按钮"
        controls: [隐藏按钮, 显示按钮]
        displays: []
        ops:
          - {type: api, method: PUT, path: /categories/:id, params: [sort <= 0]}
    api: {method: PUT, path: /categories/:id, params: [sort]}
  - seq: 5
    action: 小程序首页分类展示
    frontend:
      - platform: [weapp]
        page: /home
        role: [guest, customer]
        gate: ""
        reach: "打开首页 → GET /public/categories"
        controls: [顶级分类菜单]
        displays: [可见顶级分类（visible=true 且 sort>0，按 sort ASC）]
        ops:
          - {type: api, method: GET, path: /public/categories}
    api: {method: GET, path: /public/categories, params: [tenant（可选）]}
  - seq: 6
    action: 点击顶级分类显示子分类
    frontend:
      - platform: [weapp]
        page: /home
        role: [guest, customer]
        gate: "顶级分类存在子分类"
        reach: "点击顶级分类 → 展开子分类菜单"
        controls: [子分类菜单项]
        displays: [子分类（parent_id 匹配，visible/sort 过滤，按 sort ASC）]
        ops:
          - {type: interact}
---

# C-01 分类管理

## 前置条件
- PC 端角色为 namespace_admin / tenant_admin，拥有 `category:manage` 权限（分类为平台级共享资源）
- 小程序端为公开访问（无需登录）

## 流程
1. 分类管理页展示分类树（顶级 + 子分类，parent_id 关联）
2. 新建/编辑/删除分类（name、parent_id、visible、sort）
3. 排序：`PUT /categories/sort`，sort 值越小越靠前
4. 隐藏：编辑分类 sort 设为 <=0（或 visible=false）→ 首页不显示
5. 小程序首页 `GET /public/categories`：返回 `visible=true AND sort>0` 的分类（顶级+子分类），按 `sort ASC, created_at ASC`
6. 点击顶级分类 → 前端从返回列表 filter `parent_id === cat.id` 展示子分类

## 关键规则
- **分类展示的唯一数据源是 `category.visible/sort` 字段**（PC 管理员通过分类管理页维护）
- `home_menu_config`（system_settings）为废弃残留，**不参与**分类展示过滤/排序（#1645）
- 隐藏 = sort<=0 或 visible=false；两者任一满足即不显示
- 子分类同样受 visible/sort 过滤

## 验收（对应 API 测试）
- `go test -run TestGetPublicCategories ./handlers/ -v`
- 断言点：
  - visible=true 且 sort>0 的分类返回，按 sort ASC
  - sort<=0 或 visible=false 的分类不返回
  - home_menu_config 存在时不影响结果（#1645 修复后）

---

*Last updated: 2026-08-13*
