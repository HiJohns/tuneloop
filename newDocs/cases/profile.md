---
id: P-01
domain: profile
flow: 个人资料编辑
steps:
  - seq: 1
    action: 打开个人中心
    frontend:
      - platform: [weapp, h5]
        page: /profile
        role: [customer]
        gate: "已登录（非游客）"
        reach: "底部导航 → 我的"
        controls: [编辑资料入口, 昵称显示, 会员等级显示]
        displays: [标题栏显示名, 昵称, 会员等级, 积分]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
  - seq: 2
    action: 进入编辑页
    frontend:
      - platform: [weapp]
        page: /profile/edit
        role: [customer]
        gate: ""
        reach: "个人中心 → 编辑资料"
        controls: [昵称输入, 手机号输入, 邮箱输入, 保存按钮]
        displays: [当前昵称, 当前手机号, 当前邮箱]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
  - seq: 3
    action: 保存修改
    frontend:
      - platform: [weapp]
        page: /profile/edit
        role: [customer]
        gate: ""
        reach: ""
        controls: [保存按钮]
        displays: []
        ops:
          - {type: api, method: PUT, path: /users/me}
          - {type: interact, feedback: "toast 保存成功，延迟 800ms 后 navigateBack"}
    api: {method: PUT, path: /users/me, params: [nickname, phone, email]}
  - seq: 4
    action: 返回个人中心验证
    frontend:
      - platform: [weapp, h5]
        page: /profile
        role: [customer]
        gate: ""
        reach: "编辑页 navigateBack 返回"
        controls: [昵称显示]
        displays: [标题栏显示名, 更新后的昵称, 更新后的手机号]
        ops:
          - {type: api, method: GET, path: /users/me}
    api: {method: GET, path: /users/me, params: []}
---

# P-01 个人资料编辑

## 前置条件
- 用户已登录（顾客）

## 已知缺口（待实施）
- **H5 侧无编辑资料功能**：H5 仅有弹窗（姓名/手机/邮箱），无独立编辑页、无昵称编辑
  - 待办：H5 端补独立编辑页或扩展弹窗支持昵称（其他同事实施）
  - 本用例的 seq2/seq3（/profile/edit）目前仅 weapp 覆盖

## 流程
1. 个人中心 → 编辑资料（weapp 独立页 / H5 弹窗）
2. 修改昵称/手机号/邮箱 → 保存 → PUT /users/me
3. toast「保存成功」→ 延迟 800ms → navigateBack 返回个人中心
4. 个人中心 useDidShow 刷新 → 标题栏和列表显示更新后的值

## 关键规则

### 显示规则（问题 3）
- 标题栏显示名优先级：`nickname || name || '路人'`（昵称优先于姓名）
- weapp 与 H5 必须一致

### 昵称语义（问题 1）
- 编辑页字段标签为「昵称」（不是「微信昵称」）
- 微信昵称仅作为**默认值预填**，用户可自由修改
- 不得使用 `type="nickname"`（微信强制昵称控件会锁死输入）

### 保存契约（问题 2）
- PUT /users/me 必须真实落库：本地 users 表 + IAM 双侧更新
- 顾客无 tenant：本地更新按 `iam_sub` 匹配，不得带空 tenant_id（22P02）
- 后端必须检查更新错误，失败返回 5xx 而非吞错返回 success
- 电话更新需 IAM 侧真实保存（beaconiam phone stub 待修）

### 布局规范（问题 4）
- 字段间必须有明确间隔（如 marginBottom: 16）
- 小程序 boxSizing 默认 content-box：输入框宽度不得用 `100% + padding` 组合（会溢出/文字贴边）
- 多字段表单分行分列显示，不得粘连

### 保存后跳转（问题 5）
- toast「保存成功」→ 延迟 ~800ms → navigateBack（微信标准模式）
- 返回后个人中心必须已刷新（useDidShow）

## 验收（对应 API 测试）
- `go test` 覆盖：PUT /users/me 后 GET 回读 nickname/phone 变化
- checklist-verify.py：P-01 displays 字段与 GET 响应结构体交叉验证

---

# P-02 员工个人中心（组织上下文可见性）

## 前置条件
- 员工（STAFF）已登录（JWT 含 oid/tid）

## 需求来源
- 员工登录后无法从界面上看出自己属于哪个商户、哪个网点（#1665 排查中用户反馈）
- 跨网点员工（site_members 多网点）尤其需要知道当前上下文，避免误操作（#688 数据隔离教训）

## 流程
1. 员工登录 → 底部导航「我的」→ 个人中心
2. 头像下方昵称后显示**小字标签**：`商户名 · 网点名`（如「新街口琴行 · 新街口店」）
3. 数据来源：`GET /users/me` 返回 `tenant_name`（商户名）+ `site_name`（主网点名）

## 前端设计
- 标签样式：昵称（fontSize 24/900）右下方小字（fontSize 12，灰色 #71717a），如徽标形式
- 仅员工（`isStaff`，JWT oid/tid 非空）显示；顾客不显示
- 无网点（site_members 无记录）→ 仅显示商户名

## 接口契约
- `GET /users/me`（GetCurrentUser）须返回：
  - `site_id` / `site_name`：主网点（site_members JOIN sites，已有）
  - `tenant_name`：商户名（merchants 表按 tenant_id 查 name，**待补充**）

## 验收
- [ ] 员工个人中心昵称后显示「商户名 · 网点名」小字标签
- [ ] 顾客不显示该标签
- [ ] GET /users/me 返回 tenant_name 字段（后端测试覆盖）

---

*Model: deepseek/deepseek-v4-pro*
