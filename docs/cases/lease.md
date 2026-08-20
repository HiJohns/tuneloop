# 租赁域用例（Lease Domain）

> 来源: cases.md §2 迁移 | 用例编号: L-01 ~ L-04 | YAML 前置块供 AI 静态检查

---

id: L-00
domain: lease
flow: 登录分流（购物车提交/立即租赁触发）
steps:
  - seq: 1
    action: 未登录触发登录分流（来源 B）
    frontend:
      - platform: [weapp]
        page: /checkout
        role: [guest]
        gate: "未登录且点击提交订单/立即租赁"
        reach: "Detail「立即租赁」/ Cart「去结算」点击时先检查 token（getToken）；无 token → 记 post_auth_redirect 目标 → redirectToLogin('checkout') → wx.login → GET /auth/wx-accounts"
        controls: [提交订单按钮, 立即租赁按钮]
        displays: []
        ops:
          - {type: api, method: GET, path: /auth/wx-accounts, params: [code]}
    api: {method: GET, path: /auth/wx-accounts, params: [code]}
  - seq: 2
    action: 有顾客账户直接登录回目标页
    frontend:
      - platform: [weapp]
        page: /checkout
        role: [guest]
        gate: "accounts 含 is_customer=true"
        reach: "wx-accounts → 有顾客 → wx-login-select 登录 → 回目标页"
        controls: []
        displays: []
        ops:
          - {type: api, method: POST, path: /auth/wx-login-select, params: [code, user_id]}
          - {type: navigate, target: /checkout, gate: "登录成功后回原页面"}
    api: {method: POST, path: /auth/wx-login-select, params: [code, user_id]}
  - seq: 3
    action: 无顾客账户弹注册提示
    frontend:
      - platform: [weapp]
        page: /checkout
        role: [guest]
        gate: "无顾客账户（0 账户或只有员工）"
        reach: "wx-accounts → 无顾客 → 弹「您尚未注册会员，要注册吗？」"
        controls: [注册提示弹窗, 确认按钮, 取消按钮]
        displays: []
        ops:
          - {type: navigate, target: "/profile-complete?mode=member", gate: "弹窗选「是」"}
          - {type: navigate, target: /checkout, gate: "弹窗选「否」→ 停留原页"}
    api: {}
  - seq: 4
    action: 购物车合并去重（登录后）
    frontend:
      - platform: [weapp]
        page: /cart
        role: [customer]
        gate: "登录完成"
        reach: "游客 cart 与 cart_${userId} 合并去重（instrument_id + 租期）→ 清空游客 cart"
        controls: []
        displays: []
        ops: []
    api: {}
  - seq: 5
    action: 注册完成后回跳原页面（#1690）
    frontend:
      - platform: [weapp]
        page: /payment
        role: [guest]
        gate: "注册支付完成且存在 post_auth_redirect"
        reach: "会员费支付完成（finishMembershipFlow）→ 读 session.post_auth_redirect（如 /checkout?id=xx）→ 清除标记 → 跳转回原页面；无标记则默认个人中心"
        controls: []
        displays: []
        ops:
          - {type: navigate, target: "post_auth_redirect（详情/结算页）", gate: "有标记"}
          - {type: navigate, target: /profile, gate: "无标记"}
    api: {}
---

id: L-00B
domain: lease
flow: 购物车乐器已被租走的处理
steps:
  - seq: 1
    action: 购物车条目渲染已租出状态
    frontend:
      - platform: [weapp, h5]
        page: /cart
        role: [customer, guest]
        gate: "乐器 stock_status != available（已被租走/锁定）"
        reach: "购物车加载 → 校验条目库存状态 → 非 available 标记已租出"
        controls: []
        displays: [已租出条目（封面图黑白 + 「已被租出」标记）]
        ops: []
    api: {method: GET, path: /public/instruments/:id, params: [stock_status]}
  - seq: 2
    action: 已租出条目不可选中/不可提交
    frontend:
      - platform: [weapp, h5]
        page: /cart
        role: [customer, guest]
        gate: "条目已租出"
        reach: "已租出条目 checkbox 禁用；选中集不含该条目；grandTotal 不含其价格"
        controls: [勾选框（禁用）]
        displays: []
        ops: []
    api: {}
  - seq: 3
    action: 提交订单后端校验库存 + 加锁
    frontend:
      - platform: [weapp, h5]
        page: /checkout
        role: [customer]
        gate: "提交 POST /user/orders"
        reach: "后端校验乐器 stock_status == available，否则拒绝；通过后创建订单（reserved 待支付）即加锁"
        controls: [提交订单按钮]
        displays: []
        ops:
          - {type: api, method: POST, path: /user/orders}
    api: {method: POST, path: /user/orders, params: [instrument_id, ...]}
  - seq: 4
    action: 锁释放（与待支付状态同步）
    frontend:
      - platform: [backend]
        page: ""
        role: [system]
        gate: "订单 reserved 超 payment_deadline 未支付"
        reach: "reserved_order_scheduler 超时释放 → 订单取消 → 乐器 stock_status 恢复 available"
        controls: []
        displays: []
        ops: []
    api: {}
---

# L-00B 购物车乐器已被租走的处理

## 前置条件
- 购物车条目对应乐器 `stock_status` 非 available（已被他人租走/锁定/维修等）

## 流程
1. 购物车加载时校验每件乐器库存状态（`GET /public/instruments/:id` 或列表接口含 stock_status）
2. 非 available 条目：封面图黑白 + 显示「已被租出」标记；checkbox 禁用、不可选中、不计入小计
3. 提交订单（POST /user/orders）：后端**再次校验** `stock_status == available`，非 available 拒绝（409/400 + 提示「该乐器已被租出」）
4. 校验通过创建订单（reserved 待支付）即**加锁**（乐器不可再被下单）
5. 锁释放：reserved 超 `payment_deadline` 未支付 → scheduler 取消订单 → 恢复 available

## 关键规则
- **前端校验是体验优化，后端校验是安全底线**（并发/绕过前端均以后端为准）
- 加锁 = 创建 reserved 订单（乐器的 stock_status 在订单有效期内视为锁定）
- 锁释放与现有 `reserved_order_scheduler.go` 的 payment_deadline 机制同步（复用，不新造）

## 验收
- 已租出条目：黑白封面 + 不可选 + 不计价
- 提交订单时后端拒绝已租出乐器
- reserved 超时后乐器恢复可租
- Go 测试：后端校验 + 加锁 + 释放

---

id: L-01
domain: lease
flow: 正常租赁
steps:
  - seq: 1
    action: 提交订单
    frontend:
      - platform: [weapp, h5]
        page: /checkout
        role: [customer]
        gate: ""
        reach: "Detail 页 → 立即租赁 → Checkout"
        controls: [乐器卡片, 租期调节器, 优惠码输入, 提交订单按钮]
        displays: [租金明细, 押金, 合计]
        ops:
          - {type: api, method: POST, path: /user/orders}
          - {type: navigate, target: /payment}
    api: {method: POST, path: /user/orders, params: [instrument_id, start_date, end_date, rent_days]}
    # 关联子 API（L-01 主流程的前置/附属操作）
    related:
      - {method: GET, path: /user/instruments}
      - {method: GET, path: /user/instruments/:id}
      - {method: GET, path: /user/addresses}
      - {method: POST, path: /user/addresses}
      - {method: DELETE, path: /user/addresses/:id}
      - {method: GET, path: /user/guarantors}
      - {method: POST, path: /user/guarantors}
      - {method: DELETE, path: /user/guarantors/:id}
      - {method: GET, path: /categories}
      - {method: GET, path: /public/instruments/:id/pricing-v2}
  - seq: 2
    action: 支付
    frontend:
      - platform: [weapp, h5]
        page: /payment
        role: [customer]
        gate: "订单状态 = reserved"
        reach: "Checkout 提交 → Payment"
        controls: [点数抵扣滑块, 优惠码输入, 确认支付按钮]
        displays: [租金, 押金, 应付总额, 赠点可用上限, 现金应付额, 优惠后金额]
        ops:
          - {type: api, method: POST, path: /pay/prepay}
          - {type: navigate, target: /success}
    api: {method: POST, path: /pay/prepay, params: [order_id, order_type, amount, gift_points_used, coupon_code]}
  - seq: 3
    action: 发货（员工）
    frontend:
      - platform: [pc]
        page: /staff/orders
        role: [staff]
        gate: "订单状态 = paid/pending_shipment"
        reach: "PC 订单列表 → 发货"
        controls: [物流单号输入, 快递公司选择, 物流费输入, 提交按钮]
        displays: [订单信息, 物流费]
        ops:
          - {type: api, method: PUT, path: /warehouse/orders/:id/shipping}
    api: {method: PUT, path: /warehouse/orders/:id/shipping, params: [tracking_number, company, shipped_at, shipping_fee]}
  - seq: 3b
    action: 发货（员工，移动端跨端入口）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = paid/pending_shipment 且 isStaff"
        reach: "员工工作台 → 订单详情 → 发货按钮 → 跳转独立发货页（L-07 seq2）"
        controls: [发货按钮]
        displays: [订单信息]
        ops:
          - {type: navigate, target: "/staff/shipping?order=:id"}
    api: {method: PUT, path: /warehouse/orders/:id/shipping, params: [tracking_number, company, shipped_at, shipping_fee]}
    # 预期失败项（#1614/#1613）：/staff/shipping 需在 weappPages 注册（#1610 实现后通过）
  - seq: 4
    action: 确认收货
    frontend:
      - platform: [weapp, h5]
        page: /order-detail
        role: [customer]
        gate: "订单状态 = shipped/in_transit"
        reach: "MyLeases 列表 → 订单详情 → 确认收货"
        controls: [确认收货按钮]
        displays: [订单状态, 物流信息]
        ops:
          - {type: api, method: PUT, path: /warehouse/orders/:id/delivery}
    api: {method: PUT, path: /warehouse/orders/:id/delivery, params: [delivered_at]}
  - seq: 5
    action: 归还
    frontend:
      - platform: [weapp, h5]
        page: /my-leases
        role: [customer]
        gate: "订单状态 = in_lease/expired"
        reach: "MyLeases → 订单详情 → 归还"
        controls: [归还按钮]
    # 物流信息输入位于归还确认页（return-confirm），归还后跳转
    return_confirm:
      - platform: [weapp, h5]
        page: /return-confirm
        role: [customer]
        gate: ""
        controls: [物流公司选择, 快递单号输入]
        displays: [订单信息]
        ops:
          - {type: api, method: POST, path: /orders/:id/return}
          - {type: navigate, target: /return-settlement}
    api: {method: POST, path: /orders/:id/return, params: [courier_company, tracking_number]}
  - seq: 6
    action: 验收（员工）
    frontend:
      - platform: [pc]
        page: /staff/receiving
        role: [staff]
        gate: "订单状态 = returning"
        reach: "PC 收货界面 → 扫码 → 验收"
        controls: [扫码输入, 拍照, 条件选择(good/damaged), 提交按钮]
        displays: [乐器信息, 租赁信息]
        ops:
          - {type: api, method: PUT, path: /warehouse/orders/:id/return-inspect}
    api: {method: PUT, path: /warehouse/orders/:id/return-inspect, params: [instrument_sn, scan_time, condition, photos]}
  - seq: 7
    action: 查看结算退款
    frontend:
      - platform: [weapp, h5]
        page: /order-detail
        role: [customer]
        gate: "订单状态 = completed（已完成/done）"
        reach: "订单详情 → 费用信息/收支明细"
        controls: [费用信息区, 合同快照展开]
        displays: [实际租期, 实际租金, 赠点抵扣, 现金实付, 退款明细, 退款合计, 返点赠点]
        ops: []
    api: {method: GET, path: /orders/:id, params: []}
---

# L-01 正常租赁

## 前置条件
- 用户已登录（顾客 token，oid/tid 为空）
- 乐器库存状态 = available，有定价配置

## 流程
1. 首页 → 乐器列表 → 筛选（类别/网点/级别/可租状态）
2. 点选乐器 → 详情页（图片/品牌/型号/租金/押金）
3. 点击下单 → Checkout：选租期（日/周/月）、收货地址
4. 提交 → POST /user/orders → 跳转 Payment
5. 支付 → POST /pay/prepay（微信支付）→ 乐器进入 reserved→paid
6. 员工发货（填物流费）→ shipped
7. 顾客确认收货 → in_lease
8. 租赁期满归还 → returning
9. 员工验收（good）→ completed → 自动结算退款

## 关键规则
- 物流费：发货时员工填写（#1541），下单时不显示（#1570）
- 结算：`R = Ra + De - Dd - O - Re`（§2.7）
- **赠点抵扣（初次付款+续费通用）**：付款总额 `R0 = C0 + A0`，其中 `A0`（赠点抵扣）≤ `floor(R0 × 会员级别赠点使用比例)`，`C0`（现金）为微信实付。比例由赠点策略配置（L-05）
- **退款差额**：退款时按调整后应付 `R1` 与当前级别使用比例重算赠点上限 `A1`；`A1 < A0` 时退 `A0−A1` 到赠点账户、退 `C0−C1` 到微信（详见 L-06）
- **累计花销口径**：`total_spending` 按 `C1`（实付现金）累计，不含赠点面值——避免赠点循环放大（详见 L-06）

---
id: L-02
domain: lease
flow: 提前归还
steps:
  - seq: 1
    action: 归还（租期内）
    frontend:
      - platform: [weapp, h5]
        page: /my-leases
        role: [customer]
        gate: "订单状态 = in_lease 且 today < end_date"
        reach: "MyLeases → 订单详情 → 归还"
        controls: [归还按钮]
    # 物流信息输入位于归还确认页（return-confirm），归还后跳转
    return_confirm:
      - platform: [weapp, h5]
        page: /return-confirm
        role: [customer]
        gate: ""
        controls: [物流公司选择, 快递单号输入]
        displays: [订单信息]
        ops:
          - {type: api, method: POST, path: /orders/:id/return}
          - {type: navigate, target: /return-settlement}
    api: {method: POST, path: /orders/:id/return, params: [courier_company, tracking_number]}
  - seq: 2
    action: 查看预估结算
    frontend:
      - platform: [weapp, h5]
        page: /return-settlement
        role: [customer]
        gate: "订单状态 = returning"
        reach: "归还提交 → ReturnSettlement"
        controls: [确认按钮]
        displays: [实际租期, 实际租金, 提前归还退费, 逾期费用, 损坏赔偿]
        ops: []
    api: {method: GET, path: /user/settlements/:id/calculate, params: []}
---

# L-02 提前归还

## 前置条件
- 订单 in_lease，租期未满

## 流程
1. 租期内点归还 → returning
2. 员工验收（good）→ completed → 自动结算
3. 退款含 `early_return_rebate = Ra - Re`（未用天数按阶梯折算退回）

## 关键规则
- 实际天数 = CalculateDays(start_date, returned_at)
- 退款 = Ra + De - Re（无逾期/损坏时）
- **赠点差额与返点**：按 L-06 差额结算执行（A1 重算、C1 累计、A2 返点、完成通知）

---
id: L-03
domain: lease
flow: 超期归还
steps:
  - seq: 1
    action: 归还（超期）
    frontend:
      - platform: [weapp, h5]
        page: /my-leases
        role: [customer]
        gate: "订单状态 = in_lease/expired 且 today > end_date"
        reach: "MyLeases → 订单详情 → 归还"
        controls: [归还按钮]
    # 物流信息输入位于归还确认页（return-confirm），归还后跳转
    return_confirm:
      - platform: [weapp, h5]
        page: /return-confirm
        role: [customer]
        gate: ""
        controls: [物流公司选择, 快递单号输入]
        displays: [订单信息, 逾期提示]
        ops:
          - {type: api, method: POST, path: /orders/:id/return}
    api: {method: POST, path: /orders/:id/return, params: [courier_company, tracking_number]}
  - seq: 2
    action: 验收（员工）计算逾期费
    frontend:
      - platform: [pc]
        page: /staff/receiving
        role: [staff]
        gate: "订单状态 = returning"
        reach: "PC 收货界面 → 扫码 → 验收"
        controls: [扫码输入, 拍照, 条件选择, 提交按钮]
        displays: [乐器信息, 逾期天数]
        ops:
          - {type: api, method: PUT, path: /warehouse/orders/:id/return-inspect}
    api: {method: PUT, path: /warehouse/orders/:id/return-inspect, params: [instrument_sn, scan_time, condition]}
    related:
      - {method: GET, path: /overdue-leases}
      - {method: GET, path: /orders/:id/pickup}
      - {method: POST, path: /orders/:id/cancel}
      - {method: GET, path: /orders/by-trade-no/:out_trade_no}
      - {method: GET, path: /reports/assessment/:order_id}
---

# L-03 超期归还

## 前置条件
- 订单 in_lease/expired，超期归还

## 流程
1. 超期归还 → returning（状态 expired 仅标记不扣款）
2. 员工验收 → InspectReturn 计算 `overdue_fee = overdue_days × overdue_daily_fee`（#1493）→ 从押金扣除
3. completed → 结算退款（押金扣除逾期费后余额参与退款）

## 关键规则
- 逾期费一次性收取（不再每日扣款，#1492）
- 极端欠费线下处理，不挂账
- **赠点差额与返点**：按 L-06 差额结算执行（A1 重算、C1 累计、A2 返点、完成通知）

---
id: L-04
domain: lease
flow: 定损与申诉（退款三路径）
steps:
  - seq: 1
    action: 验收定损（员工）
    frontend:
       - platform: [weapp, h5, pc]
         page: /staff/receiving
         role: [staff]
         gate: "订单状态 = returning"
         reach: "员工工作台 → 收货验收（订单详情 → 接收按钮跳转）"
         controls: [扫码输入, 拍照, 条件选择(good/damaged), 定损金额输入, 评论输入]
         displays: [乐器信息, 租赁信息]
         ops:
           - {type: api, method: PUT, path: /warehouse/orders/:id/return-inspect}
     api: {method: PUT, path: /warehouse/orders/:id/return-inspect, params: [instrument_sn, scan_time, condition, damage_amount, notes]}
     # 物流费在设计变更后于发货页填写（L-01 seq3），验收页不再录入（#1621 设计修正）
     # #1708/#1709：验收统一写 damage_reports（good 与 damaged 都建；good → status=completed，
     # damaged → status=pending 待顾客决策）；damage_assessments 表已废弃。
     # 员工照片写入 instrument_media（batch_type=receiving, is_display=false），
     # 不再写 assessment.photos JSONB（已废弃）。
     # 路径 1（good）：验收无损坏 → 立即自动结算退款 → 汇聚 seq 8 收据
     # 路径 2/3（damaged）：进入 seq 2 定损决策
  - seq: 2
    action: 收到定损通知并决策（顾客）
    frontend:
      - platform: [weapp, h5]
        page: /message-detail
        role: [customer]
        gate: "订单状态 = pending_damage_response"
        reach: "消息列表 → 定损通知详情"
        controls: [接受按钮, 拒绝按钮(发起申诉)]
        displays: [定损照片, 定损金额, 评论]
        ops:
          - {type: api, method: POST, path: /appeals/:id/agree}
          - {type: api, method: POST, path: /appeals}
      # #1707：订单详情页为死锁自救入口（定损通知可能丢失）——待回应定损态
      # 订单详情展示定损面板（金额/描述/照片）+ 接受/拒绝按钮，与通知按钮一致。
      - platform: [weapp, h5]
        page: /order/:id
        role: [customer]
        gate: "订单状态 = pending_damage_response 或 damage_appealing"
        reach: "订单详情（用户必然可达，无需通知）"
        controls: [接受定损按钮, 拒绝定损按钮(发起申诉), 定损照片缩略图(点击放大/轮换)]
        displays: [定损金额, 定损说明, 定损照片, 费用明细(实际租期/实际租金/赔偿金额/退款)]
        ops:
          - {type: api, method: POST, path: /appeals/:damage_id/agree}
          - {type: api, method: POST, path: /appeals}
    api: {method: POST, path: /appeals/:id/agree, params: []}
  - seq: 3
    action: 用户接受 → 系统通知员工（路径 2）
    frontend:
      - platform: [weapp, h5]
        page: /messages
        role: [staff]
        gate: "订单状态 = deposit_refunding"
        reach: "系统通知 → 员工消息列表 → 通知详情"
        controls: [通知条目, 查看订单按钮]
        displays: [通知标题, 订单号, 乐器 SN]
        ops:
          - {type: api, method: GET, path: /notifications}
          - {type: navigate, target: /order/:id}
    api: {method: GET, path: /notifications, params: []}
  - seq: 4
    action: 员工订单详情点击退款（路径 2）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = deposit_refunding"
        reach: "通知链接 → 订单详情"
        controls: [退款按钮]
        displays: [订单信息, 定损金额, 应付差额预览]
        ops:
          - {type: api, method: POST, path: /orders/:id/refund}
          - {type: navigate, target: "/payment?type=refund"}
    api: {method: POST, path: /orders/:id/refund, params: []}
  - seq: 5
    action: 用户拒绝 → 申诉创建（路径 3 起点）
    frontend:
      - platform: [weapp, h5]
        page: /message-detail
        role: [customer]
        gate: "订单状态 = pending_damage_response"
        reach: "定损通知 → 拒绝 → 申诉表单"
        controls: [申诉原因输入, 提交按钮]
        displays: [定损金额, 申诉填写区]
        ops:
          - {type: api, method: POST, path: /appeals}
    api: {method: POST, path: /appeals, params: [object_type, object_id, description, images]}
  - seq: 6
    action: 申诉终审（网点或商户管理员）
    frontend:
      - platform: [pc]
        page: /appeals
        role: [site_admin, merchant_admin]
        gate: "存在 pending appeal"
        reach: "PC 申诉管理页 → 打开申诉"
        controls: [无损坏按钮, 调整金额输入(可选), 说明输入, 提交按钮]
        displays: [乐器信息, 租金, 图片, 员工定损说明, 顾客申诉理由, 原判金额]
        ops:
          - {type: api, method: PUT, path: /appeals/:id/resolve}
    api: {method: PUT, path: /appeals/:id/resolve, params: [result, amount, notes]}
    # 提交后：系统同时通知顾客（终审结果）与员工（待退款）
  - seq: 7
    action: 员工收到通知 → 订单详情点击退款（路径 3）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = deposit_refunding"
        reach: "员工消息列表 → 终审通知链接 → 订单详情"
        controls: [退款按钮]
        displays: [订单信息, 终审金额, 应付差额预览]
        ops:
          - {type: api, method: POST, path: /orders/:id/refund}
          - {type: navigate, target: "/payment?type=refund"}
    api: {method: POST, path: /orders/:id/refund, params: []}
  - seq: 8
    action: 支付页退款收据（三路径汇聚）
    frontend:
      - platform: [weapp, h5]
        page: /payment
        role: [customer, staff]
        gate: "type = refund"
        reach: "退款执行后自动跳转/通知链接"
        controls: [收据展示区]
        displays: [乐器SN分类, 实际租期, 实际租金R1, 赠点抵扣A1, 现金应付C1, 已收总额, 退赠点A0-A1, 退现金C0-C1, 退款合计]
        ops: []
    api: {method: GET, path: /pay/calculate, params: [order_id, order_type=refund]}
---

# L-04 定损与申诉（退款三路径）

## 前置条件
- 乐器归还验收（good 或 damaged）

## 三路径总览
| 路径 | 触发 | 退款发起 | 汇聚点 |
|------|------|---------|--------|
| (1) 无损坏 | 验收 good | 系统自动（InspectReturn 内） | seq 8 收据 |
| (2) 损坏-用户接受 | 定损通知 → 接受 | 员工订单详情点退款 | seq 8 收据 |
| (3) 损坏-用户申诉 | 定损通知 → 拒绝 → 申诉 → 经理终审 | 员工订单详情点退款 | seq 8 收据 |

## 流程
1. 员工验收 damaged → 订单 pending_damage_response，乐器进维修 → 通知顾客
2. 顾客在通知详情选择：
   - **接受** → 订单 deposit_refunding → 系统通知员工
   - **拒绝** → 申诉创建页（填写原因/照片）→ 订单 damage_appealing
3. 申诉 → 网点或商户管理员在 PC 申诉管理页打开 → 修改赔偿额（可选）→ 提交终审 → 系统通知顾客与员工
4. 员工收到通知 → 点击链接跳订单详情 → 点击退款 → 执行差额结算（L-06）→ 跳转支付页收据
5. 三路径最终都呈现标准退款收据（seq 8），差额 = 最初实付 − 实需付款

## 关键规则
- 押金足以覆盖定损：接受后进入 deposit_refunding 待员工退款；不足：用户先补付再走退款
- 员工退款按钮仅在 `deposit_refunding` 状态显示（订单详情）
- 终审后通知**同时**发顾客（结果）与员工（待退款链接）
- 支付页收据必须含「乐器 SN（分类）」行与赠点/现金分账明细（L-06）
- 退款执行后订单关单 `completed`（已完成/done），累计花销按 C1 累计（L-06）
- AcceptDamage/RejectDamage 旧端点废弃：前端统一走 `POST /appeals/:id/agree` 与 `POST /appeals`
- #1707 静态检查点：
  - 待回应定损/定损申诉态订单详情（weapp+h5）渲染定损面板：定损金额、说明、照片缩略图（点击 `previewImage` 放大/多幅轮换）、接受/拒绝按钮
  - 费用明细含实际租期/实际租金/赔偿金额/退款（数据来自后端 `order.damage` 对象，前端不自行算退款）
  - 接受按钮调用 `POST /appeals/:damage_id/agree`；拒绝按钮提交 `POST /appeals`（body: damage_report_id + appeal_reason）
  - report 已处理（agreed/appealed）时按钮隐藏、显示状态文案
  - **定损照片数据来源（#1708 更新）**：`order.damage.photos` **仅**来自 `instrument_media`（batch_type=receiving, file_type=image, is_display=false，storage_key 补 `/uploads/media/` 前缀）——damage_assessments.photos JSONB 已废弃（#1710/#1711），不再读取
  - **静态检查必须验证数据源而非仅 JSX**：displays 的"定损照片"需交叉核对 GetOrder 的 photos 组装（两个来源），防止"前端有渲染、后端无数据"的脱节

## 验收
- `go test` 覆盖：路径(2) 接受 → deposit_refunding → 员工 POST /orders/:id/refund → settlement 生成 + 通知；路径(3) 申诉 → resolve → 员工退款 → 收据含 SN 行
- `go test` 覆盖：GetOrder 待回应定损态返回 `damage` 对象（TestGetOrder_DamagePanel：金额/描述/照片/退款公式）
- checklist-verify.py：L-04 seq 2（订单详情入口）+ seq 3/4/7/8 的 displays 与 MessageDetail/OrderDetail/Payment JSX 交叉验证

### L-04B 定损补差支付（damage，#1719 补充）

**前置条件**：员工同意定损且定损金额 > 押金（`POST /appeals/:damage_id/agree` 返回 `payment_required:true` + amount + out_trade_no）

**流程**：
1. 员工同意定损（押金不足）→ 系统发消息通知顾客（action=payment，action_data 含 payment_required/amount/out_trade_no）
2. 消息详情 → 「去支付」→ 跳转 `/payment?type=damage&id=:order_id`（MessageDetail.jsx:83,127）
3. 支付页（type=damage）→ 可填写优惠码（OREZ 全免 / ENO 1%，服务端重算）→ 微信支付

**关键规则**：
- prepay order_type=damage 金额 = 补差金额（优惠码重算后），openid 由后端按 iam_sub 回填
- OREZ → amount=0 走 waive 记账（applySideEffects damage case → 订单 completed + 结算退款）
- 支付完成回调 → 订单 completed → instrument 恢复 available

**验收**：prepay damage 无 open_id 可支付；OREZ/ENO 优惠码生效；回调后订单完成。

---

id: L-05
domain: lease
flow: 赠点策略配置
steps:
  - seq: 1
    action: 打开赠点策略管理页
    frontend:
      - platform: [pc]
        page: /system/gift-policies
        role: [namespace_admin]
        gate: "sys_perm 赠点策略权限位（或 rebate:manage）"
        reach: "系统管理 → 赠点策略"
        controls: [会员级别列表, 赠点使用比例输入, 退款返点比例输入, 启用开关, 保存按钮]
        displays: [级别名称, 当前使用比例, 当前返点比例, 生效状态]
        ops:
          - {type: api, method: GET, path: /admin/gift-policies}
          - {type: api, method: PUT, path: /admin/gift-policies/:level_id}
    api: {method: PUT, path: /admin/gift-policies/:level_id, params: [pay_ratio, refund_ratio, is_active]}
  - seq: 2
    action: 修改某级别比例并保存
    frontend:
      - platform: [pc]
        page: /system/gift-policies
        role: [namespace_admin]
        gate: ""
        reach: "赠点策略页 → 编辑行 → 保存"
        controls: [比例输入, 保存按钮]
        displays: [保存成功提示]
        ops:
          - {type: api, method: PUT, path: /admin/gift-policies/:level_id}
    api: {method: PUT, path: /admin/gift-policies/:level_id, params: [pay_ratio, refund_ratio, is_active]}
  - seq: 3
    action: 支付页按新比例生效（顾客）
    frontend:
      - platform: [weapp, h5]
        page: /payment
        role: [customer]
        gate: ""
        reach: "支付页加载 → 按用户当前级别取比例"
        controls: [点数抵扣滑块]
        displays: [赠点可用上限=floor(应付总额×级别pay_ratio)]
        ops:
          - {type: api, method: GET, path: /pay/calculate}
    api: {method: GET, path: /pay/calculate, params: [order_id, order_type]}
---

# L-05 赠点策略配置

## 前置条件
- namespace_admin 登录 PC
- `membership_levels` 表有数据（级别主数据）

## 流程
1. 系统管理 → 赠点策略 → 列表展示各会员级别两行配置：
   - `pay_ratio`（赠点使用比例）：初次付款/续费时，赠点抵扣 ≤ floor(应付总额 × pay_ratio)
   - `refund_ratio`（退款返点比例）：退款完成后，按实付现金 C1 × refund_ratio 发放返点赠点
2. 编辑某级别 → 保存 → PUT /admin/gift-policies/:level_id
3. 支付页与结算页即时按用户当前级别取最新比例

## 关键规则
- **一套策略，两处消费**：pay_ratio 管支付抵扣（L-01 seq2），refund_ratio 管退款返点（L-06 seq4）
- **分会员级别独立设置**：默认级别兜底（level_id=0 或 is_default 行）
- 归并旧体系：`points_policies.max_pay_ratio` 与 `membership_gift_ratios.SelfSpendRatio` 迁移至本策略，旧表废弃
- 修改后仅影响**新发生的支付/退款**，历史订单快照不变

## 数据模型
```sql
gift_policies
├── id (uuid)
├── level_id (int, unique, FK → membership_levels)  -- 0 = 默认兜底
├── pay_ratio (decimal)      -- 赠点使用比例 0.00~1.00
├── refund_ratio (decimal)   -- 退款返点比例 0.00~1.00
├── is_active (bool)
├── created_at / updated_at
```

## 验收
- `go test` 覆盖：PUT 更新 → GET 回读 → /pay/calculate 按新比例返回 max_gift_amount；退款后返点按新 refund_ratio 发放
- checklist-verify.py：L-05 displays 与 PC GiftPolicies 页面交叉验证

---
id: L-06
domain: lease
flow: 退款差额结算与返点
steps:
  - seq: 1
    action: 员工触发退款
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = deposit_refunding"
        reach: "通知链接 → 订单详情 → 退款按钮"
        controls: [退款按钮, 差额预览区]
        displays: [应付差额预览(R1, A1, 退赠点, 退现金)]
        ops:
          - {type: api, method: POST, path: /orders/:id/refund}
    api: {method: POST, path: /orders/:id/refund, params: []}
  - seq: 2
    action: 系统差额结算（内部）
    frontend: []
    api: {method: POST, path: /orders/:id/refund, params: [], internal: "A1=floor(R1×pay_ratio); 退赠点=A0−A1; 退现金=C0−C1; C1=R1−A1"}
  - seq: 3
    action: 关单并累计花销（内部）
    frontend: []
    api: {method: POST, path: /orders/:id/refund, params: [], internal: "order.status=completed; total_spending+=C1"}
  - seq: 4
    action: 发放退款返点（内部）
    frontend: []
    api: {method: POST, path: /orders/:id/refund, params: [], internal: "A2=floor(C1×refund_ratio); promo_points+=A2; points_transactions(type=refund_rebate)"}
  - seq: 5
    action: 完成通知客户（顾客）
    frontend:
      - platform: [weapp, h5]
        page: /message-detail
        role: [customer]
        gate: ""
        reach: "消息列表 → 订单完成通知"
        controls: [完整收据展示区, 会员中心按钮/链接]
        displays: [订单完成标题, 感谢语, 标准收据(含乐器SN), 退赠点, 退现金, 返点赠点A2, 会员中心入口]
        ops:
          - {type: api, method: GET, path: /notifications/:id}
          - {type: navigate, target: /membership}
    api: {method: GET, path: /notifications/:id, params: []}
---

# L-06 退款差额结算与返点

## 前置条件
- 订单处于 deposit_refunding（员工已点退款触发）

## 流程
1. 员工触发退款 → `POST /orders/:id/refund`
2. **差额结算**：按调整后应付租金 `R1` 与当前级别赠点使用比例算 `A1 = floor(R1 × pay_ratio)`：
   - 若 `A1 < A0`：退 `A0−A1` 回赠点账户，退 `C0−C1` 回微信（`C1 = R1 − A1`）
   - 若 `A1 ≥ A0`：赠点不退，退现金 `C0 − C1`（`C1 = R1 − A0`，赠点保持 A0）
   - 校验：`退赠点 + 退现金 = R0 − R1`（总差额守恒）
3. **关单**：订单状态 → `completed`（已完成/done）；`total_spending += C1`（实付现金口径）
4. **返点**：`A2 = floor(C1 × refund_ratio)` 发放到赠点账户 + 流水
5. **通知**：发送订单完成通知——完整收据 + 感谢语 + 赠点到账（A0−A1 退回 + A2 返点）+ 会员中心链接

## 关键规则
- **累计花销口径 = C1**：仅实付现金累计 total_spending，不含赠点面值（防赠点循环放大；行业惯例：航司里程/信用卡积分/电商成长值均按实付）
- **返点基数 = C1**：与累计花销口径统一
- 支付快照必须存 `pay_ratio`（修复：PointsPolicySnapshot 此前仅存 scope_type/scope_id，cap_rate 恒为 0）
- 支付回调使用赠点后必须同步扣减 `cash_paid`（防结算双计）
- `ConfirmSettlement` 手动确认路径同样置 `completed` 并走同一差额结算

## 验收
- `go test` 覆盖：A1<A0 分账退返 / A1≥A0 仅退现金 / total_spending=C1 / A2 返点入账 / 完成通知含收据+会员中心链接
- checklist-verify.py：L-06 seq1/5 的 displays 与 OrderDetail/MessageDetail JSX 交叉验证

---
id: L-07
domain: lease
flow: 订单详情页行为（角色×状态→控件矩阵）
steps:
  - seq: 1
    action: 顾客视角订单操作
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [customer]
        gate: ""
        reach: "MyLeases → 订单详情"
        controls: [支付按钮(reserved), 取消按钮(paid/pending_shipment), 确认收货按钮(in_transit/shipped), 续期按钮(in_lease/expired), 归还按钮(in_lease/expired)]
        displays: [订单状态, 订单信息, 物流信息, 乐器信息]
        ops:
          - {type: api, method: POST, path: /pay/prepay}
          - {type: api, method: POST, path: /orders/:id/cancel-by-user}
          - {type: api, method: PUT, path: /warehouse/orders/:id/delivery}
    api: {}
  - seq: 2
    action: 员工发货（跳转独立发货页）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = paid/pending_shipment 且 isStaff"
        reach: "员工订单详情 → 发货按钮"
        controls: [发货按钮]
        displays: [订单信息]
        ops:
          - {type: navigate, target: "/staff/shipping?order=:id"}
    api: {}
  - seq: 3
    action: 员工接收并转发（跳转独立发货页）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = in_transit 且 isStaff"
        reach: "员工订单详情 → 接收并转发按钮"
        controls: [接收并转发按钮]
        displays: [订单信息]
        ops:
          - {type: navigate, target: "/staff/shipping?order=:id"}
    api: {}
  - seq: 4
    action: 员工收货验收（跳转独立收货页）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = returning 且 isStaff"
        reach: "员工订单详情 → 接收按钮"
        controls: [接收按钮]
        displays: [订单信息]
        ops:
          - {type: navigate, target: "/staff/receiving?order_id=:id"}
    api: {}
  - seq: 5
    action: 员工退款（#1607）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "订单状态 = deposit_refunding 且 isStaff"
        reach: "员工订单详情 → 退款按钮"
        controls: [退款按钮]
        displays: [订单信息, 定损金额, 应付差额预览]
        ops:
          - {type: api, method: POST, path: /orders/:id/refund}
          - {type: navigate, target: "/payment?type=refund"}
    api: {method: POST, path: /orders/:id/refund, params: []}
  - seq: 6
    action: 员工取消免押金订单（担保人不符）
    frontend:
      - platform: [weapp, h5]
        page: /order/:id
        role: [staff]
        gate: "order.deposit_waived = true 且 isStaff（担保人卡片内）"
        reach: "员工订单详情 → 担保人卡片 → 取消订单按钮"
        controls: [取消订单按钮]
        displays: [担保人信息]
        ops:
          - {type: api, method: POST, path: /warehouse/orders/:id/staff-cancel}
    api: {method: POST, path: /warehouse/orders/:id/staff-cancel, params: [reason]}
---

# L-07B 续期支付（renewal，#1719 补充）

## 前置条件
- 订单状态 in_lease / expired（顾客可续期）
- 续期金额由 Renewal 页计算（GET /orders/:id/renewal/calculate → total_amount）

## 流程
1. 订单详情（in_lease/expired）→ 点「续期」→ 跳转 Renewal 页（`/renewal?order=:id`，H5）/ `pages-weapp/renewal/index?order=:id`（weapp）
2. Renewal 页计算续期费 → 跳转支付页 `/payment?type=renewal&id=:orderId&amount=:total_amount`（Renewal.jsx:84 / pages-weapp/renewal/Renewal.jsx:80）
3. 支付页（type=renewal）→ 可填写优惠码（OREZ 全免 / ENO 1%，服务端重算）→ 微信支付（openid 由后端从本地 users 缓存回填，前端不传 open_id）

## 关键规则
- 优惠码对所有支付类型通用（#1719）：amount 传原金额 + coupon_code，后端 coupons 表重算（waive→0 走记账分支；percent→按比例）
- 无优惠码时 amount = 续期计算总额

## 验收
- prepay order_type=renewal：无 open_id 时后端按 iam_sub 回填 wx_openid（不再 400）
- OREZ → amount=0 走 waive 记账（applyRenewalSideEffects）；ENO → 1% 走真实 JSAPI

---

# L-07 订单详情页行为（跨端一致性标准）

## 前置条件
- 用户已登录（顾客或员工）
- 订单详情数据已加载（GET /orders/:id）

## 跨端一致性要求（H5 与 weapp 必须一致）

| 角色 | 状态 | 控件 | 动作 | 交互模式（统一） |
|------|------|------|------|------|
| 顾客 | reserved | 支付 | POST /pay/prepay | 订单详情内嵌 |
| 顾客 | paid/pending_shipment | 取消 | cancel-by-user | 订单详情内嵌 |
| 顾客 | in_transit/shipped | 确认收货 | delivery | 订单详情内嵌 |
| 顾客 | in_lease/expired | 续期/归还 | renewal/return | 订单详情内嵌 |
| 员工 | paid/pending_shipment | 发货 | 跳转发货页 | **跳转独立页** `/staff/shipping` |
| 员工 | in_transit | 接收并转发 | 跳转发货页 | **跳转独立页** `/staff/shipping` |
| 员工 | returning | 收货验收 | 跳转收货页 | **跳转独立页** `/staff/receiving` |
| 员工 | deposit_refunding | 退款 | POST /orders/:id/refund → 收据 | 订单详情内嵌（跳收据） |
| 员工 | deposit_waived | 取消订单 | staff-cancel（reason=担保人不符合要求） | 担保人卡片内嵌 |

## 关键规则（决策记录）
- **发货/收货采用跳转独立页模式**（订单详情页只放入口按钮，业务操作在独立页完成）——H5 现行模式，weapp 需对齐（当前 weapp 是内嵌表单，需改造为跳转，见 #1610/#1611）
- 员工取消订单按钮仅出现在免押金订单（deposit_waived）担保人卡片内（#1557）
- 退款按钮仅 deposit_refunding 状态（#1607）
- weapp 的 OrderDetail 独立副本（pages-weapp/order-detail）需与共享版行为一致——最终目标：**weapp 薄壳引用共享 OrderDetail.jsx**（消除双代码库）

## 验收
- checklist-verify.py：L-07 各 seq 的 page/controls/api 在 weapp+h5 均通过（配合 #1613 修复）
- H5 与 weapp 订单详情渲染同一控件集（真机对比）

## 已知缺口（待 #1610/#1611 实现）
- weapp 发货/收货跳转目标页面未注册（死链）→ #1610
- weapp 退款按钮缺失 → #1611
- ~~H5 员工取消订单按钮缺失~~ → ✅ 已修复（#1616，commit 682cdd4a）
- weapp 独立副本与共享版行为漂移 → #1611（薄壳化）

---
id: L-08
domain: lease
flow: 员工工作台（跨端入口）
steps:
  - seq: 1
    action: 员工打开工作台
    frontend:
      - platform: [weapp, h5]
        page: /profile
        role: [staff]
        gate: "isStaff（JWT role ≠ USER 且含 oid/tid）"
        reach: "底部导航 → 我的 → 员工工作台金刚区"
        controls: [乐器管理入口, 订单管理入口, 发货入口, 收货入口]
        displays: [员工角色标识, 工作台入口列表]
        ops: []
    api: {}
  - seq: 2
    action: 进入订单管理
    frontend:
      - platform: [weapp, h5]
        page: /staff/orders
        role: [staff]
        gate: ""
        reach: "员工工作台 → 订单管理"
        controls: [订单列表, 状态筛选, 分页加载]
        displays: [订单号, 状态, 租赁人, 创建时间]
        ops:
          - {type: api, method: GET, path: /merchant/orders}
    api: {method: GET, path: /merchant/orders, params: [page, pageSize, status]}
---

# L-08 员工工作台（跨端一致性）

## 前置条件
- 员工登录（site_admin/site_member），JWT 含 oid/tid
- H5 与 weapp Profile 页均已实现员工金刚区

## 流程
1. 底部导航「我的」→ 员工视角显示工作台金刚区（顾客视角显示顾客菜单）
2. 入口：乐器管理 / 订单管理 / 发货 / 收货（与 H5 员工金刚区一致）
3. 订单管理 → 订单列表（GET /merchant/orders，本网点订单）

## 关键规则
- H5 与 weapp 员工工作台入口一致（#1609 调查：weapp Profile 缺员工工作台）
- 员工订单列表在 weapp 需注册 staff-orders 页（#1611）
- 乐器管理在 weapp 需注册 staff-instruments 页（#1612）

## 验收
- checklist-verify：L-08 各 seq 的 page/controls/api 在 weapp+h5 均通过（#1610/#1611/#1612 实现后）

---

## 域级参考

- 状态机: `docs/state-machine.md`
- 结算公式: cases.md §2.7（迁移后见本文件历史）
- 展示原则: §2.8 合同快照 vs 实际结算分离

*Model: deepseek/deepseek-v4-flash*
