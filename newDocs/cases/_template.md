---
id: XX-00
domain: xxx
flow: 流程名
steps:
  - seq: 1
    action: 动作描述
    frontend:
      - platform: [weapp, h5, pc]
        page: /route
        role: [customer, staff, manager]
        gate: "门控条件或空串"
        reach: "来源页 → 触发操作 → 目标页"
        controls: [控件1, 控件2]
        displays: [字段1, 字段2]
        ops:
          - {type: api, method: POST, path: /api/path}
          - {type: interact}
          - {type: navigate, target: /target}
    api: {method: POST, path: /api/path, params: [param1, param2]}
---

# XX-00 流程名

## 前置条件
- 条件 1
- 条件 2

## 流程
1. 步骤 1 描述
2. 步骤 2 描述

## 关键规则
- 规则 1
- 规则 2

## 验收（对应 API 测试）
- `go test -run TestXxx ./handlers/ -v`
- 断言点列表

---

*Model: deepseek/deepseek-v4-flash*
