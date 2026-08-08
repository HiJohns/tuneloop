# 租赁域用例（Lease Domain）

> 来源: cases.md §2 迁移 | 用例编号: L-01 ~ L-04 | YAML 前置块供 AI 静态检查

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
        controls: [点数抵扣滑块, 确认支付按钮]
        displays: [租金, 押金, 应付总额]
        ops:
          - {type: api, method: POST, path: /pay/prepay}
          - {type: navigate, target: /success}
    api: {method: POST, path: /pay/prepay, params: [order_id, order_type, amount]}
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
        controls: [归还按钮, 物流信息输入]
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
        gate: "订单状态 = completed"
        reach: "订单详情 → 费用信息/收支明细"
        controls: [费用信息区, 合同快照展开]
        displays: [实际租期, 实际租金, 退款明细, 退款合计]
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
        controls: [归还按钮, 物流信息输入]
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
        controls: [归还按钮, 物流信息输入]
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

---
id: L-04
domain: lease
flow: 定损与申诉
steps:
  - seq: 1
    action: 验收发现损坏（员工）
    frontend:
      - platform: [pc]
        page: /staff/receiving
        role: [staff]
        gate: "订单状态 = returning"
        reach: "PC 收货界面 → 扫码 → 验收"
        controls: [扫码输入, 拍照, 条件选择(damaged), 定损金额输入, 评论输入]
        displays: [乐器信息, 租赁信息]
        ops:
          - {type: api, method: PUT, path: /warehouse/orders/:id/return-inspect}
    api: {method: PUT, path: /warehouse/orders/:id/return-inspect, params: [instrument_sn, scan_time, condition, damage_amount, notes]}
  - seq: 2
    action: 收到定损通知（顾客）
    frontend:
      - platform: [weapp, h5]
        page: /order-detail
        role: [customer]
        gate: "订单状态 = pending_damage_response"
        reach: "通知/订单详情 → 定损信息"
        controls: [同意按钮, 申诉按钮]
        displays: [定损照片, 定损金额, 评论]
        ops:
          - {type: api, method: POST, path: /orders/:id/accept-damage}
          - {type: api, method: POST, path: /orders/:id/reject-damage}
    api: {method: POST, path: /orders/:id/accept-damage, params: []}
  - seq: 3
    action: 申诉处理（经理）
    frontend:
      - platform: [pc]
        page: /appeals
        role: [manager]
        gate: "存在 pending appeal"
        reach: "PC 申诉列表 → 处理"
        controls: [无损坏按钮, 调整金额输入, 说明输入, 确定按钮]
        displays: [乐器信息, 租金, 图片, 员工定损说明, 顾客申诉理由]
        ops:
          - {type: api, method: PUT, path: /appeals/:id/resolve}
    api: {method: PUT, path: /appeals/:id/resolve, params: [result, amount, notes]}
---

# L-04 定损与申诉

## 前置条件
- 乐器归还验收时发现损坏

## 流程
1. 员工验收 damaged → 订单 pending_damage_response，乐器进维修
2. 顾客收到定损通知 → 同意（扣押金/补付）或申诉
3. 申诉 → 经理查看 → 无损坏（取消赔款）/调整金额 → resolve
4. resolve 后结算退款（`R = Ra + De - Dd - O - Re`）

## 关键规则
- 押金足以覆盖：自动扣定损；不足：进入支付页
- 支付失败/超时：按申诉处理
- 申诉记录 + 通知（ResolveAppeal 创建 notification）

---

## 域级参考

- 状态机: `docs/state-machine.md`
- 结算公式: cases.md §2.7（迁移后见本文件历史）
- 展示原则: §2.8 合同快照 vs 实际结算分离

*Model: deepseek/deepseek-v4-flash*
