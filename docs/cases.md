# 用例合集

## 管理后台菜单分类（#1545 #1573 重构）

> 按"操作对象"划分，替代原"经营策略/运营管理"混叠方案。

| 菜单组 | 含义 | 包含项 | 角色可见性 |
|--------|------|--------|-----------|
| **平台管理** | 平台级实体（人、组织、权限） | 商户、用户、网点、人员、权限、警告 | sysPerm 控制 |
| **商品管理** | 乐器相关配置与库存 | 分类、属性、乐器列表、库存监控 | cusPerm |
| **交易管理** | 订单流转与库管操作 | 订单、库管工作台、会话管理 | cusPerm |
| **策略配置** | 定价/折扣/返点等规则 | 租金、定价、折扣、报修、返点、会员级别、轮播图 | cusPerm |
| **系统设置** | 纯技术运维项 | 内容编辑、仪表盘 | 基础权限 |

**设计原则**：
- 用"操作对象"（平台/商品/交易/策略/系统）而非笼统的"xxx 管理"
- 商户与用户从"系统管理"移到"平台管理"——都是平台上的实体
- 库存从"运营"移到"商品"——乐器库存是商品维度的属性
- 会员级别从"策略"划分——它是定价规则的一部分

**对应用例**：商户管理 → §0.1；用户管理 → §0.2（#1545）；网点管理 → §2.x；库存监控 → §3.x

---

## 0. 冷启动 (Bootstrapping)

**目标**: 建立系统第一个超级管理员，并锁定初始化入口。

### 0.1 系统初始化流程

1. **访问首页**: 用户访问 `/`
2. **系统检测**: 后端检测 `User` 表是否为空
3. **路由锁定**:
   - 若 `User` 表为空 → 前端自动跳转至 `/setup`
   - 若 `User` 表不为空且访问 `/setup` → 返回 403 或重定向至登录页
4. **创建系统管理员**:
   - 页面显示表单：邮箱、密码
   - 后端动作：
     a. 调用 IAM 创建该用户（角色：Project Admin）
     b. 在 Tuneloop 本地 `users` 表记录 UID
     c. 标记 `is_system_admin = true`
5. **登录循环**: 创建成功后跳转回 `/`，触发 OIDC 流程跳转 IAM 完成首次认证

---

## 0.1 商户管理 (Merchant Management)

**术语对齐**: 商户 (Merchant) → IAM 组织 (Organization)

### 0.1.1 商户列表

**权限**: 仅 JWT 中带有 `project_admin` 声明的用户可见

- 展示字段：商户名称、创建时间、商户唯一代码 (Code/Slug)
- **删除逻辑**: 若该商户下仍有活跃网点或未结清乐器订单，禁止删除

### 0.1.2 商户创建

**表单字段**:
- 商户名称
- 商户代码（用于 URL 或数据隔离标识）
- 联系人信息（姓名、邮箱、电话）
- **指定管理员**: 支持两种场景：
  1. **已有用户** — 搜索 Tab 输入用户名/邮箱/手机，下拉选中，提交 `admin_uid`
  2. **新建用户** — 创建 Tab 填写用户名/姓名/邮箱/手机，提交时 `admin_uid=null`，后端先创建 IAM 用户再创建组织

**后端动作**:
1. 判断 `admin_uid`：
   - 有值 → 场景1，直接使用该用户 ID 创建组织
   - 为 null → 场景2，先调用 IAM `POST /api/v1/users` 创建用户
2. IAM 用户创建：
   - 成功 → 获取 `user_id`，填入 `admin_uid`，继续创建组织
   - 用户名冲突 → 返回 `409` + 已存在用户信息（id/name/email/phone），前端自动切换为场景1
3. 调用 IAM `POST /api/v1/namespaces/:id/organizations` 创建顶级组织
4. IAM 确认后 302 重定向至 `callback_url`，Tuneloop 执行本地同步操作
5. Tuneloop 本地 `merchants` 表记录商户信息及管理员 UID

---

## 0.2 指定用户流程 (User Selection & Provisioning)

**设计思想**: 表单内联、先查后联、无则创建、确认会话。

> **交互模式变更**: 用户搜索与创建功能**直接内嵌在表单中**，不再使用弹窗对话框。
> 搜索框和创建用户按钮作为表单字段的一部分呈现，已选用户以列表形式显示在表单内。

> **商户创建场景特殊说明**: 管理员指定使用双 Tab 结构（搜索/创建），与下文流程一致。区别在于：
> 1. 创建 Tab 提交时 `admin_uid=null`，管理员信息随商户表单一并提交（无需单独「创建并添加」按钮）
> 2. IAM 用户名冲突时后端返回 `409` + 已存在用户详情，前端**自动切换**为搜索 Tab 并预填选中
> 3. 站点/网点成员管理等其他场景不受影响，使用标准流程（见 §0.2.1 - §0.2.4）

### 0.2.1 用户输入与搜索

**界面元素**（内嵌于表单，Tab 切换）:
- **搜索 Tab**：用户搜索输入框（AutoComplete）+ 搜索结果下拉列表
- **创建 Tab**：点击即显示创建表单
- 已选用户列表（显示在 Tab 区域下方）

**交互流程**:
1. 默认为搜索 Tab
2. 在搜索框中输入用户名、邮箱或手机号
3. 系统自动以输入为关键字搜索（debounce 300ms）
4. 下拉框显示搜索结果（最多10项），每项显示：
   - 用户名
   - 匹配的字段（如匹配到邮箱则显示邮箱）
   - 是否已与当前商户关联（associated 标志）
5. 点击结果项添加到已选用户列表
6. 切换到创建 Tab 显示创建表单（隐藏搜索区域）
7. 已选用户始终显示在 Tab 区域下方，可继续搜索添加（多选场景）

**搜索逻辑**:
- 后端分别模糊匹配 name、email、phone 字段
- 返回匹配的用户列表，每项包含：id、name、email、phone、matched_field、associated
- associated=false 表示该用户尚未与当前商户关联

### 0.2.2 已选用户列表

**列表显示**:
- 用户名
- 邮箱
- 手机号
- 删除按钮（每行）
- associated=false 的用户显示醒目标识

**操作**:
- 点击删除按钮从列表中移除用户
- 单选场景（如指定网点管理员）：只显示一条记录，选中后替换已有选择
- 多选场景（如网点增加成员）：显示多条记录，支持批量选择

### 0.2.3 创建新用户

**触发方式**:
- 点击「创建」Tab，直接显示创建表单（无需额外按钮）

**创建表单**（内联展开）:
- 用户名（必填）
- 邮箱（选填，配置后可支持密码重置）
- 手机号（选填）
- 密码设置：支持手动设密或自动生成（12位随机密码）
- 首次登录强制修改密码开关

**四种创建场景**:

| 场景 | admin 操作 | password | 结果 |
|------|-----------|----------|------|
| A | 手动设密 | 提供 | 用户直接激活，无邮件 |
| B | 自动生成（无邮箱） | 空 | 用户激活，前端展示初始密码 |
| C | 自动生成（有邮箱） | 空 | IAM 生成密码 + 发邮件通知 |
| D | 兼容旧流程 | 空 | IAM 发确认邮件 |

**密码规则**（前后端双重校验）:
- 长度 ≥ 8
- 至少 1 个大写字母
- 至少 1 个小写字母
- 至少 1 个数字

**自动生成算法**: 先保证 1 位数字 + 1 位大写 + 1 位小写，再填充 9 位随机字符（大写+小写+数字），最终 shuffle。

**创建成功后流程**:
- 自动生成密码 → 弹出 Modal 展示初始密码（仅展示一次，关闭后无法查看）
- 手动设置密码 → 直接创建成功
- 邮箱为空时，重置密码按钮灰显

### 0.2.4 确认会话 (Confirmation Session)

**架构决策**: 确认流程委托 IAM 管理，Tuneloop 仅接收回调。

**业务规则**:
- 商户创建时，如指定管理员为新用户，IAM 自动发送确认邮件
- 网点添加成员时，**无需确认**（下级组织仅邮件通知）
- 确认提示（仅在商户创建场景）：
  「管理员需在确认邮件中点击确认链接，才会完成商户创建流程」

**IAM 确认流程**:
1. Tuneloop 调用 IAM API 时传入 `callback_url`
2. IAM 创建确认会话（Redis，TTL=24h），发送确认邮件
3. 用户点击邮件中的确认链接 → IAM `GET /confirm?session={id}&action={accept|reject}`
4. IAM 处理确认后 302 重定向至 `callback_url?result=accept|reject|failed`
5. Tuneloop 回调端点接收重定向，执行本地同步操作

**确认类型 (confirm_type)**:
- `create_user`: 用户 status → active
- `create_org`: 用户 status → active + 完成组织绑定；reject → 组织进入孤儿状态（24h 清理）
- `update_user`: 更新用户邮箱
- `bind`: 完成用户与组织绑定

**本地确认会话（状态跟踪）**:
- Tuneloop 本地 `confirmation_sessions` 表仅用于状态跟踪
- 新增 `iam_session_id` 字段关联 IAM 会话
- 新增 `callback_url` 字段记录回调地址
- 回调时同步更新本地会话状态

**失败处理**:
- 超过24小时未确认 → IAM 自动将状态更新为 expired
- 回调 result=failed → 本地记录失败日志

### 0.2.6 个人密码重置

**触发方式**: 用户在个人中心点击「通过邮件重置密码」

**角色**: 所有已登录用户

**前置条件**: 用户已绑定邮箱（`users.email` 不为空）

**操作流程**:
1. 用户点击「通过邮件重置密码」按钮
2. 弹窗确认：「系统将向您的邮箱 xxx 发送密码重置邮件，邮件中的链接 24 小时内有效」
3. 确认后调用 `POST /api/user/reset-password`
4. 后端检查频率限制：每用户每 30 分钟最多 3 次
5. 后端查本地 `users` 表获取邮箱，验证不为空
6. 后端通过服务认证（client_credentials）调用 beaconiam `POST /api/v1/users/reset-password?user_ids=xxx`
7. beaconiam 创建 ConfirmationSession（ConfirmSetupPassword），发送中文密码重置邮件
8. 用户点击邮件中链接，在 beaconiam 页面设置新密码

**密码重置后 JWT 状态**:
- 现有 tuneloop JWT Token 仍然有效（直到过期）
- 这是 JWT 无状态特性决定的，非 bug
- 如需强制所有会话失效，需 beaconiam 侧支持 Token Revocation List（TRL）
- 用户可主动登出后重新登录

**频率限制**:
- 每用户每 30 分钟最多 3 次
- 超出返回 `42900`：「操作过于频繁，请 30 分钟后再试」

**错误处理**:
- 邮箱为空：「您的账户未绑定邮箱，请联系管理员」
- 发送失败：「邮件发送失败，请稍后重试」

**API 代理端点**:
- `POST /api/user/reset-password` — tuneloop 后端代理转发到 beaconiam
- 不涉及密码输入/存储，仅做代理转发

### 0.2.7 自服务修改密码

**触发方式**: 用户在个人中心点击「修改密码」或首次登录强制改密

**角色**: 所有已登录用户

**操作流程**:
1. 用户在新密码表单填写新密码 + 确认密码
2. 前端校验：8位 + 大写 + 小写 + 数字
3. 调用 `POST /api/user/change-password`（`{ new_password }`）
4. 后端双重校验密码规则
5. 后端通过服务认证调用 IAM `PUT /api/v1/users/:id`（更新 password 字段）
6. IAM 成功后，后端清除本地 `users.force_password_change` 标志
7. 返回成功

**频率限制**: 每用户每 5 分钟最多 3 次

**首次登录强制改密**:
- 创建用户时设置 `force_password_change=true`
- `GET /api/users/me` 返回 `force_password_change` 字段
- 前端 `GET /api/users/me` 检查该标志，true 时重定向到 `/user/change-password?first_login=1`
- 后端 `RequirePasswordNotForceChange` 中间件拦截所有 API（除 `/user/change-password`），返回 40302

### 0.2.5 后端实现要点

**IAM Client 代理层**:
- `GetClientToken()` → IAM Token Endpoint (client_credentials grant) — 仅用于 M2M 场景（回调、定时任务等）
- `ExtractUserToken(c *gin.Context)` → 从请求 cookie 或 Authorization header 提取用户 JWT — 用于用户发起的操作
- 用户发起的操作（创建组织/绑定用户等）使用用户 JWT，确保 IAM 权限检查（Owner/Admin 角色）通过
- M2M 操作（回调处理等）使用 client_credentials
- `CreateOrganization(name, parentID, adminInfo, callbackURL)` → POST /namespaces/:id/organizations
- `CreateOrganizationWithToken(token, ...)` → 同上，使用用户 JWT 认证
- `CreateUser(username, name, email, phone, callbackURL)` → POST /api/v1/users
- `CreateUserWithToken(token, ...)` → 同上，使用用户 JWT 认证
- `UpdateUser(userID, name, email, phone, password, callbackURL)` → PUT /api/v1/users/:id
- `BindUserToOrganization(userID, orgID, role)` → PUT /users/:uid/organizations/:oid/bind
- `BindUserToOrganizationWithToken(token, userID, orgID, role, operatorID)` → 同上，使用用户 JWT 认证
- `UnbindUserFromOrganization(userID, orgID)` → PUT (action=unbind)

**新增 API**:
- `GET /api/iam/users/search?q=xxx&limit=10&merchant_id=xxx` —— 模糊搜索用户，返回关联状态
- `POST /api/iam/users` —— 创建用户（传入 callback_url，移除 skipEmail）
- `PUT /api/iam/users/:id` —— 修改用户（邮箱变更触发 IAM 确认）
- `GET /api/iam/confirmation-callback` —— 接收 IAM 确认回调（302 重定向）

**修改 API**:
- `POST /api/merchants` —— 调用 IAM Create Organization（传入 admin 信息 + callback_url）
- `POST /api/sites` —— 新增调用 IAM Create Organization (parent_id=商户org_id) + IAM 三步绑定管理员
- `POST /api/sites/:id/members` —— 调用 IAM Bind API + SetUserCustomerPermissions + AssignRoleTemplate（三步绑定）
- `PUT /api/sites/:id/members/:uid` —— 调用 IAM UpdateUserRoleInOrg/Bind + AssignRoleTemplate 同步角色变更

**数据模型**:
- `sites.org_id` 语义变更：网点自身的 IAM 组织 ID（非商户的组织 ID）
- `confirmation_sessions` 新增 `iam_session_id`、`callback_url` 字段

### 通用 IAM 绑定三步骤

所有人-组织-角色绑定操作统一遵循以下顺序：

1. **BindUser** — PUT /organizations/:oid/users/:uid/bind (action=bind)，建立 user-org 关系，设置 org role（OWNER|ADMIN|STAFF|WORKER）
2. **SetUserCustomerPermissions** — PUT /organizations/:oid/users/:uid/customer-permissions (raw_bits=true)，按角色模板写个人 cus_perm 位图
3. **AssignRoleTemplateToUserWithToken** — POST /users/:uid/roles (role_ids=[template_id])，分配功能角色模板决定 JWT roles/sys_perm

角色名映射：site_admin→ADMIN, site_member→STAFF, worker→WORKER（兼容旧格式 Manager/Staff）

本地 DB 缓存（site_members/users）在所有 IAM 调用成功后同步写入。

---


---

## 0.3 乐器与订单状态机 (Instrument & Order State Machine)

### Instrument 状态机

乐器自身状态与订单流转状态分离。乐器状态仅反映物理状态：

```mermaid
flowchart TD
  available[可租 available] -- 用户下单 --> reserved[已预约 reserved]
  reserved -- 支付成功 --> rented[租赁中 rented]
  reserved -- 超时/取消 --> available
  rented -- 员工签收归还,完好 --> available
  rented -- 员工签收归还,有损坏 --> maintenance[维修 maintenance]
  maintenance -- 维修完成 --> available
  available -- 经理下架 --> archived[下架 archived]
  archived -- 经理恢复 --> available
  rented -- 物流丢失 --> lost[丢失 lost]
  available -- 经理标记丢失 --> lost
  maintenance -- 无法修复 --> lost
```

| 状态代码 | 中文名 | 说明 |
|----------|--------|------|
| `available` | 可租 | 乐器在库，可供租赁 |
| `reserved` | 已预约 | 订单已创建但未支付，乐器暂时锁定（超时30分钟自动释放） |
| `rented` | 租赁中 | 乐器已租出（含发货/运输/归还途中） |
| `maintenance` | 维修 | 乐器损坏，等待或正在维修 |
| `archived` | 下架 | 乐器已下架，不对外租赁 |
| `lost` | 丢失 | 乐器已丢失（物流丢失或实物灭失） |

### Order 状态机

订单状态覆盖从下单到完成的完整流转：

```mermaid
flowchart TD
  reserved[已预约 reserved] -- 超时未支付 --> cancelled[已取消 cancelled]
  reserved -- 支付成功 --> paid[已支付 paid]
  paid -- 提交发货 --> pending_shipment[待发货 pending_shipment]
  pending_shipment -- 仓库发货 --> in_transit[运输中 in_transit]
  in_transit -- 到达中转站 --> shipped[已发货 shipped]
  shipped -- 用户签收 --> in_lease[租赁中 in_lease]
   in_lease -- 用户申请归还 --> returning[归还中 returning]
   returning -- 仓库验收完好 --> completed[已完成 completed]
   returning -- 仓库验收有损坏 --> pending_damage_response[待回复 pending_damage_response]
   pending_damage_response -- 客户接受 --> deposit_refunding[退款中 deposit_refunding]
   pending_damage_response -- 客户拒绝申诉 --> damage_appealing[申诉中 damage_appealing]
   damage_appealing -- 商户管理员处理 --> deposit_refunding
   deposit_refunding --> completed
   in_lease -- 租约超期 --> expired[超期 expired]
  expired -- 用户归还 --> returning
  expired -- 用户续期 --> in_lease
  reserved -- 用户取消 --> cancelled
  paid -- 用户取消 --> cancelled
  in_transit -- 用户取消 --> cancelled
  in_lease -- 租转售 --> transferred[已过户 transferred]
```

| 状态代码 | 中文名 | 说明 |
|----------|--------|------|
| `reserved` | 已预约 | 订单已创建，等待支付（30分钟超时自动取消） |
| `paid` | 已支付 | 支付已完成，等待发货 |
| `pending_shipment` | 待发货 | 支付完成，准备物流 |
| `in_transit` | 运输中 | 乐器已发出，到达转运站前（用户可取消） |
| `shipped` | 已发货 | 已到达目的地（不可取消） |
| `in_lease` | 租赁中 | 用户已签收，租期内 |
| `returning` | 归还中 | 用户已提交归还，返程物流中 |
| `returned` | 已归还 | 已废弃（#1544），归还验收后直接进入 completed 或 pending_damage_response |
| `damage_appealing` | 申诉中 | 客户拒绝赔偿后进入申诉，等待商户管理员处理 |
| `deposit_refunding` | 退款中 | 赔偿确认/申诉调整后，**员工在订单详情点「退款」**触发差额结算后 → completed |

### 2.1a pending_damage_response（待回复）

| 字段 | 说明 |
|------|------|
| 进入条件 | 仓库验收损坏，员工设定赔偿金额 |
| 分支 | 客户接受 → `deposit_refunding`；客户拒绝 → `damage_appealing` |
| 超时 | 无自动超时——客户**必须**响应 |
| `completed` | 已完成 | 租赁流程全部结束 |
| `cancelled` | 已取消 | 订单已取消 |
| `expired` | 超期 | 租约已过期，可续期或归还；逾期费在归还验收时统一收取（从押金扣除） |
| `transferred` | 已过户 | 租转售完成，乐器所有权转移 |

### 角色可见性

- **用户（顾客）**:
  - 查看乐器列表：除了下架外可见所有乐器，状态只显示"可租"/"不可租"（非 available 即为不可租）
  - 在租赁列表中可看到自己租赁乐器的真实状态
- **员工**:
  - 查看乐器列表：除下架外可见所有乐器及真实状态
- **网点经理**:
  - 查看乐器列表：可见所有乐器（含下架）
  - available/maintenance 状态可切换为 archived，archived 可切换回 available/maintenance
  - 提供开关，可切换查看下级网点乐器（含所属网点列）
- **商户管理员**:
  - 同网点经理，可查看下级网点所有乐器

### 订单可见性

- **用户（顾客）**:
  - 仅可查看自己创建的订单（按 `user_id` 过滤）
- **员工 / 网点管理员**:
  - 可查看所属网点的全部订单（按乐器的 `site_id` 过滤）
  - 不可查看其他网点的订单

---

## 1. 乐器列表
### 1.1 乐器录入

**角色**：租户管理员、网点管理员、网点成员

#### 操作流程

1. **进入乐器录入页面**
   - 从导航栏进入乐器列表 `/instruments`
   - 点击"新建乐器"按钮，跳转至 `/instruments/new`

2. **填写基本信息**
   - **识别码 (sn)**：必填，输入后自动调用后端 API 校验唯一性
   - **乐器分类**：树形下拉框，支持懒加载，点击结点选中，提供链接跳转分类管理
   - **乐器分级**：下拉选择（入门、专业、大师）
   - **归属网点**：
     - 租户管理员：可选任意网点
     - 网点管理员/成员：默认锁定当前所属网点，不可修改

3. **填写附加信息**
   - **描述**：文本输入
    - **动态属性**：根据属性管理中定义的属性，显示对应输入控件
      - 下拉框选择现有值 + 直接输入均可
      - 输入时实时搜索过滤，提供包含输入值的前 3 个常用项
      - 级联选择：选择分类后，category-scoped 属性（如品牌）按分类过滤；选择父属性值后，property-scoped 属性（如型号）按父值过滤
      - 输入已知别名时（如"yamaha"→"雅马哈"），系统自动映射为标准值，无需审批
      - 输入不存在的新值时，该值自动进入"待核定"状态
      - 提供链接跳转属性管理

4. **上传媒体文件**
   - 图片：最多 6 张，支持拖拽上传
   - 视频：最多 1 段
   - 前端先上传媒体文件到服务器，返回文件 UUID
   - 提交表单时将 UUID 附带上

5. **提交或取消**
   - 点击"提交"：创建乐器成功，返回列表页
   - 点击"取消"：直接返回列表页，不保存

#### 权限说明

| 角色 | 创建乐器 | 默认网点锁定 |
|------|---------|-------------|
| 租户管理员 | ✅ | ❌ 可选 |
| 网点管理员 | ✅ | ✅ 锁定当前网点 |
| 网点成员 | ✅ | ✅ 锁定当前网点 |

### 1.2 乐器批量录入

**场景**：租户管理员或网点管理员需要一次性录入大量乐器（如盘点入库）

**角色**：租户管理员、网点管理员

**前置条件**：已准备好 CSV 模板和对应的乐器图片/视频文件

#### 操作流程

1. **下载模板**
   - 进入乐器列表页面
   - 点击"批量导入"按钮
   - 下载 CSV 模板（包含字段：识别码、分类、级别、描述及动态属性列）
   - 参考模板说明填写数据

2. **上传 CSV 校验**
   - 选择已填好的 CSV 文件上传
   - 系统立即解析并在 Grid 表格中展示数据
   - 自动高亮错误行：
     - 识别码与库中重复（红色背景）
     - 识别码在文件内重复（红色背景）
     - 必填项缺失（红色背景）

3. **在线纠错**
   - 双击错误单元格直接修改
   - 无需重新上传文件
   - 修改后自动重新校验

4. **上传媒体文件包（可选）**
   - 上传 ZIP 文件（包含图片/视频，命名格式：识别码_序号.jpg）
   - 系统自动匹配到对应乐器
   - 未匹配的文件显示在"未匹配区"，可修改文件名后重新匹配

5. **确认创建**
   - 点击"确认导入"
   - 系统事务性创建乐器
   - 完成后显示结果：成功 X 条，失败 Y 条及详情

#### 异常处理

- 文件格式错误：提示正确的 CSV 格式
- 识别码重复：阻止导入，提示冲突
- 部分成功：显示成功/失败明细，支持单独重试失败项

### 1.3 游客浏览乐器（新增）

**角色**：游客（未登录用户）

**场景**：用户打开微信前端链接，无需登录即可浏览乐器列表和详情。

#### 操作流程

1. **访问首页（无参数）**
   - 游客打开链接 `/` 或扫描二维码进入
   - 前端调用 `GET /api/public/instruments`（不含 `tenant` 参数）
   - 后端返回**所有租户**的乐器列表
   - 页面展示乐器图片、名称、租金、网点等信息

2. **带租户参数的访问**
   - 游客打开链接 `/?tenant=<tenant_id>`
   - 前端检测 URL 中的 `tenant` 参数并附加到 API 请求
   - 后端仅返回该商户的乐器列表
   - 适用于不同商户的推广链接场景

3. **浏览乐器详情**
   - 点击乐器卡片进入详情页 `/instrument/:id`
   - 调用 `GET /api/public/instruments/:id`
   - 显示完整信息（租金政策、级别选择、网点位置、服务权益对比）

4. **下单入口**
   - 底部"立即租赁"按钮：未登录则跳转登录，已登录则进入结算流程

### 1.4 购物车管理

**角色**：游客 / 已登录用户

**场景**：用户希望一次租赁多部乐器，先收集到购物车再统一下单。

#### 操作流程

1. **悬浮购物车图标**
   - 屏幕右下角悬浮显示购物车图标
   - 图标右上角显示数字角标（购物车中乐器件数）
   - 点击跳转到 `/cart` 购物车页面

2. **加入购物车**
   - 在乐器详情页，若乐器状态为 `available`（可租），底部显示"加入购物车"按钮
   - 若该乐器已在购物车中，按钮显示"已加入"并 disabled
   - 点击"加入购物车"，数据存储到 localStorage `cart` key：
     ```json
     { "items": [{ "id", "instrument_id", "sn", "name", "cover_image", "category_name",
       "daily_rent", "deposit", "shipping_fee", "rent_qty": 30,
       "site_id", "site_name", "site_address", "site_phone",
       "tenant_id", "tenant_name", "level_name" }] }
     ```
   - 弹出"加入成功"Toast，提供两个选项：
     - **继续浏览**：关闭弹窗，留在详情页
     - **去结算**：导航至 `/cart` 购物车页面
   - 悬浮购物车图标数字 +1

3. **查看购物车**
   - 用户访问 `/cart` 页面，从 localStorage 读取购物车数据
   - **三级分组展示**：商户 → 网点 → 乐器
   - 每项显示：复选框、缩略图、SN/名称、级别标签、分类标签、租期调节器（—/N天/+）
    - 每项右侧显示费用明细：租金（日租金×天数）、押金、小计
   - 每项有删除按钮，点击确认后从购物车移除
   - 加载时检查乐器状态，已下架/被租者置灰显示"已失效"，提供"一键清理"

4. **复选框选择**
   - 每项左侧有复选框，默认全选
   - 仅选中项计入**网点小计**和**合计总额**
   - 取消选中后网点小计和合计实时更新
   - 合计为 ¥0 时"去结算"按钮灰色禁用

5. **网点汇总**
   - 每网点显示：发货仓地址、电话
    - 网点小计 = Σ选中项(租金+押金)（物流费在下单时不收取，#1570）

6. **收货地址**
   - 批量下单在结算页（`/checkout`）收集收货地址
   - 每用户可维护多个地址（CRUD 接口 `user/addresses`）
   - 下单时可选既有地址或填写新地址
   - 填写新地址时，默认勾选「设置为我的收货地址」加入地址簿
   - 无论是否勾选保存，地址均作为订单的 `delivery_address` 写盘
   - 各字段：收货人、电话、省、市、区、详细地址、邮编

7. **费用明细**
   - 每件：日租金 × 租期（默认30天）+ 押金（物流费在发货时确定，下单时不收取）
   - 底部合计：Σ选中项(租金+押金)

6. **空购物车**
   - 购物车为空时显示空状态提示
   - 提供"去逛逛"按钮，点击返回首页

### 1.5 批量下单

**角色**：已登录用户

**场景**：从购物车提交多件乐器的租赁订单。

#### 操作流程

1. **下单校验**
   - 用户点击购物车底部"去结算"按钮
   - 系统检查登录状态：
     - 未登录 → 跳转登录页，登录后返回购物车
     - 已登录 → 导航至 `/checkout` 结算页
   - 结算页仅对选中的商品结算（通过 `cart_checkout` 传递）

2. **结算页（Checkout）**
   - 展示商品清单（按商户→网点分组，含缩略图、SN、分类、租期、小计）
   - 选择/填写收货地址（`user/addresses`）
   - 确认后调用 `POST /api/user/orders/batch` 批量创建订单
   - 订单状态为 `reserved`（已预约），乐器库存标记为 `reserved`
   - 创建成功后，从 `cart` 中移除已下单项，清空 `cart_checkout`
   - **自动跳转统一支付页**：`/payment?type=rent&id={order_id}`

3. **统一支付页（Payment）**
   - 调用 `POST /api/pay/calculate { type: "rent", id }` 获取支付详情
    - 展示阶梯定价明细、押金、应付总额（物流费在发货后订单详情中展示，#1570）
    - 支持点数抵扣：赠点（赠点上限 = min(余额, floor(应付×**用户当前级别 pay_ratio**))，比例由赠点策略配置）
    - 现金差额 = 应付总额 - 赠点使用
   - 现金差额 > 0 显示"微信支付"，= 0 显示"确认支付 ¥0（使用点数）"
   - 点击支付 → `POST /api/pay/prepay { order_id, order_type:"rent", amount }`
   - 当前环境（mock=true）：直接显示"支付成功"，跳转 `/success?order_id=...`；付款页同时显示「模拟支付」按钮可跳过微信 JSAPI 调起
   - 生产环境：返回 prepay 参数，调起微信支付 JSAPI

4. **完成页（Success）**
   - 显示：订单号、支付状态
   - "完成"按钮 → 回到首页

5. **我的租赁列表**
   - 处于 `reserved` 状态的订单显示"立即支付"按钮
   - 点击直接跳转统一支付页：`/payment?type=rent&id={order_id}`（不经过订单详情）

### 1.6 登录后购物车合并（新增）

**角色**：游客 → 已登录用户

**场景**：未登录时选的乐器，登录后需要合并到数据库。

1. **登录成功后检查**
   - 登录成功后检查 localStorage cart
   - 若不为空，提示"是否合并未登录时选中的商品？"
    - 确认后合并到数据库（预留 API 接口）

### 1.7 属性管理（命名空间管理员）

**角色**：命名空间管理员（cus_perm: `attribute:manage`）

**权限说明**：属性键（Properties）和属性值（PropertyOptions）是平台级共享资源，所有租户可见，仅命名空间管理员可管理。

#### 1.7.1 属性键管理

1. **创建属性键**
   - 名称（name）、类型（property_type: string/int/float/date/time）、说明（description）、单位（unit）
   - 选择 scope_type：global（与类别无关）、category（与类别相关）、property（与父属性相关）
   - 选择 scope_type=category 时，需关联一个乐器分类
   - 选择 scope_type=property 时，需关联一个父属性键
   - 创建后 scope_type 不可修改

2. **查看属性键**
   - 左侧属性列表：名称、类型、说明
   - 选中后右侧显示该属性下的所有属性值

#### 1.7.2 属性值审批

命名空间管理员在属性值矩阵中可查看所有属性值及其状态。属性值有三种状态：

| 状态 | 标签 | 含义 |
|------|------|------|
| pending | 待核定 | 用户新增，等待审批 |
| confirmed | 已核定 | 审批通过，对所有用户可见 |
| abort | 已废弃 | 已归并或废弃，不再使用 |

三种审批场景：

- **场景一（核定）**：新值合理，直接采用。`PUT /api/property/confirm { property_id, value }` → status=confirmed
- **场景二（归并）**：应使用已有值，如用户输入"yamaha"，要求使用"雅马哈"。`PUT /api/property/merge { property_id, source_value, target_value }` → source.status=abort, source.alias=target.id。已使用该值的乐器自动更新为标准值
- **场景三（修正）**：接受但需改名，如 typo 修正。`PUT /api/property/confirm { property_id, value, new_value }` → 创建 confirmed 新值，原值 status=abort 并设为新值的别名。已使用该值的乐器自动更新

#### 1.7.3 属性管理页面布局

左侧属性列表（名称、类型、操作按钮），右侧属性值矩阵（值、状态、使用频次、操作）。pending 状态的属性值显示三个操作按钮：核定、修正、归并。

#### 1.7.4 别名自动映射

用户创建/编辑乐器时，如果输入的属性值是某个已核定属性值的别名（如"yamaha"是"雅马哈"的别名），后端自动解析为标准值。整个过程对用户透明，无需再次审批。

### 1.8 统一支付页

**角色**：已登录用户

**场景**：统一的支付入口，覆盖租赁支付、维修支付、定损支付、退款等所有支付/退款类型。

#### 路由参数

```
/payment?type={type}&id={id}
```

#### 十个支付类型的信息布局

| # | type | 标题 | 费用明细 | 点数面板 | 按钮文案 |
|---|------|------|---------|:---:|------|
| 1 | `rent` | 租赁支付 | 阶梯定价（tier_segments）+ 押金 | ✅ | 微信支付 / 确认支付 ¥0 |
| 2 | `repair` | 报修支付 | 材料费 + 服务费 + 物流费 | ✅ | 同上 |
| 3 | `requote` | 报修增补差价 | 新总额 - 已付 = 差额 | ✅ | 同上 |
| 4 | `damage` | 定损赔偿 | 已付明细（租金/押金/物流，灰显）+ 损失评估 + 押金抵扣 + 需补付（红） | ✅ | 微信支付 / 确认支付 ¥0 / 无需支付 |
| 5 | `refund` | 退款确认 | 可退现金 + 赠点退回 | ❌ | 确认退款 |
| 6 | `deposit-refund` | 押金退款 | 可退现金 + 赠点退回 | ❌ | 确认退款 |
| 7 | 取消订单退款 | 退款确认 | 同 refund | ❌ | 确认退款 |
| 8 | 结算退款 | 退款确认 | 同 refund | ❌ | 确认退款 |
| 9 | 申诉退款 | 退款确认 | 同 refund | ❌ | 确认退款 |

#### 操作流程

1. **加载支付详情**
   - 调用 `POST /api/pay/calculate { type, id }` 获取数据
   - 返回：`type`, `title`, `amount`, `wallet`, `details`

2. **费用明细展示**（按 type 不同）
   - `rent`：阶梯定价（tier_segments）、租金小计、押金
   - `repair`/`requote`：材料费、服务费、物流费
   - `damage`：已付部分（租金/押金，灰色只读）+ 损失评估金额 + 押金抵扣 + 需补付（红色高亮）
     - 需补付 = max(0, 损失评估金额 - 押金)
   - `points`：不显示费用明细，仅显示充值金额
    - 退款类型（refund/deposit-refund）：可退现金、赠点退回

3. **点数使用**（仅 type=rent/repair/requote/damage 且 应付金额 > 0）
    - 显示赠点余额
    - 用户可输入使用点数（不超过余额）
    - 赠点使用上限 = min(赠点余额, floor(应付金额 × max_gift_ratio))
    - 现金差额 = 应付金额 - 赠点使用

4. **支付执行**
   - 现金差额 > 0：显示"微信支付 ¥{amount}"按钮
   - 现金差额 ≤ 0：显示"确认支付 ¥0（使用点数）"按钮（绿色）
   - type=damage 且 需补付 = 0：显示"无需支付 ¥0"按钮（绿色）
   - 调用 `POST /api/pay/prepay { order_id, order_type, amount }`
   - mock 模式：显示"支付成功"并跳转；付款页显示「模拟支付」「模拟退款」按钮（由 `GET /api/pay/config` 控制显隐）
   - 生产模式：微信 JSAPI 不可用（H5）→ 提示"暂不支持H5支付"；可调用（weapp）→ `Taro.requestPayment`

5. **退款执行**（refund/deposit-refund）
   - 退款金额在进入支付页**之前**已由后端处理（`CancelOrderByCustomer` 或 `ConfirmSettlement` 内部创建 `OrderRefundRecord`）
   - 支付页仅展示退款明细，不执行实际退款
    - **退款优先级**（后端实现）：赠点超cap部分退回 promo_points → 剩余现金退款（#1537）
   - 点击"确认退款"按钮 → toast 提示 → 导航回上一页

6. **进入入口**
   - 购物车结算 → `/payment?type=rent&id={order_id}`
   - 我的租赁列表 '立即支付' → 直接跳转支付页
   - 订单详情 '去支付' → 跳转支付页
   - 维修报价 '确认支付' → `/payment?type=repair&id={requestId}`
   - 定损接受 → `/payment?type=damage&id={order_id}`
   - 取消订单 → 后端调 `cancel-by-user`，前端检测 `refund_amount > 0` → `/payment?type=refund&id=...`

### 1.9 定损响应与退款

**角色**：已登录用户、商户管理员

**场景**：仓库验收有损坏，员工设定赔偿金额后，客户必须响应（接受或拒绝）。

#### 流程（#1544）

```
仓库验收有损坏 → 员工设定赔偿金额 → 系统通知客户
→ 订单进入 pending_damage_response（客户必须响应）

场景 A：客户接受
  客户 → 接受赔偿 → 订单 → deposit_refunding
  → 系统通知员工（含订单链接）
  → 员工订单详情点「退款」→ 差额结算 → completed

场景 B：客户拒绝
  客户 → 拒绝并填写申诉理由 → 订单 → damage_appealing
  → 网点或商户管理员（PC 端）处理申诉 → 可调整赔偿金额（可选）
  → 通知客户与员工 → 订单 → deposit_refunding
  → 员工订单详情点「退款」→ 差额结算 → completed
```

#### 关键变化 vs 旧流程

| 项 | 旧 | 新 |
|----|----|-----|
| damaged 后状态 | returning（客户可不响应） | pending_damage_response（**必须响应**） |
| 客户接受 | AgreeDamage → completed/deposit_refunding | /appeals/:id/agree → deposit_refunding → **员工点退款** → completed |
| 客户拒绝 | 可选 Appeal | POST /appeals → 申诉单 → damage_appealing |
| 申诉处理者 | 平台管理员 | **网点或商户管理员**（可改赔偿额） |
| 退款触发 | InspectReturn(good)自动 / 顾客退款确认页 | **员工在订单详情点「退款」**（deposit_refunding 状态） |

#### 支付页信息布局

```
┌─────────────────────────────────────┐
│  定损赔偿确认                        │
├─────────────────────────────────────┤
│  租金小计        ¥1,050.00   灰色    │
│  押金            ¥245.00     灰色    │
│  物流费          ¥100.00     灰色    │
│  ─────────────────────────          │
│  损失评估        ¥180.00            │
│  押金抵扣        -¥180.00           │
│  需补付          ¥0.00      红色    │
├─────────────────────────────────────┤
│  赠点余额         ¥20.00            │
│  使用赠点         [____]            │
│  现金差额         ¥0.00             │
├─────────────────────────────────────┤
│  [无需支付 ¥0] 绿色                 │
└─────────────────────────────────────┘
```

- `需补付 = max(0, 损失评估金额 - 押金)`
- 需补付 = 0 → 绿色按钮"无需支付 ¥0"（直接确认，无需调支付接口）
- 需补付 > 0 → 显示点数面板 + "微信支付 ¥X"

## 2. 租赁闭环

### 2.1 库存管理&租金设定
网点经理登录
从右侧导航栏的库存监控进入管理界面，可看到库存中的乐器列表（识别码、分类、级别、品牌、型号、网点、租金基准）
可设置按品牌、型号、类别、级别筛选乐器
最右列为租金基准，以货币输入框显示每件乐器的日租金、押金、逾期日费，可修改
当经理做了修改，页面上的『保存』按钮就激活，点击就批量完成租金设定

### 2.2 乐器租赁 

用户打开小程序界面可以看到乐器列表，有日租金说明、可以通过类别、网点、级别、可租状态筛选
点选乐器，进入乐器详情，
- 可以看到乐器最新一批图片
- 可以看到品牌、型号、简介
- 可以看到日、周、月租金、押金说明
  - 费用计算公式：费用 = 单期费用 × 期数 + 押金
  - 日租金 = instrument.pricing.daily_rent（对象格式；`base_daily_rate` 与阶梯首档同源）
  - 周租金 = instrument.pricing.weekly_rent（未定义时使用 daily_rent × 6 作为回退）
  - 月租金 = instrument.pricing.monthly_rent（未定义时使用 daily_rent × 25 作为回退）
- 押金：后台设定（乐器归还、质检通过后原路退还，损坏则定损抵扣）
- 物流费：发货时由员工填写（#1541）
- 逾期日费：后台设定（默认等于日租金），逾期后每日自动扣款
- 可以点击下单，选择租期类型（按日/周/月）、数量，确认收货地址，跳转到支付界面
- 完成支付，乐器进入预订状态
- 系统
  - 生成发货通知
  - 系统自动生成一张电子合同或收据（PDF 格式），存入用户的“我的资料”中，作为租赁凭证
租赁期间，用户进入『我的』，
- 可以看到租赁会话列表（乐器类别、到期时间）
- 点击可以查看订单详情
租赁期满，用户进入『我的』，
- 点击期满的租赁会话，在租赁详情中点归还
- 输入物流信息，乐器进入归还状态

### 2.3 库管
员工在PC端登录
检查预订状态的订单列表，安排物流
- **发货前拍照留档**：员工在发货前，按乐器分类对应的拍照要求对乐器进行拍照
  - 拍照要求由商户级配置（当前使用默认占位数据）
  - 每次拍照上传至 instrument_media 表（batch_type='shipping'），同一天同一乐器的员工拍照归为同一 batch_id
  - 系统保留最近一次员工拍照的照片，用于归还验收时对比
- 交付物流后，将物流信息填写在订单上，乐器进入发货状态
每天定时检查发货的订单列表
- 发货状态的乐器物流到达后进入租赁状态，以物流到达时间点为起租点
归还的乐器到货后
- 扫码乐器上的二维码，可以看到乐器相关信息和租赁信息
- 按规定对乐器拍照，照片会上传到服务器
- 若乐器没有损坏，则点击『归还』恢复为在库状态
  - 系统自动生成退还押金事务
- 若乐器损坏，
  - 点击定损，输入评论，金额，点击提交
  - 乐器进入维修状态、创建维修会话待分配状态

## 2.4 申诉
用户小程序上，在有损坏的情况下，
- 收到定损通知（包括照片、评论、金额）
- 点击『同意』，
  - 如果押金足以覆盖赔付，则系统自动扣除定损金额后生成退还押金事务
  - 如果押金不足以覆盖，则进入支付页面
  - 如果支付失败可重试，如果超时未完成则按申诉处理，系统记录申诉
- 点击『申诉』，输入理由，提交。系统记录申诉，乐器进入待处理状态
网点经理可通过查看申诉列表：
- 相关乐器基本信息、租金、当前图片
- 用户、员工信息，员工的定损说明和用户的申诉理由
- 租赁过程
网点经理可以
- 点击『无损坏』，取消赔款，直接生成退还事务，乐器进入在库状态
- 调整定损金额
- 输入说明
- 点击『确定』，乐器进入维修状态。若押金扣除赔款后>0则系统会自动生成退还押金事务。

### 2.5 逾期扣款生命周期

#### 状态转移

```
到期日 D     D+1 任意时刻
   │              │
   ▼              ▼
 in_lease ─── expired（仅改状态，不扣款）
```

关键规则：
- **状态转移**（任何时刻）：`in_lease` 且 `end_date < today` → 自动转移为 `expired`，仅改状态不扣款
- **停用每日自动扣款**：不再有每天 01:00 的自动扣款，不再产生 `overdue_charges` 挂账记录
- **逾期费收取时机**：逾期费在**归还验收时统一收取**（见 §2.7），按 `overdue_daily_fee`（`instruments.pricing` JSONB）或默认 `1.5 × 基准日租金` 计算，从押金扣除
- **极端欠费**（押金不足以覆盖逾期费+定损）：线下处理，不挂账、不自动告警
- 历史 `overdue_charges` 数据保留（查询兼容），新流程不再写入

### 2.6 续期

#### 触发条件

顾客在订单详情页（`in_lease` 或 `expired` 状态）看到续期按钮，点击进入续期页面。

#### 续期定价

**从逾期日起算，连续覆盖**。续期从**原到期日**（`end_date`）起算，逾期日并入续期阶梯计费（不再单独收逾期费）：

```
例：原租期 10 天（1-10 日，end_date=10 日），15 日续租 20 天

逾期日 = 11 日（end_date+1）起
续期覆盖 11-30 日 → 新 end_date = 30 日 = end_date + 20 天

1-10天   Tier 1  基价（已消费）
11-30天  Tier 1  20天（延续阶梯，逾期日并入计费，不单独收逾期费）

续费 = 20 × Tier1单价（按已消费天数继续跑阶梯）
```

- **最小续期天数**：逾期时（`today > end_date`）最少续期 `today - end_date`（含端点）天，需覆盖逾期期
- **费用**：`CalculateRenewalPricing` 从已消费天数起算阶梯（现有实现），`total_amount = 续期费`（无逾期费）
- 押金已在首单支付过，续期不重复收；物流费不适用（乐器已到手）

#### 续期流程

1. 用户选择续期天数（少于最小天数时置灰/提示）→ 系统计算费用（仅续期费）
2. 进入支付确认页 → 支付（WeChat Pay JSAPI）
3. 支付成功后：
   - `expired` → `in_lease`，`end_date = 原 end_date + 续期天数`（连续覆盖）
   - 追加订单日志
   - 向顾客发送"续期成功"通知
4. 支付失败：向顾客发通知

### 2.7 归还退款结算（含逾期与提前归还）

#### 变量定义

| 符号 | 含义 | 数据来源 |
|------|------|---------|
| Dd | 定损赔付额 | 定损记录（`damage_reports.deposit_deducted`） |
| De | 押金 | `orders.deposit` |
| Dri | 第 i 阶梯上的实际租借天数 | `pricing_breakdown.tier_segments[i].days`（按实际归还日截断） |
| O | 逾期费 | 归还验收时计算（`damage_assessments.overdue_fee`，#1493） |
| Re | 应付租金总额 | `Σ Dri × 基准日租金 × 阶梯折扣`（按实际使用天数） |
| Ra | 实付租金总额 | 所有支付记录之和（不含押金、物流费） |
| R | 应退金额 | `Ra + De - Dd - O - Re`（最小为 0） |

#### 计算步骤

1. 从 `pricing_breakdown.tier_segments` 读取各阶梯信息（`Dri`, 日租金, 折扣率）
2. 按实际归还日截断各阶梯天数（`actual_days`）
3. 对每个阶梯 i：`应付租金_i = Dri × 基准日租金 × 阶梯折扣率`
4. 汇总：`Re = Σ 应付租金_i`
5. 逾期费 `O`：归还验收时计算（`overdue_days × overdue_daily_fee`），从押金扣除
6. 应退金额：`R = Ra + De - Dd - O - Re`（最小为 0）
7. **提前归还退费**：实付租期 > 实际使用天数时，`early_return_rebate = Ra - Re`（按阶梯折算退回）
8. **退款顺序**（#1537）：赠点超 cap 部分先退回 `promo_points` → 剩余现金退回 `order_refund_records`

#### 分阶段流程（#1494）

**阶段1（顾客点归还）**：`ReturnOrder` 更新订单为 `returning`，前端跳转结算通知页（预估明细 + 感谢语，**不立即退款**）
**阶段2（网点验收+定损）**：`InspectReturn` 计算超期费并持久化到 `damage_assessments`；damaged 时走申诉 → `ResolveAppeal`/`AgreeDamage`
**阶段3（最终退款）**：订单进入 `completed` 时**自动触发**（#1537）：
  - 引擎：`computeSettlement` 计算实际开销（实际租期 × tier 阶梯 + 损坏赔偿）→ 退款 = 付款 - 开销
  - 退款顺序：赠点超 cap 部分 → 剩余退现金
  - 创建 `settlements` + `points_transactions`，`deposit_refunded=true`
  - 触发路径：`InspectReturn`(good)、`ResolveAppeal`/`AgreeDamage`(completed)、damage 支付回调
  - `ConfirmSettlement`（手动）与自动结算**统一走同一引擎**，避免双轨分歧

#### 示例

```
押金 De = ¥3000
定损 Dd = ¥200
阶梯: Tier1: 30天 @ ¥50/天 (0%折扣), Tier2: 150天 @ ¥47.5/天 (5%折扣), Tier3: 365天 @ ¥45/天 (10%折扣)
租期: 40天 (Tier1: 30天 + Tier2: 10天)
逾期: 5天 (逾期费 = 5 × ¥75 = ¥375，从押金扣)
实付租金 Ra = ¥1975 (Tier1: ¥1500 + Tier2: ¥475)

应付租金 Re = 30×¥50 + 10×¥47.5 = ¥1500 + ¥475 = ¥1975
应退 R = 1975 + 3000 - 200 - 375 - 1975 = ¥2425
```

> 注：逾期费按归还验收时的一次性计算（`overdue_days × overdue_daily_fee`），从押金扣除；剩余押金（`De - O - Dd`）参与退款。提前归还时（实际天数 < 租期），`Re` 按实际天数折算，`Ra - Re` 为退费。

### 2.8 订单详情页展示原则 — 合同快照 vs 实际结算分离

**订单详情页是下单时的合同快照，不是实际结算页。**

| 页面 | 场景 | 展示内容 |
|------|------|---------|
| OrderDetail / StaffOrderDetail | A: 合同快照（下单收据） | 合同租期、合同阶梯定价、总金额、押金、物流、合计 |
| ReturnSettlement | B: 实际结算（对账退款） | 实际租期、实际应付、合同差额、退款明细 |

**区隔规则**：
1. 合同租期始终来自 `pricing_breakdown.rent_days`
2. 阶梯定价明细始终展示合同版本 —— 这是用户下单时看到的定价
3. 当实际租期 ≠ 合同租期时，订单详情仅附加灰色提示行：「实际租期 X天（提前归还）」，不替换合同数据
4. 实际天数计算：已归还订单用 `CalculateDays(start_date, returned_at)`，忽略时间分量避免时区偏移
5. 所有退/补款逻辑（赠点退回、现金退回）仅在 ReturnSettlement 页展示


# 3. 维修

## 3.1 维修状态机（新设计）

乐器维修采用扁平状态机：
```
待维修 (repair_pending) → 维修中 (repair_in_progress) → 已修复 (repair_completed) → 可租 (available)
                                            ↑                   ↓
                                        验收不通过         验收通过
```

状态基于 instrument 表的 `repair_status` 字段：
- `repair_pending` — 定损后自动设置（替代原 "创建维修会话待分配"）
- `repair_in_progress` — 维修师傅扫码开始后设置，设 `repair_worker_id` 为当前用户
- `repair_completed` — 维修师傅完成维修后设置（需至少一张照片记录）
- 空值 — 乐器不在维修流程中

验收不通过时 `repair_status` 回退为 `repair_in_progress`，维修师傅继续处理。

## 3.2 维修师傅管理

网点经理进入网点管理
可以创建维修师傅账户（输入姓名、电话等）
可以删除维修师傅账户
可以查看维修师傅列表，包括姓名、电话，最近一个月完成的单数
点击进入可以查看每个师傅最近完成的维修订单具体情况

## 3.3 维修流程

### 3.3.1 维修师傅扫码接单
维修师傅用自己的账户登录微信端
扫描乐器二维码，如果乐器状态为待维修（repair_pending），显示乐器信息 + "开始维修"按钮
点击"开始维修"，当前用户成为维修负责人，状态变为维修中

### 3.3.2 维修过程记录
维修过程中师傅可以输入评论、上传照片
维修记录存储在 `repair_records` 表

### 3.3.3 维修完成
维修完成后，师傅点击"完成"按钮（需至少上传一张照片）
乐器进入已修复状态（repair_completed）

### 3.3.4 接手
如果扫描的乐器处于维修中状态但当前用户不是负责人：
- 提示"本乐器由XXX负责处理中，接手吗？"
- 点击"是"则当前用户成为新负责人
- 点击"否"则退出

### 3.3.5 员工验收
网点员工用自己的账户登录微信端
在乐器管理中看到已修复的乐器
- 确认维修质量后，点击"验收通过"，乐器恢复到可租状态（available）
- 如维修不合格，点击"验收不通过"并输入评论，乐器回到维修中状态（repair_in_progress）

### 3.3.6 乐器管理维修入口
网点员工在乐器管理中：
- 待维修乐器 → 显示"开始维修"按钮
- 维修中乐器（当前用户是负责人）→ 显示"维修完成"按钮
- 维修中乐器（其他人负责）→ 提示当前负责人
- 已修复乐器 → 显示"验收通过"按钮（本网点员工可见）

# 3.3 客户报修（v3）

> 详细设计见 `docs/repair.md` §4.2（v3）。v3 核心：**先远程估价+竞价、用户接受并付款后才寄件**（取代旧的"先寄件→到货质检→再报价"流程）。

## 3.3.1 流程概述

报修指用户将自有乐器发往网点维修。两条路径：**全权商户**（直接到网点）与**合作商户**（经中转网点扇出到多个受控网点竞价）。

### 状态机

**全权路径**
```
pending_assessment(待估价) → pending_payment(待付款) → pending_ship(待发送)
  → shipping(已发货) → repairing(维修中) → return_pending(待发回) → returned(已发回) → closed
```

**合作/受控路径**
```
transit_processing(中转处理中) → pending_assessment → pending_payment → pending_ship
  → shipping → transit_in(转入中) → repairing → return_pending → transit_out(转出中) → returned → closed
```

**分支**
```
pending_assessment ──(5工作日未接受任何报价, 到期前24h提醒)──> closed
repairing ──(师傅重新报价·仅一次)──> 接受→补差款→repairing / 拒绝→回退结算→return_pending
returned → appealing → (管理员关闭) → closed
```

### 角色视角

**用户**：查看/创建报修单（识别码 500ms 防抖回填；SN+类型+品牌+型号 定唯一）。
- 选商户：全权→其网点；合作→中转网点（受控网点不可见）
- 待估价：可续传照片/评论/视频；查看**可见报价**（受控仅见报价单号）；**择一接受**→待付款
- 待付款：看计费（赠点），支付→待发送
- 待发送：填物流（系统给收货人；受控给中转网点地址+转入单号，须写入物流留言）
- 维修中重新报价：接受→补差款 / 拒绝→回退（仅付检查费+物流+中转费，多余退款封底0）
- 已发回→确认收货→评价（关联网点+师傅+商户）或申诉

**员工**（全程仅两个动作，其余只读）：网点收货扫码（→维修中）；待发回填发回物流（激活转出单）。

**中转网点员工**（动作独立）：中转处理填中转服务费+中转物流费（扇出受控网点）；实物中转扫单号/拆箱/拍照/重装/转发；申诉人工核查双向脱敏后转受控网点管理员。

**维修师傅**：待估价提交报价单（材料费+服务费+物流费+工期+评论）；维修中拍照/评论/完成；可重新报价一次。报价单仅本网点成员+报修人可见，**跨网点互不可见**；受控情形师傅不见报修人、评论禁含联系方式。

**商户管理员**：PC 端查看下属各网点报修列表（按状态/网点筛选）；处理申诉。

**系统管理员**：设置检查费（系统统一）。

### 费用模型（详见 repair.md §5）

- 报价单（师傅）：材料费 + 服务费 + 物流费(C段) + 工期
- 中转附加（中转员工，受控）：中转服务费 + 中转物流费(B+D段)
- 检查费：系统管理员统一设置，仅中断回退时收取
- 物流四段 `顾客-A→中转-B→受控-C→中转-D→顾客`：A 用户直付，C=师傅物流费，B+D=中转物流费；全权仅返程单程
- 报修**无押金**（自有乐器）

# 4. 组织管理

## 4.1 网点管理

### 4.1.1 网点列表

网点经理登录PC端
从左侧导航栏的组织管理->网点管理进入
左侧显示网点树状列表（懒加载）
可点击展开查看子网点
右上角有『创建顶级网点』按钮

### 4.1.2 新建网点

点击『创建顶级网点』
URL切换到/sites/new
右侧显示新建网点表单
填写网点名称、类型（加盟/直营/合作店）、地址、联系电话
**指定网点管理员**: 表单内嵌用户搜索框 + 「创建新用户」虚线按钮（见 §0.2），单选模式
- 选中后显示管理员姓名和邮箱，可点击「更换」重新选择
- 初始角色默认为 `site_admin`
提交前检查网点名是否重复
**后端动作**:
1. 调用 IAM `POST /api/v1/namespaces/:id/organizations` 创建下级组织（parent_id = 所属商户的 org_id）
2. 存储返回的 org_id 到 site 记录（网点自身的 IAM 组织 ID）
3. 执行 IAM 三步绑定（管理员角色 = site_admin）：
   a. PUT /organizations/:oid/users/:uid/bind (action=bind, role=ADMIN)
   b. PUT /organizations/:oid/users/:uid/customer-permissions (raw_bits=true, cus_perm=site_admin 模板值)
   c. POST /users/:uid/roles (role_ids=[site_admin_template_id]) — 分配角色模板
4. 本地 `site_members` 表同步创建成员记录
提交成功后左侧网点树自动更新并选中新建网点

### 4.1.3 查看网点详情

点击网点树节点
URL切换到/sites/:id
右侧显示网点详情（名称、类型、地址、电话、负责人）
负责人作为链接可点击跳转至/staff/:id

### 4.1.4 编辑网点

在详情页点击『编辑』按钮
URL切换到/sites/:id/edit
右侧显示编辑网点表单（可复用新建表单）
提交后返回详情页，左侧网点树同步更新

### 4.1.5 网点人员管理

**权限**: 商户管理员或具有租户全局权限的用户

#### 4.1.5.1 人员列表展示

| **用户名** | **角色类别 (Role)** | **操作**        |
| ---------- | ------------------- | --------------- |
| 张三       | 管理员 (Manager)    | 切换角色 / 移除 |
| 李四       | 成员 (Staff)        | 切换角色 / 移除 |

#### 4.1.5.2 角色切换逻辑

- **规则保护**: 若该用户是当前网点**最后一名管理员**，禁止将其切换为成员或删除
- IAM 侧同步（升级/降级均需）：
  a. PUT /organizations/:oid/users/:uid/role — 更新 org role（ADMIN↔STAFF）
  b. POST /users/:uid/roles — 重新分配角色模板（site_admin↔site_member）
- 本地 `site_members` 表同步更新角色字段

#### 4.1.5.3 增加成员

- 成员列表上方内嵌用户搜索框 + 「创建新用户」虚线按钮（见 §0.2），多选模式
- 选中用户后显示在已选列表中，可继续搜索添加
- 点击「确认添加」后，执行 IAM 三步绑定：
  1. PUT /organizations/:oid/users/:uid/bind — 绑定用户到网点，设置 org role（ADMIN|STAFF|WORKER）
  2. PUT /organizations/:oid/users/:uid/customer-permissions (raw_bits=true) — 按角色模板写个人 cus_perm 位图
  3. POST /users/:uid/roles — 分配功能角色模板（决定 JWT roles / sys_perm / cus_perm）
- 角色名映射规则：site_admin→ADMIN, site_member→STAFF, worker→WORKER（兼容旧格式 Manager/Staff）
- 下级组织绑定仅需邮件通知，**无需用户确认**，即时生效
- 本地 `site_members` 表同步创建记录
- 初始角色默认为 `site_member`

#### 4.1.5.4 移除成员

- 点击『移除』按钮
- **保护规则**: 最后一名管理员不可移除
- 在 `site_members` 表删除对应记录

### 4.1.6 删除网点

**前置检查**（重要安全校验）：

1. **资产校验**: 调用库存模块接口，检查该网点下 `instruments` 表：
   - 若有 `stock_status = 'available'`（在库）的乐器 → 警告并拒绝删除，提示"请先转移资产"
   - 若有 `stock_status = 'rented'`（在租）的乐器 → 警告并拒绝删除，提示"请先处理在租订单"
   
2. **人员检查**: 若 `site_members` 表中仍有成员 → 提示"请先移除所有成员"

**删除流程**:
- 在详情页点击『删除』按钮
- 系统执行上述校验
- 全部通过时弹出确认对话框
- 确认后调用API删除网点（软删除，状态设为 'closed'）
- 左侧网点树同步移除该节点

---

### 4.1.5 中转网点

中转网点是受控商户与顾客之间的物流和信息隔离层。受控商户的货物通过中转网点转发，双方信息相互不可见。

**创建**：中转网点由系统管理员在顶层组织下创建，复用标准网点管理界面。在网点表单中选择类型为"中转网点"。

**路由配置**：系统管理员在路由配置界面（PC端）为每个受控商户网点指定对应的中转网点。一个受控网点只能对应一个中转网点，一个中转网点可服务多个受控网点。

**角色**：中转网点员工不需要特殊权限——同一 API 对不同角色输出不同粒度的信息（信息混淆，非权限控制）。

| 角色 | 可见信息 |
|------|----------|
| 中转网点员工 | 全量（顾客 + 受控商户信息） |
| 顾客 | 商品调配中/已发货（不暴露受控商户名称） |
| 受控商户员工 | 商品发往顾客/已收货（不暴露顾客姓名） |

**流程**：中转网点在租赁订单、归还、报修三条线中均有独立流程，详见 §3.3 客户报修和 §5.

---

### 4.1.6 微信绑定

**角色**：网点管理员 / 商户管理员

**场景**：管理员在 PC 端为人员生成微信绑定二维码，人员用微信扫码完成绑定，此后可在微信小程序中一键登录。

#### 绑定流程时序图

```mermaid
sequenceDiagram
    actor Admin as 管理员 (PC)
    participant PC as PC 前端
    participant API as Tuneloop API
    participant IAM as Beacon IAM
    actor Staff as 人员 (手机微信)
    participant WX as 微信服务器

    Admin->>PC: 点击「绑定微信」
    PC->>API: POST /api/users/me/wechat-bind
    API->>IAM: 校验当前用户身份
    IAM-->>API: user_id
    API->>API: 生成 bind_token (5min TTL)<br/>存入内存 map
    API-->>PC: {token, qrcode_url}
    PC->>PC: 渲染二维码<br/>值 = origin + /api/wechat-bind/confirm-page?token=xxx
    PC-->>Admin: 显示二维码弹窗
    Admin-->>Staff: 指向屏幕让人员扫码
    PC->>API: 轮询 GET /api/users/me/wechat-bind/{token} (每2秒)

    Staff->>WX: 微信扫一扫

    rect rgb(240, 248, 255)
        Note over Staff, API: OAuth 网页授权流程 (获取 openid)
        WX--)Staff: 打开网页: /api/wechat-bind/confirm-page?token=xxx
        Staff->>API: GET /api/wechat-bind/confirm-page?token=xxx
        API->>API: 检查 token 是否有效 (pending & 未过期)
        API-->>Staff: 302 重定向到微信 OAuth
        Staff->>WX: GET open.weixin.qq.com/connect/oauth2/authorize
        WX-->>Staff: 授权页面
        Staff->>WX: 点击「授权」
        WX-->>Staff: 302 回调 /api/wechat-bind/confirm-page?code=xxx&token=xxx
        Staff->>API: GET confirm-page?code=xxx&token=xxx
        API->>WX: POST /sns/oauth2/access_token?code=xxx
        WX-->>API: {openid, access_token}
    end

    API->>API: 渲染确认页面 (含 openid)
    API-->>Staff: HTML 页面: 欢迎信息 + 确认绑定按钮
    Staff->>API: POST /api/wechat-bind/confirm<br/>{token, wx_openid}

    rect rgb(255, 248, 240)
        Note over API, IAM: 绑定落库
        API->>API: token status → bound
        API->>IAM: PATCH /api/v1/users/{user_id}<br/>{wx_openid}
        IAM-->>API: OK
        API->>API: 更新本地 users.wx_openid
    end

    API-->>Staff: 绑定成功

    PC->>API: 轮询到 status=bound
    API-->>PC: {status: "bound"}
    PC->>PC: 关闭二维码弹窗<br/>显示「已绑定」
    PC-->>Admin: 绑定完成
```

#### 接口清单

| 方法 | 路径 | 端 | 说明 |
|------|------|:--:|------|
| `POST` | `/api/users/me/wechat-bind` | PC | 生成绑定 token（需登录态） |
| `GET` | `/api/users/me/wechat-bind/:token` | PC | PC 轮询绑定状态 |
| `GET` | `/api/wechat-bind/confirm-page` | 微信 | 扫描二维码打开的确认页（OAuth 回调） |
| `POST` | `/api/wechat-bind/confirm` | 微信 | 确认绑定（提交 token + wx_openid） |
| `POST` | `/api/users/me/wechat-unbind` | PC | 解绑微信 |

#### token 生命周期

```
生成 (POST /users/me/wechat-bind) → pending
    ├── 扫码确认 (POST /wechat-bind/confirm) → bound → 删除
    └── 超时 5 分钟 → expired → 定时清理删除
```

#### 关键规则

1. **二维码值**：使用当前 PC 端的 `origin`（`window.location.origin`）+ `/api/wechat-bind/confirm-page?token=...`，不硬编码域名，确保开发/预生产/生产环境均可使用
2. **OAuth 授权**：确认页首次访问无 `code` 参数时，302 重定向到 `https://open.weixin.qq.com/connect/oauth2/authorize`，scope 为 `snsapi_base`（静默授权，无需用户确认即可获取 openid）
3. **token 一次性**：确认后立即标记 bound 并清除，不可重放
4. **轮询频率**：PC 端每 2 秒轮询一次，5 分钟超时自动关闭弹窗
5. **多点绑定**：后绑定的微信号覆盖前一次（IAM 侧 `wx_openid` 写入即覆盖）

## 4.2 人员管理

## 4.2 人员管理

### 4.2.1 人员列表

从左侧导航栏的组织管理->人员管理进入
URL为/staff
主体显示人员列表（姓名、邮箱、电话、归属网点、职位、角色、状态）
支持按姓名和网点搜索
支持分页
姓名可点击查看用户详情

### 4.2.2 创建用户

点击『创建用户』按钮
弹出用户创建对话框
填写用户名、姓名、邮箱、电话、归属网点、职位、角色
归属网点下拉框采用树状展示
点击提交
- 系统���查邮箱/电话唯一性
- 如有冲突，弹出选择对话框列出已有用户
- 可选择"继续创建"或"选择已有用户"
创建成功，对话框关闭，列表刷新

### 4.2.3 编辑用户

点击人员列表中的『编辑』按钮
弹出用户编辑对话框
可修改姓名、归属网点、职位、角色
可修改邮箱和电话：
- 邮箱变更：调用 IAM `PUT /api/v1/users/:id`，传入 `callback_url`，IAM 自动发送确认邮件
- 确认后回调 Tuneloop 更新本地邮箱记录
- 手机号变更：留 Stub（IAM 侧暂不实现）
提交后列表刷新


---

## 附录：权限相关流程总结

### 1. 权限体系架构

TuneLoop 使用 **双层位图** 实现权限控制：

| 层级 | 来源 | 字段 | 用途 |
|------|------|------|------|
| **sys_perm** | IAM 内置位码 (0-24) | JWT 字段 | 控制结构操作：网点管理、人员管理、角色配置等 |
| **cus_perm** | TuneLoop 业务注册 | JWT 字段 | 控制业务操作：乐器 CRUD、库存、订单、维修等 |

**关键概念澄清**：
- sys_perm 和 cus_perm 只是**权限来源不同**，不是级别高低
- site_admin 可以有 sys_perm（看到菜单）+ cus_perm（业务操作）
- **授权范围**由后端 API 的 site_id/org_id 过滤控制（不是前端菜单）

### 2. 角色模板定义位置

**Beaconiam 端** (`internal/models/functional_role_template.go`)：
- 存储角色模板基础信息
- 包含 `cus_perm`（客户权限位图）
- **当前缺失 `sys_perm` 字段**（见 beaconiam Issue #157）

**TuneLoop 端** (`backend/services/role_templates.go`)：
- 定义完整的角色权限映射
- 包含 `SysPermBits`（系统权限位码数组）
- 包含 `CusPermCodes`（客户权限代码数组）

### 3. 权限计算流程（登录时）

```
用户登录
  ↓
Beaconiam /oauth/token
  ↓ 计算最终权限：
  - SysPerm = organization.sys_perm OR user_org_relations.sys_perm OR functional_role_templates.sys_perm
  - CusPerm = organization.cus_perm OR user_org_relations.cus_perm OR functional_role_templates.cus_perm
  ↓
生成 JWT Token（包含 sys_perm, cus_perm）
  ↓
返回给前端
```

### 4. 前端菜单过滤流程

```
用户登录成功
  ↓
App.jsx 解析 JWT：
  - sys_perm = payload.sys_perm || payload.sysPerm
  - cus_perm = payload.cus_perm || payload.cusPerm
  - 从 localStorage 读取 permission_mapping
  ↓
filterMenuByRole() - 角色过滤
  ↓
checkRule() - 权限位过滤
  - 检查 menuRules 中的 sysPermBits / cusPermCodes
  - requireAllGroups: true 要求两个条件同时满足
  ↓
渲染可见菜单
```

### 5. 修改角色权限后的刷新问题

当修改 `backend/services/role_templates.go` 后：
- **已登录用户的权限不会自动刷新**（JWT token 已签发）
- 需要用户**清除 localStorage 并重新登录**
- 如果用户不主动刷新，需要实现 perm_version 机制（见 tunerloop Issues #446, #447 及 beaconiam Issue #156）

### 6. 相关 Issue

| Issue | 仓库 | 标题 | 状态 |
|-------|------|------|------|
| #156 | beaconiam | JWT 权限变更后需手动清除 localStorage，缺乏自动刷新机制 | todo |
| #157 | beaconiam | FunctionalRoleTemplate 缺少 sys_perm 字段 | todo |
| #446 | tuneloop | 后端权限变更时递增 JWT perm_version | todo |
| #447 | tuneloop | 前端 JWT perm_version 检测 | todo |

### 7. sys_perm 位码对照表

| 位码 | 名称 | 控制的菜单 |
|------|------|-----------|
| 0 | namespace_view | 客户端管理 |
| 5 | tenant_view | 商户管理 |
| 10 | organization_view | 网点管理 |
| 11 | organization_list | 网点管理 |
| 15 | user_view | 人员管理 |
| 16 | user_list | 人员管理 |
| 17 | user_create | 人员批量导入 |
| 20 | role_view | 角色管理 |

---
*最后更新: 2026-06-30*
*Model: deepseek/deepseek-v4-flash*

# 5. 中转工作流

> 源自 #1132 设计文档。中转网点是受控商户与顾客之间的物流和信息隔离层。

## 5.1 租赁订单—受控商户发货

```
顾客下单 → 生成订单（收货人=中转网点地址）
       → 生成中转订单（status = dispatching）
受控商户 → 发货留言填中转单号 → 调配中(dispatching)
中转网点 → 收货拆包、拍照记录 → arrived
       → 转包（填写物流公司+单号）→ repacked → 发出 → shipped
顾客端 → dispatching 时显示"商品调配中"
       → shipped 后显示"已发货"
受控商户端 → shipped 后显示"商品发往顾客"
```

## 5.2 租赁订单—顾客归还

```
顾客归还 → 归还界面自动填中转网点地址和电话
       → 提醒在物流留言中填写中转单号
中转网点 → 收货拆包拍照 → 按识别码找到中转订单
       → 重新打包发往受控网点 → 流转中
受控商户 → 收货定损 → 流程结束
```

## 5.3 报修订单—顾客发往受控商户（v3）

> v3 与旧版差异：**报价前置且多受控网点竞价**（非中转网点单选转发）；**单一报修单+双向脱敏**（非另建"真实报修单"）；实物运输在付款后。详见 `docs/repair.md` §4.2。

```
顾客创建报修单 → 选"合作商户" → 选一个中转网点（受控网点不可见）→ transit_processing(中转处理中)
中转网点员工 → 填 中转服务费+中转物流费 → 提交 → 对该中转网点关联的所有受控网点可见 → pending_assessment(待估价)
受控网点师傅（多家竞价，跨网点互不可见）→ 各自远程报价（材料+服务+物流(C段)+工期+评论，评论禁联系方式）
顾客 → 查看报价(仅报价单号) → 择一接受 → 付款(材料+服务+物流+中转服务+中转物流) → pending_ship(待发送)
顾客 → 填物流(收货人=中转网点, 转入单号写入留言, 激活转入单) → shipping(已发货)
中转网点 → 扫转入单号/拆箱拍照/重装/发受控网点 → transit_in(转入中) → 受控网点收货 → repairing(维修中)
```

## 5.4 报修订单—受控商户发回（v3）

```
受控网点师傅 → 维修完成(或中断回退) → return_pending(待发回)
受控网点/员工 → 打包发往中转网点(转出单号写入留言, 激活转出单)
中转网点 → 扫转出单号/拆箱拍照/重装/发顾客 → transit_out(转出中) → returned(已发回)
顾客端 → 确认收货 → 评价(网点+师傅+商户, 展示脱敏) / 申诉(经中转网点人工核查双向脱敏→受控网点管理员)
```

> 中断回退（维修中师傅重新报价被拒）：仅收 检查费+物流(C)+中转服务+中转物流(B+D)，多余退款封底 0，乐器按上方发回路径返还。

---

## 调试模式（DEBUG_MODE=true）

启用后商户管理员和网点管理员在管理后台额外可操作：

### 乐器状态修改
- 乐器详情/列表页 → 点击状态标签 → 下拉修改（可租/租赁中/维护中/库存/报废）
- `PUT /instruments/:id/status`

### 订单修改
- 订单详情页 → 可修改起止时间（start_date / end_date）
- 订单状态（reserved/paid/in_lease/completed 等）
- `PUT /orders/:id/admin-update`

### 角色
- merchant_admin: 本商户
- site_admin: 本网点

---

### 场景：会员注册 — 支付 — 激活

**前置条件**：用户完成注册（PostRegister 创建本地用户 + 生成 ref_code）

**流程**：
1. 注册完成后跳转支付页（`/pages-weapp/payment/index?type=membership&amount=99`）
2. 支付页显示**标准收据**（#1575）：
   - 费用明细：VIP 会员注册 ¥99.00 + 合计 ¥99.00
   - 权益说明：解锁全部乐器租赁服务；获赠 99 赠点（每次最多抵扣租金 {max_pay_ratio}%）
   - 不显示点数抵扣滑块（membership 不适用点数支付）
3. 支付（微信支付 / 模拟支付）→ 回调 `applySideEffects` case membership：
   - 按 `MembershipLevel.MinAmount ≤ 支付金额` 匹配最高等级 → 更新 `users.membership_level_id`
   - 赠点 99 已在注册时（PostRegister `membership_gift_points`）发放，回调不重复赠送
4. 支付成功 toast：「会员已激活，赠点已到账」→ 跳转个人中心

**关键规则**：
- 等级激活条件：`amount >= level.MinAmount`（如 99 元 → 初级/VIP 等级）
- 会员中心显示当前等级（不再是"普通会员"）
- `GET /user/points` 返回当前级别 `pay_ratio`（赠点策略 pay_ratio，供前端展示抵扣上限；旧 PointsPolicy.MaxPayRatio 默认 0.3 已并入赠点策略）

---

### 场景：会员推广二维码

- **前置条件**：用户已登录，ref_code 已生成
- **用户操作**：会员中心 → 点击"获取推广二维码"
- **系统行为**：
  - 后端调用微信 wxacode.getUnlimited 生成小程序码，同时返回 H5 落地链接
  - 前端展示二维码（小程序环境优先展示微信原生码，H5 环境展示普通 QR 码）
- **好友操作**：扫码 → 进入注册页（携带 ref 参数）→ 完成注册
- **结果**：referrals 表记录推荐关系（referrer_id: 推荐人, referee_id: 被推荐人, ref_code, status=registered）

---

### 场景：会员中心 — 编辑资料

- **前置条件**：用户已登录
- **用户操作**：
  1. H5：个人中心 / 会员中心 → 点击「编辑资料」
  2. 微信小程序：个人中心 → 点击头像区域或「✏️ 编辑资料」
- **系统行为**：
  - H5：跳转独立编辑资料页 `/profile/edit`（name/phone/email 可编辑，`PUT /users/me` 保存）
  - 微信小程序：跳转 `/pages-weapp/profile/edit/index`（同字段）
- **结果**：用户资料更新后，个人中心/会员中心即时刷新显示新信息

#### 编辑资料 — 手机号/邮箱冲突

- **前置条件**：用户 A 正在编辑资料；用户 B 已占用手机号 `138XXXX` 或邮箱 `a@b.com`
- **用户操作**：用户 A 将手机号或邮箱改为已被占用的值，提交
- **系统行为**：
  1. 前端 `PUT /users/me { phone, email }` → tuneloop → IAM UpdateUser
  2. IAM 检测到 phone/email 已被另一 active 用户占用 → 返回 409 `phone/email already exists`
  3. tuneloop 透传 40900 给前端，**不更新本地 DB**（#1600）
- **结果**：前端显示冲突提示（如「手机号已被占用」），表单不保存

---

### 场景：商户配置定价策略

- **前置条件**：商户管理员登录 PC 端，已有乐器数据
- **系统入口**：左侧菜单 → 经营策略 → **定价策略**（`/pricing/config`）
- **页面展示**：
  - 当前生效的定价策略（阶梯折扣表 + 押金模式）
  - 若未配置，显示系统默认策略
- **阶梯配置**：
  - 每个阶梯定义：天数上限 + 折扣率（%）
  - 示例：前 30 天 0% 折扣、31-180 天 5% 折扣、180 天以上 10% 折扣
  - 支持添加/删除阶梯，最后一行自动设为"不限"
- **押金模式**：
  - **按原价比列**：押金 = 乐器原价 × 比列（默认 100%）
  - **自定义金额**：在乐器编辑中逐件设置押金
- **用户操作**：修改阶梯/折扣/押金模式 → 点击"保存配置"
- **系统行为**：
  - 写入 `merchant_pricing_configs` 表
  - 此后所有该商户下的乐器，前端展示和结算计算均使用此策略
  - `CalculatePricing(baseDailyRate, totalPrice, config)` 基于阶梯表计算每件乐器的实际日租金
- **结果**：定价策略保存成功，顾客端乐器详情页展示对应阶梯价格

---

### 场景：批量设定乐器租金

- **前置条件**：网点管理员登录 PC 端，已有乐器数据
- **系统入口**：经营策略 → **租金设定**（`/inventory/rent-setting`）
- **页面展示**：乐器列表（识别码、分类、网点、级别、日租金、押金、逾期日费）
- **用户操作**：修改任意乐器的日租金/押金/逾期日费 → 点击保存
- **系统行为**：`PUT /api/inventory/rent-setting/batch` 批量更新选定乐器的定价字段
- **结果**：定价字段立即生效，顾客端按新价格展示

---

## 完整退货-定损-申诉-退款用例

> 来源：#1600 审核及重构。覆盖归还→验收→good/damaged分流→申诉→终审→退款全链路。

### 路径总览

```
顾客点"归还"（in_lease/expired）
  → 填写物流公司+单号+归还拍照
  → POST /orders/:id/return → 订单 → returning
  → 结算预览页（ReturnSettlement）：
      - 显示预估结算明细（租金、押金、物流费，醒目标记"等待验收"）
      - 此时不退款

网点员工"接收"（returning）
  → InspectReturn — 定损面板（无损坏/有损坏+拍照+备注）
  ├── 路径 A：good（无损坏）→ 订单 → completed
  │      → 差额结算退款（含物流费扣除、逾期费、赠点分账，见「退款差额结算与返点」）
  │      → 发完成通知（标准收据 + 感谢 + 赠点到账 + 会员中心链接）
  │      → 顾客看到退款明细，"已退款"状态
  │
  └── 路径 B：damaged（有损坏）→ 订单 → pending_damage_response
         → 发系统通知给顾客（actionType=damage_accept_reject，
            ActionData 含 damage_amount/deposit/order_id）
         → MessageDetail 渲染"接受"/"拒绝"按钮
         │
         ├── B1：顾客接受
         │      → POST /appeals/:id/agree
         │      → 订单 → deposit_refunding
         │      → 系统通知员工（含订单链接）
         │      → 员工在消息中心打开通知 → 点击链接跳订单详情
         │      → 员工点击「退款」→ POST /orders/:id/refund
         │      → 执行差额结算（赠点/现金分账，见「退款差额结算与返点」）
         │      → 订单 → completed → 跳转支付页退款收据
         │      → 完成通知（收据 + 感谢 + 赠点到账 + 会员中心链接）
         │
         └── B2：顾客拒绝（申诉）
                → 填写申诉理由 → POST /appeals
                → 订单 → damage_appealing
                → 通知网点管理员（actionType=repair_request）
                │
                └── 网点/商户管理员处理申诉
                       → PC 端 /appeals → 可调整赔偿金额（可选）→ 提交
                       → ResolveAppeal（终审）→ 订单 → deposit_refunding
                       → 两条通知：
                          ① 顾客：终审结果 + 收据预览
                          ② 员工：终审完成 + 订单链接
                       → 员工点链接跳订单详情 → 点击「退款」
                       → POST /orders/:id/refund → 差额结算
                       → 订单 → completed → 支付页退款收据 → 完成通知
```

### 退款通知格式（标准收据）

> 适用于路径 A（good 验收）和路径 B1/B2（终审后）的退款通知。收据明细直接写入通知 Content。

```
┌──────────────────────────────────────────┐
│  租赁结算明细                              │
│                                          │
│  乐器：{SN}（{分类}）                      │
│  实际租期          {N} 天                 │
│  ────────────────────────────             │
│  租金              ¥{rent}               │
│  物流费            ¥{shipping_fee}        │
│  逾期费            ¥{overdue_fee}         │
│  损坏赔偿          ¥{damage_amount}        │
│  续期费用          ¥{renewal_total}       │
│  ────────────────────────────             │
│  应付合计          ¥{total_charged}        │
│  其中：赠点抵扣     {gift_used} 点         │
│       现金应付     ¥{cash_payable}        │
│  ────────────────────────────             │
│  已收（含押金）     ¥{total_paid}          │
│  押金退还          ¥{deposit_refunded}     │
│  ────────────────────────────             │
│  退回赠点          {gift_refunded} 点      │
│  退回微信          ¥{cash_refunded}        │
│  实际退款合计      ¥{actual_refund}        │
│  ────────────────────────────             │
│  返点赠点到账      {rebate_points} 点      │
│  前往会员中心查看 → /membership            │
└──────────────────────────────────────────┘
```

各行的取值逻辑：
- **租金**：实际天数 × 阶梯定价（与 §2.7 `computeSettlement` 同算法）
- **物流费**：`order.shipping_fee`，无则为 0（不显示行）
- **逾期费**：`damage_assessments.overdue_fee`（good 验收时算），无则为 0
- **损坏赔偿**：damaged 验收时 `req.DamageAmount`（申诉终审后为 adjust 金额），good 时为 0
- **续期费用**：续期支付记录的 SUM(amount)，无续期为 0
- **已收**：所有支付记录 SUM（租金+押金+物流费预收+续期）
- **赠点抵扣**：`A1 = floor(应付租金 × 当前级别 pay_ratio)` 与实付 `A0` 取小
- **现金应付**：`C1 = 应付合计 - A1`（无押金参与时）；押金参与时另行扣减
- **退回赠点**：`A0 - A1`（A1 < A0 时），退回 `users.promo_points`
- **退回微信**：`C0 - C1`，走微信原路退款
- **实际退款合计**：`已收 - 应付合计`（最小 0）= 退回赠点 + 退回微信
- **返点赠点到账**：`A2 = floor(C1 × 当前级别 refund_ratio)`，写入 promo_points + points_transactions

### 关键规则

| 规则 | 说明 |
|------|------|
| 物流费何时支付 | **归还时填写物流单号，但费用计入结算**——不单独收费，从押金里统一扣 |
| good 验收后 | **自动执行差额结算退款**（含物流费/逾期费/赠点分账），发收据通知 |
| damaged 验收后 | **不退款**——需顾客先响应（接受/申诉），响应后由员工触发退款 |
| 顾客接受后 | 订单 → deposit_refunding → 通知员工 → 员工订单详情点「退款」→ 差额结算 |
| 申诉终审后 | ResolveAppeal → deposit_refunding → 双通知 → 员工订单详情点「退款」→ 差额结算 |
| 退款通知 | 所有退款通知均包含标准收据明细（含赠点分账行 + 返点 + 会员中心链接） |
| 关单与累计 | 退款执行后订单 `completed`（已完成/done），`total_spending` 按 **C1**（实付现金）累计 |
| 结算预览页 | ReturnSettlement 显示"等待定损，非最终费用"醒目提示 |

### 退款差额结算与返点（赠点策略）

> 统一定义：付款时 `R0 = C0 + A0`（现金 + 赠点抵扣）；退款时按调整后应付 `R1` 重算。

#### 付款侧（初次付款 + 续费通用）
- 赠点抵扣上限：`A0 ≤ floor(应付总额 × 用户当前级别 pay_ratio)`
- 比例来源：**赠点策略**（gift_policies 表，namespace_admin 在 PC「系统管理 → 赠点策略」按会员级别配置）
- 支付快照必须落库 `pay_ratio`（PointsPolicySnapshot 仅存 scope 是缺陷，须修复）

#### 退款侧差额结算
- 调整后应付：`R1`（含阶梯折算/逾期费/损坏赔偿/续期）
- 重算赠点上限：`A1 = floor(R1 × 当前级别 pay_ratio)`
- 分账规则：
  - `A1 < A0`：退 `A0−A1` 回赠点账户；退 `C0−C1` 回微信（`C1 = R1 − A1`）
  - `A1 ≥ A0`：赠点不退（仍为 A0）；退现金 `C0 − (R1 − A0)`
  - 守恒校验：`退赠点 + 退现金 = R0 − R1`
- 支付回调若用了赠点，必须同步扣减 `cash_paid`（防结算双计）

#### 退款后动作
1. 订单状态 → `completed`（已完成/done）
2. `total_spending += C1`（**实付现金口径**——行业惯例按实付计成长值，防赠点循环放大）
3. 发放返点：`A2 = floor(C1 × 当前级别 refund_ratio)` → promo_points + points_transactions
4. 完成通知：完整收据 + 感谢语 + 赠点到账（退回 + 返点）+ 会员中心链接

#### 累计花销口径决策（R1 vs C1）
**采用 C1（实付现金）**：
- 行业惯例：航司里程/信用卡积分/电商成长值均按实付金额累计
- 防循环：若按 R1（含赠点面值），「用赠点→累计消费→升级→返更多赠点」形成复利放大
- 返点基数同为 C1，与累计口径统一

---

