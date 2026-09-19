# 订单-租赁状态机标准文档

> 本文档定义 tuneloop 订单、乐器、租赁会话的**标准状态机**——即"期待的样子"。
> 后续所有修复和新增功能均以此文档为准。

---

## 一、订单状态 (Order.Status)

### 1.1 状态定义

| 状态 | 含义 | 进入条件 | 可执行操作 |
|------|------|---------|-----------|
| `in_store` | 在库（可租） | 新乐器入库 / 验收通过 / 取消 / 终止 / 维修完成 | 预约下单 |
| `reserved` | 顾客已下单，等待支付 | 顾客点击"立即租赁" | 支付、取消 |
| `paid` | 已支付，等待发货 | 支付成功 | 发货、取消 |
| `shipped` | 已发货，运输中 | 网点员工录入物流 | 确认收货 |
| `in_lease` | 租赁中（乐器在用户手中） | 确认收货 | 归还 |
| `returning` | 归还中（运输中） | 用户发起归还 | 验收（通过/损坏） |
| `maintenance` | 维修中 | 验收发现损坏 | 定损、修复 |

**注意**：`cancelled`、`terminated`、`completed` 不再独立存在——三者在语义上均等价于"乐器回到可租状态"，统一用 `in_store` 表示。

### 1.2 状态跳转图

```
┌──────────────────────────────────────────────────────────┐
│                      订单生命周期                          │
│                                                          │
│  顾客预约 ┌──────────┐  支付  ┌──────────┐  发货 ┌──────────┐ │
│  ──────► │ reserved │ ────► │   paid   │ ────► │ shipped  │ │
│          └────┬─────┘       └────┬─────┘       └────┬─────┘ │
│               │                 │                   │       │
│               │ 取消            │ 取消               │ 确认收货│
│               ▼                 ▼                   ▼       │
│          ┌──────────────────────────────────────────────┐   │
│          │                in_lease                       │   │
│          │            （乐器在用户手中）                    │   │
│          └────────────────────┬─────────────────────────┘   │
│                               │                             │
│                               │ 归还                        │
│                               ▼                             │
│                          ┌──────────┐                       │
│                          │returning │                       │
│                          └────┬─────┘                       │
│                               │                             │
│                          ┌────┴─────┐                       │
│                          │          │                       │
│                       通过        损坏                       │
│                          │          │                       │
│                          ▼          ▼                       │
│                    ┌──────────┐ ┌───────────┐              │
│                    │ in_store │ │maintenance │              │
│                    │  (在库)   │ │  (维修中)  │              │
│                    └────┬─────┘ └─────┬─────┘              │
│                         ▲             │                     │
│                         │   维修完成    │                     │
│                         └─────────────┘                     │
│                         ▲                                   │
│                         │ terminated（管理员）               │
│                    ┌────┴─────┐                            │
│                    │ 任意状态   │                            │
│                    └──────────┘                            │
└──────────────────────────────────────────────────────────┘
```

**闭环**：`in_store` 可被顾客预约进入 `reserved`，形成完整的租赁循环。

### 1.3 状态门（Guard）

每个跳转必须校验前置状态：

| 跳转 | 允许的前置状态 |
|------|-------------|
| in_store → reserved | in_store |
| reserved → paid | reserved |
| paid → shipped | paid |
| shipped → in_lease | shipped |
| in_lease → returning | in_lease |
| returning → in_store | returning（验收通过） |
| returning → maintenance | returning（验收损坏） |
| reserved → in_store | reserved（取消） |
| paid → in_store | paid（取消） |
| maintenance → in_store | maintenance（维修完成） |
| * → in_store | 任意状态（管理员终止） |

---

## 二、乐器库存状态 (Instrument.StockStatus)

### 2.1 状态定义

| 状态 | 含义 | 对应订单状态 |
|------|------|-----------|
| `available` | 在库，可租 | in_store |
| `reserved` | 已被预订 | reserved |
| `shipping` | 运输中（发往用户） | shipped |
| `rented` | 租赁中 | in_lease |
| `returning` | 归还途中 | returning |
| `maintenance` | 维修中 | maintenance |
| `archived` | 已归档/已售 | —（租转售终态） |

### 2.2 状态跳转

```
available ──→ reserved ──→ shipping ──→ rented ──→ returning ──┬──→ available (验收通过)
    ▲                              ▲                           │
    │                              │                           └──→ maintenance (验收损坏)
    │      取消(in_store) ─────────┘                               │
    │                                                              │
    ├──────────── 维修完成 ◄─── maintenance ◄──────────────────────┘
    │
    └──────────── 管理员终止（任意订单状态 → in_store）
```

---

## 三、租赁会话状态 (LeaseSession.Status)

### 3.1 状态定义

| 状态 | 含义 | 对应订单状态 |
|------|------|-----------|
| `active` | 租赁进行中 | reserved → in_lease |
| `return_requested` | 已申请归还 | returning |
| `completed` | 租赁结束 | in_store |

### 3.2 状态跳转

```
active ──→ return_requested ──→ completed
```

---

## 四、可见性矩阵

### 4.1 按角色 × 按状态的可见范围

| 操作/信息 | 顾客（本人） | 全权商户网点员工 | 全权商户管理员 | 命名空间管理员 |
|-----------|:-----------:|:------------:|:-----------:|:----------:|
| **reserved 状态** | | | | |
| 订单详情（本人） | ✅ | ❌ | ✅ | ❌ |
| 订单中的乐器 | ❌（不再是 available） | ✅ | ✅ | ❌ |
| 租赁人姓名 | 本人 | ✅ | ✅ | ❌ |
| 租赁人电话/邮箱 | 本人 | ✅ | ✅ | ❌ |
| **in_lease 状态** | | | | |
| 订单详情（本人） | ✅ | ❌ | ✅ | ❌ |
| 乐器状态（rented） | ✅ 可查看 | ✅ 可见 | ✅ | ❌ |
| 租赁人姓名 | 本人 | ✅ | ✅ | ❌ |
| 收货地址 | 用户填的 | ✅ | ✅ | ❌ |
| **returning 状态** | | | | |
| 归还地址 | 网点地址 | — | — | — |
| **受控商户（所有状态）** | | | | |
| 租赁人姓名/电话/邮箱 | 本人 | ❌ 隐藏 | ❌ 隐藏 | ❌ |
| 收货地址 | 转发网点地址 | 转发网点地址 | 转发网点地址 | ❌ |
| 转发会话 ID | ❌ | ✅ | ❌ | ❌ |
| **转发网点员工（跨商户）** | | | | |
| 转发会话表 | ❌ | ✅ | — | ✅ |

### 4.2 可见性规则说明

1. **命名空间管理员不涉及具体业务**：不可见乐器、订单等业务对象
2. **乐器在 in_lease 状态时对网点员工可见**：员工需要知道乐器去向（哪个用户、多长时间）
3. **全权商户**：网点员工可看到租赁人姓名，用于客户服务
4. **受控商户**：网点员工不可看到租赁人信息，只有转发网点地址和转发会话 ID
5. **顾客本人**：永远只看到自己的订单
6. **转发网点员工**：可查看所有关联的转发会话表，用于查找真实用户信息

---

## 五、受控商户——转发流程

受控商户的订单状态流转与全权商户**完全一致**，差异在于地址和可见性。

### 5.1 核心差异

| 维度 | 全权商户 | 受控商户 |
|------|---------|---------|
| 订单状态机 | §1 标准流 | **完全相同** |
| 乐器状态机 | §2 标准流 | **完全相同** |
| 租赁会话 | §3 标准流 | **完全相同** |
| 收货地址（发货方向） | 用户填写 | 转发网点地址 + 电话 |
| 归还地址（收货方向） | 网点地址 | 转发网点地址 + 电话 |
| 用户信息 | 商户员工可见姓名/电话 | 商户员工**不可见** |
| 附加表 | — | `forwarding_sessions` |

### 5.2 发货方向流程

```
用户下单租赁受控商户乐器：
1. 正常创建 Order（status=reserved）+ LeaseSession
2. 同步创建 forwarding_session 记录：
   - lease_session_id = 当前 LeaseSession
   - merchant_id = 受控商户
   - forwarding_site_id = 转发网点
   - direction = "outbound"
   - session_code = 6 位短码（供查找）
3. 受控商户网点员工看到的订单：
   - 收件人 = 转发网点地址 + 电话
   - 不显示真实用户姓名/电话/邮箱
   - 要求员工在物流单上标注订单号（lease_session_id）

受控商户网点员工发货：
4. 填写物流信息 → Order.Status = shipped
5. 物流单上标注 lease_session_id

转发网点员工收货：
6. 根据物流单上的 lease_session_id 查找 forwarding_session
7. 拆封包裹 → 拍照留底
8. 根据 forwarding_session 找到 LeaseSession.DeliveryAddress（真实用户地址）
9. 重新打包 → 发往真实用户
10. forwarding_session.status = forwarded

用户确认收货：
11. Order.Status = in_lease（无变化）
```

### 5.3 收货方向流程（归还）

```
用户归还：
1. 用户发起归还 → Order.Status = returning
2. 系统自动创建 forwarding_session（direction="return"）
3. 生成 session_code（6 位短码）
4. 归还界面展示：
   - 转发网点地址 + 电话
   - 提示：「请在物流单上填写单号 {session_code} 并将乐器寄出」
5. 用户提交物流信息

转发网点员工收货：
6. 根据物流单上的 session_code 或订单号查找 forwarding_session
7. 如果 session_code 不匹配 → 按乐器 ID 标签查找（双保险）
8. 拆封 → 拍照留底
9. 查找关联的受控商户网点信息
10. 重新打包 → 发往受控商户网点
11. forwarding_session.status = forwarded

受控商户网点收货：
12. Order.Status = in_store（验收通过）或 maintenance（验收损坏）
```

### 5.4 转发会话表结构

| 字段 | 类型 | 说明 |
|------|------|------|
| `id` | uuid | 主键 |
| `lease_session_id` | uuid | 关联租赁会话 |
| `merchant_id` | uuid | 受控商户 ID |
| `forwarding_site_id` | uuid | 转发网点 ID |
| `direction` | varchar(20) | `outbound`（发货方向）/ `return`（归还方向） |
| `status` | varchar(20) | `pending` / `shipped` / `received` / `forwarded` / `completed` |
| `session_code` | varchar(20) | 6 位短码，用户填写在物流留言中 |
| `instrument_id` | uuid | 关联乐器 |
| `photos` | jsonb | 拆封/打包照片 |
| `notes` | text | 备注 |
| `created_at` | timestamp | |
| `updated_at` | timestamp | |

---

## 六、与当前代码的差距汇总

| # | 问题 | 期望 | 实际 |
|---|------|------|------|
| 1 | 两条创建路径 | 仅 `POST /api/user/orders` (`reserved`) | 存在死代码 `POST /api/orders` (`pending`) |
| 2 | `reserved → paid` 无 handler | `PayOrder` 应接受 `reserved` | `PayOrder` 只接受 `pending` |
| 3 | 前端路径错误 | `OrderPayment.jsx` 调用 `/api/orders/:id/*` | 实际调 `/api/user/orders/:id/*`（不存在） |
| 4 | 状态门缺失 | 每个跳转校验前置状态 | `UpdateShipping`/`AssessDamage`/`SubmitAssessment` 不校验 |
| 5 | `in_store` 未实现 | cancelled/completed/terminated → `in_store` | 当前用 `in_stock`（语义不清）、`cancelled`、`terminated` 分散 |
| 6 | 幽灵状态 | 删除 `preparing` | 代码中使用但从未被设置 |
| 7 | 订单状态无常量 | 定义 const | 全裸字符串 |
| 8 | LeaseSession 不完整 | 验收完成后更新为 `completed` | 验收后不更新 |
| 9 | 乐器在 in_lease 时不可见 | 网点员工可见 rented 乐器 | 需确认查询逻辑 |
