---
id: T-01
domain: transit
flow: 中转发货（受控商户）
steps:
  - seq: 1
    action: 中转处理
    frontend:
      - platform: [pc]
        page: /staff/transit
        role: [transit_site_member]
        gate: "中转会话待处理"
        reach: "中转工作流 → 待处理列表"
        controls: [中转服务费输入, 中转物流费输入, 扫码, 拆箱, 拍照]
        displays: [中转会话信息, 扇出网点列表]
        ops:
          - {type: api, method: PUT, path: /forwarding/sessions/:id/ready}
    api: {method: PUT, path: /forwarding/sessions/:id/ready, params: [service_fee, logistics_fee]}
    related:
      - {method: GET, path: /forwarding/sessions}
      - {method: PUT, path: /forwarding/sessions/:id/ship}
      - {method: PUT, path: /forwarding/sessions/:id/receive}
      - {method: PUT, path: /forwarding/sessions/:id/last-mile}
      - {method: PUT, path: /forwarding/sessions/:id/complete}
      - {method: PUT, path: /forwarding/sessions/:id/lost}
      - {method: GET, path: /transit-routes}
      - {method: GET, path: /common/sites/nearby}
---

# T-01 中转发货

## 流程
1. 中转员工填中转费 → 扇出受控网点
2. 实物中转：扫单号/拆箱/拍照/重装/转发

## 关键规则
- 受控商户发货走 in_transit 状态
- 中转费仅本网点可见

## 验收
- `go test -run TestScenarioB_ControlledMerchantForwarding ./handlers/ -v`

---

*Model: deepseek/deepseek-v4-flash*
