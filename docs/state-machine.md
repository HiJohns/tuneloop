# 订单-租赁状态机标准文档

> 本文档定义 tuneloop 订单、乐器、租赁会话的**标准状态机**——即"期待的样子"。
> 后续所有修复和新增功能均以此文档为准。

---

## 一、订单状态 (Order.Status)

### 1.1 状态定义

| 状态 | 含义 | 进入条件 | 可执行操作 |
|------|------|---------|-----------|
| `reserved` | 顾客已下单，等待支付 | 顾客点击"立即租赁" | 支付、取消 |
| `paid` | 已支付，等待发货 | 支付成功 | 发货 |
| `shipped` | 已发货，运输中 | 网点员工录入物流 | 确认收货 |
| `in_lease` | 租赁中（乐器在用户手中） | 确认收货 | 归还 |
| `returning` | 归还中（运输中） | 用户发起归还 | 验收（通过/损坏） |
| `completed` | 已完成（验收通过） | 验收通过 | —（终态） |
| `maintenance` | 验收不通过，送修 | 验收发现损坏 | 定损、修复 |
| `cancelled` | 已取消 | 用户在 reserved/paid 状态取消 | —（终态） |
| `terminated` | 管理员强制终止 | 管理员操作 | —（终态） |

### 1.2 状态跳转图

```
                         ┌──────────┐
                         │ reserved │ ← 顾客下单
                         └────┬─────┘
                              │ 支付
                         ┌────▼─────┐
                    ┌───►│   paid    │
                    │    └────┬─────┘
                    │         │ 发货
                    │    ┌────▼─────┐
                    │    │  shipped  │
                    │    └────┬─────┘
                    │         │ 确认收货
                    │    ┌────▼─────┐
                    │    │ in_lease  │ ← 乐器在用户手中
                    │    └────┬─────┘
                    │         │ 归还
                    │    ┌────▼──────┐
                    │    │ returning  │
                    │    └────┬──────┘
                    │         │ 验收
                    │    ┌────┴─────┐
                    │    │          │
                    │ 通过        损坏
                    │    │          │
               ┌────▼──┐    ┌─────▼──────┐
               │completed│    │maintenance │
               └────────┘    └────────────┘

任意状态 ──→ cancelled （用户在 reserved/paid 状态取消）
任意状态 ──→ terminated（管理员强制终止）
```

### 1.3 状态门（Guard）

每个跳转必须校验前置状态，不允许从任意状态跳转：

| 跳转 | 允许的前置状态 |
|------|-------------|
| reserved → paid | reserved |
| paid → shipped | paid |
| shipped → in_lease | shipped |
| in_lease → returning | in_lease |
| returning → completed | returning |
| returning → maintenance | returning |
| reserved → cancelled | reserved |
| paid → cancelled | paid |
| * → terminated | 任意状态（管理员特权） |

---

## 二、乐器库存状态 (Instrument.StockStatus)

### 2.1 状态定义

| 状态 | 含义 | 对应订单状态 |
|------|------|-----------|
| `available` | 可租 | — |
| `reserved` | 已被预订 | reserved |
| `shipping` | 运输中（发往用户） | shipped |
| `rented` | 租赁中 | in_lease |
| `returning` | 归还途中 | returning |
| `maintenance` | 维修中 | maintenance |
| `archived` | 已归档/已售 | completed / 租转售 |

### 2.2 状态跳转

```
available ──→ reserved ──→ shipping ──→ rented ──→ returning ──┬──→ available (验收通过)
    ▲                                ▲                        │
    │                                │                        └──→ maintenance (验收损坏)
    └──── 取消 ──────────────────────┘                              │
                                                                   │
    available ◄── 维修完成 ◄─── maintenance ◄──────────────────────┘
                                           │
    available ◄── 定损完成 ◄───────────────┘
```

---

## 三、租赁会话状态 (LeaseSession.Status)

### 3.1 状态定义

| 状态 | 含义 | 对应订单状态 |
|------|------|-----------|
| `active` | 租赁进行中 | reserved → in_lease |
| `return_requested` | 已申请归还 | returning |
| `completed` | 租赁结束 | completed |

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
| 订单详情（本人） | ✅ | ❌ | ✅ | ✅ |
| 订单中的乐器 | ❌（不再是 available） | ✅ | ✅ | ✅ |
| 租赁人姓名 | 本人 | ✅ | ✅ | ✅ |
| 租赁人电话/邮箱 | 本人 | ✅ | ✅ | ✅ |
| **in_lease 状态** | | | | |
| 订单详情（本人） | ✅ | ❌ | ✅ | ✅ |
| 乐器状态（rented） | ✅ 可查看 | ✅ 可见 | ✅ | ✅ |
| 租赁人姓名 | 本人 | ✅ | ✅ | ✅ |
| 收货地址 | 用户填的 | ✅ | ✅ | ✅ |
| **returning 状态** | | | | |
| 归还地址 | 网点地址 | — | — | — |
| **受控商户（所有状态）** | | | | |
| 租赁人姓名/电话/邮箱 | 本人 | ❌ 隐藏 | ❌ 隐藏 | ✅ |
| 收货地址 | 转发网点地址 | 转发网点地址 | 转发网点地址 | ✅ |
| 转发会话 ID | ❌ | ✅ | ❌ | ✅ |

### 4.2 可见性规则说明

1. **乐器在 in_lease 状态时对网点员工可见**：员工需要知道乐器去向（哪个用户、多长时间），但不意味着能看到用户的全部隐私信息
2. **全权商户**：网点员工可看到租赁人姓名，用于客户服务
3. **受控商户**：网点员工不可看到租赁人信息，只有转发网点地址和转发会话 ID
4. **顾客本人**：永远只看到自己的订单

---

## 五、受控商户差异（Phase 2）

受控商户在状态机层面与全权商户**相同**，区别在于：

| 维度 | 全权商户 | 受控商户 |
|------|---------|---------|
| 订单状态流转 | 同上 | 同上 |
| 乐器状态流转 | 同上 | 同上 |
| 租赁会话状态 | 同上 | 同上 |
| 收货地址 | 用户填写 | 转发网点地址 |
| 归还地址 | 网点地址 | 转发网点地址 |
| 用户信息可见性 | 商户员工可见姓名/电话 | 商户员工不可见，仅转发会话 ID |
| 附加表 | — | forwarding_sessions（转发会话表） |

---

## 六、与当前代码的差距汇总

| # | 问题 | 期望 | 实际 |
|---|------|------|------|
| 1 | 两条创建路径 | 仅 `POST /api/user/orders` (`reserved`) | 存在死代码 `POST /api/orders` (`pending`) |
| 2 | `reserved → paid` 无 handler | `PayOrder` 应接受 `reserved` | `PayOrder` 只接受 `pending` |
| 3 | 前端路径错误 | `OrderPayment.jsx` 调用 `/api/orders/:id/*` | 实际调 `/api/user/orders/:id/*`（不存在） |
| 4 | 状态门缺失 | 每个跳转校验前置状态 | `UpdateShipping`/`AssessDamage`/`SubmitAssessment` 不校验 |
| 5 | `completed` 状态不存在 | final 状态用 `completed` | 用 `in_stock`（语义不清） |
| 6 | 幽灵状态 | 删除 `preparing` | 代码中使用但从未被设置 |
| 7 | 订单状态无常量 | 定义 const | 全裸字符串 |
| 8 | LeaseSession 不完整 | 验收完成后更新为 `completed` | 验收后不更新 |
| 9 | 乐器在 in_lease 时不可见 | 网点员工可见 rented 乐器 | 需确认查询逻辑 |
