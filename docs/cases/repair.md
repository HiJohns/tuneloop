---
id: R-01
domain: repair
flow: 客户报修（v3 主流程）
steps:
  - seq: 1
    action: 创建报修单
    frontend:
      - platform: [weapp, h5]
        page: /create-repair
        role: [customer]
        gate: "已登录"
        reach: "我的 → 报修 → 创建报修"
        controls: [识别码输入(500ms防抖), SN, 类型, 品牌, 型号, 商户选择, 照片上传]
        displays: [SN 回填信息, 商户/网点选项]
        ops:
          - {type: api, method: POST, path: /repair-requests}
    api: {method: POST, path: /repair-requests, params: [sn, instrument_type, brand, model, merchant_id]}
  - seq: 2
    action: 查看报价并接受
    frontend:
      - platform: [weapp, h5]
        page: /repair-request/:id
        role: [customer]
        gate: "状态 = pending_assessment"
        reach: "报修单 → 报价列表"
        controls: [报价卡片, 接受报价按钮]
        displays: [材料费, 服务费, 物流费, 工期, 评论]
        ops:
          - {type: api, method: POST, path: /repair-requests/:id/quotes/:qid/accept}
          - {type: navigate, target: /payment}
    api: {method: POST, path: /repair-requests/:id/quotes/:qid/accept, params: []}
  - seq: 3
    action: 支付维修费
    frontend:
      - platform: [weapp, h5]
        page: /payment
        role: [customer]
        gate: "状态 = pending_payment"
        reach: "接受报价 → 支付"
        controls: [点数抵扣, 优惠码输入, 确认支付按钮]
        displays: [材料费, 服务费, 物流费, 合计, 优惠后金额]
        ops:
          - {type: api, method: POST, path: /pay/prepay}
    api: {method: POST, path: /pay/prepay, params: [order_type=repair, order_id, amount, coupon_code]}
  - seq: 4
    action: 填写寄送物流
    frontend:
      - platform: [weapp, h5]
        page: /repair-request/:id
        role: [customer]
        gate: "状态 = pending_ship"
        reach: ""
        controls: [物流公司, 单号输入]
        displays: [收货地址(系统给), 中转地址+转入单号]
        ops:
          - {type: api, method: PUT, path: /repair-requests/:id/return-shipping}
    api: {method: PUT, path: /repair-requests/:id/return-shipping, params: [courier, tracking_number]}
  - seq: 5
    action: 确认收货
    frontend:
      - platform: [weapp, h5]
        page: /repair-request/:id
        role: [customer]
        gate: "状态 = returned"
        reach: ""
        controls: [确认收货按钮]
        displays: [维修结果]
        ops:
          - {type: api, method: POST, path: /repair-requests/:id/complete}
    api: {method: POST, path: /repair-requests/:id/complete, params: []}
---

# R-01 客户报修（v3）

## 前置条件
- 顾客已登录，自有乐器

## 流程
1. 创建报修（SN 防抖回填 → 唯一性 → 选商户）
2. 待估价：查看报价（受控仅见单号）→ 择一接受
3. 待付款：支付（材料+服务+物流）
4. 待发送：填物流 → 网点收货 → 维修中
5. 已发回 → 确认收货 → 评价/申诉

## 关键规则
- 5 工作日未接受报价 → 自动关闭（到期前 24h 提醒）
- 合作商户经中转网点扇出竞价
- 报价单跨网点互不可见

## 验收
- `go test -run TestIntegration_Scenario3_MaintenanceProcess ./handlers/ -v`

---
id: R-02
domain: repair
flow: 维修报价调整（requote）
steps:
  - seq: 1
    action: 收到重新报价通知
    frontend:
      - platform: [weapp, h5]
        page: /repair-request/:id
        role: [customer]
        gate: "状态 = repairing 且存在 requote"
        reach: "通知 → 报修单"
        controls: [新报价卡片, 接受按钮, 拒绝按钮(待前端接入)]
        displays: [原报价, 新报价, 差额, 材料/服务/物流费对比]
        ops:
          - {type: api, method: POST, path: /repair-requests/:id/quotes/:qid/accept}
          - {type: navigate, target: /payment}
    api: {method: POST, path: /repair-requests/:id/quotes/:qid/accept, params: []}
---

# R-02 维修报价调整

## 前置条件
- 维修中师傅重新报价（仅一次）

## 流程
1. 师傅 requote → 通知顾客
2. 顾客接受 → 补差款 → 维修继续
3. 拒绝 → 回退结算 → return_pending

## 关键规则
- 拒绝后仅付检查费+物流+中转费，多余退款封底 0
- 关联 Issue: #1577（新旧报价对比确认页）

---

*Model: deepseek/deepseek-v4-flash*
