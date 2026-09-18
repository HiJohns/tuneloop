---
id: R-03
domain: repair
flow: 租赁乐器维修（员工/师傅）
steps:
  - seq: 1
    action: 定损判定损坏并进入待维修
    frontend:
      - platform: [weapp, h5]
        page: /staff/receiving（归还验收）
        role: [staff]
        gate: "订单状态 = returning"
        reach: "订单 → 归还验收 → 提交"
        controls: [验收条件（good/damaged）, 定损金额, 备注, 照片]
        displays: [乐器信息, 租赁信息, 出库照片对比, 拍照规格]
        ops:
          - {type: api, method: PUT, path: /api/warehouse/orders/:id/return-inspect}
    api: {method: PUT, path: /warehouse/orders/:id/return-inspect, params: [instrument_sn, scan_time, condition, notes, photos, damage_amount]}
  - seq: 2
    action: 师傅扫码开始维修
    frontend:
      - platform: [weapp, h5]
        page: /repair
        role: [repair_technician, staff]
        gate: "repair_status = repair_pending"
        reach: "维修中心 → 扫码/列表 → 维修工作台"
        controls: [开始维修]
        displays: [乐器信息, 定损信息]
        ops:
          - {type: api, method: POST, path: /api/repair/:id/start}
  - seq: 3
    action: 维修过程记录（拍照 + 评论）
    frontend:
      - platform: [weapp, h5]
        page: /repair
        role: [repair_technician]
        gate: "repair_status = repair_in_progress 且当前用户为负责人"
        reach: "维修工作台 → 添加记录"
        controls: [评论输入, 拍照/相册（≤10 张）, 缩略图预览, 提交记录]
        displays: [维修记录列表（时间 · 提交人 · 照片网格）]
        ops:
          - {type: api, method: POST, path: /api/upload}
          - {type: api, method: POST, path: /api/repair/:id/records}
  - seq: 4
    action: 维修完成（需至少一条含照片记录）
    frontend:
      - platform: [weapp, h5]
        page: /repair
        role: [repair_technician]
        gate: "repair_status = repair_in_progress 且当前用户为负责人"
        controls: [维修完成]
        ops:
          - {type: api, method: POST, path: /api/repair/:id/complete}
    api: {method: POST, path: /repair/:id/complete, params: []}
  - seq: 5
    action: 接手（其他师傅）
    frontend:
      - platform: [weapp, h5]
        page: /repair
        role: [repair_technician]
        gate: "repair_status = repair_in_progress 且当前用户非负责人；同站点（R4）"
        controls: [接手]
        displays: [当前负责人姓名]
        ops:
          - {type: api, method: POST, path: /api/repair/:id/takeover}
  - seq: 6
    action: 员工验收
    frontend:
      - platform: [weapp, h5]
        page: /repair
        role: [staff]
        gate: "repair_status = repair_completed；乐器所在站点员工；且非本单维修人（R1）"
        reach: "维修中心 → 已修复乐器 → 验收面板"
        controls: [验收通过, 验收不通过（必填原因）]
        displays: [乐器信息, 维修记录（含照片）]
        ops:
          - {type: api, method: POST, path: /api/repair/:id/accept}
          - {type: api, method: POST, path: /api/repair/:id/reject}
---

# R-03 租赁乐器维修（员工/师傅）

## 前置条件

- 归还验收判定损坏（或定损提交 damaged）→ 乐器 `repair_status = repair_pending`、`stock_status = maintenance`，**R2**：`current_site_id` = 操作员站点
- 验收人须为乐器所在站点 `site_admin/site_member` 且非本单维修负责人（R1）

## 流程

1. 预约上门/到店维修：定损 damaged → `repair_pending`
2. 师傅扫码 → 开始维修（`repair_in_progress`，记录 `repair_worker_id`）
3. 维修过程：评论 + 照片记录（`repair_records`）
4. 维修完成（至少一条含照片记录）→ `repair_completed`
5. 员工验收：通过 → `available`（清空 repair 字段）；不通过 → `repair_in_progress`（原因入记录，R3）
6. 其他师傅可同站点接手（R4）；改派仅 `site_admin`（R5）

## 关键规则

- **R1 禁止自验收**：含兼职（site_member + repair_technician）
- **R2** 进入维修写 `current_site_id`；无站点则验收被拒（需数据修复）
- **R3** 验收驳回原因写入 `repair_records`（comment 前缀「验收驳回：」）
- **R4/R5** 接手限同站点；改派限 `site_admin`
- **R6** 遗留维保工单（maintenance_tickets）已废弃
- 权限码：`repair:start` / `repair:complete` / `repair:accept`（接线见 #1882）
- 维修记录照片：JSON 数组（URL/key 兼容）；历史假文件名不渲染

## 验收（对应 API 测试）

- `go test -run TestCompleteRepair_ ./handlers/ -v`（完成校验：含照片/无记录/空数组/非负责人/非维修中）
- `go test -run TestResolveOperatorSiteID|TestInspectReturn_Damaged_ ./handlers/ -v`（R2 站点写入）
- `go test -run TestRepairRecords_ ./handlers/ -v`（worker_name 按 IAM sub 解析）
- 规划：`TestAcceptRepair` / `TestRejectRepair`（R1 自验收拒绝/驳回落记录，见 #1882）

---

*Model: deepseek/deepseek-v4-flash*
