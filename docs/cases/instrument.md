---
id: I-01
domain: instrument
flow: 乐器录入
steps:
  - seq: 1
    action: 进入录入页
    frontend:
      - platform: [pc]
        page: /instruments/new
        role: [tenant_admin, site_admin, site_member]
        gate: "拥有 instrument:create 权限"
        reach: "导航栏 → 乐器列表 → 新建乐器"
        controls: [新建乐器按钮]
        displays: []
        ops:
          - {type: navigate, target: /instruments/new}
  - seq: 2
    action: 填写基本信息
    frontend:
      - platform: [pc]
        page: /instruments/new
        role: [tenant_admin, site_admin, site_member]
        gate: ""
        reach: ""
        controls: [识别码输入, 分类树选择, 级别下拉, 归属网点选择, 描述输入, 动态属性控件, 媒体上传]
        displays: [SN 唯一性校验结果, 属性候选值, 已上传媒体]
        ops:
          - {type: api, method: GET, path: /instruments/check}
          - {type: api, method: GET, path: /instruments/levels}
          - {type: api, method: GET, path: /instruments/filter-options}
          - {type: api, method: GET, path: /instruments/:id/pricing-v2}
          - {type: api, method: GET, path: /instruments/:id/media}
          - {type: api, method: DELETE, path: /instruments/:id/media/:batch_id}
          - {type: api, method: POST, path: /media/upload}
          - {type: interact}
  - seq: 3
    action: 提交
    frontend:
      - platform: [pc]
        page: /instruments/new
        role: [tenant_admin, site_admin, site_member]
        gate: "SN 必填 + 分类必填 + level_id 必填"
        reach: ""
        controls: [提交按钮, 取消按钮]
        displays: []
        ops:
          - {type: api, method: POST, path: /instruments}
          - {type: navigate, target: /instruments}
    api: {method: POST, path: /instruments, params: [sn, category_id, level_id, site_id, base_daily_rate, pricing]}
    related:
      - {method: GET, path: /categories}
      - {method: GET, path: /categories/:id/children}
      - {method: POST, path: /categories}
      - {method: PUT, path: /categories/sort}
      - {method: PUT, path: /categories/:id}
      - {method: DELETE, path: /categories/:id}
      - {method: GET, path: /instrument-photo-specs/:category_id}
      - {method: GET, path: /properties}
      - {method: GET, path: /properties/:id/options/search}
      - {method: POST, path: /instruments/:id/photos/upload}
      - {method: GET, path: /instruments/:id/photos/latest}
      - {method: GET, path: /instruments/:id/activity-log}
      - {method: PUT, path: /instruments/:id/promo-overrides}
      - {method: DELETE, path: /instruments/:id/media/key/:storage_key}
---

# I-01 乐器录入

## 前置条件
- 角色为租户管理员/网点管理员/网点成员
- 拥有 `instrument:create` 权限

## 流程
1. 乐器列表 → 新建乐器
2. 填 SN（自动校验唯一）、分类、级别、网点、描述、动态属性、媒体
3. 提交 → POST /instruments → 返回列表

## 关键规则
- SN 唯一性：输入即校验（GET /instruments/check）
- 属性别名自动映射（"yamaha"→"雅马哈"）
- 新属性值进入 pending 状态待审批
- 网点锁定：网点管理员/成员锁定当前网点

## 验收
- `go test -run TestInstrumentCRUD ./handlers/ -v`

---
id: I-02
domain: instrument
flow: 批量导入
steps:
  - seq: 1
    action: 下载模板
    frontend:
      - platform: [pc]
        page: /instruments
        role: [tenant_admin, site_admin]
        gate: "拥有 instrument:create"
        reach: "乐器列表 → 批量导入"
        controls: [批量导入按钮, 模板下载链接]
        displays: []
        ops:
          - {type: navigate, target: /instruments/import}
  - seq: 2
    action: 上传 CSV 校验
    frontend:
      - platform: [pc]
        page: /instruments/import
        role: [tenant_admin, site_admin]
        gate: ""
        reach: ""
        controls: [CSV 文件选择, 校验结果表格]
        displays: [错误行高亮(重复/缺失)]
        ops:
          - {type: api, method: POST, path: /instruments/batch-import/validate}
          - {type: interact}
  - seq: 3
    action: 确认导入
    frontend:
      - platform: [pc]
        page: /instruments/import
        role: [tenant_admin, site_admin]
        gate: "无错误行"
        reach: ""
        controls: [确认导入按钮]
        displays: [成功X条, 失败Y条明细]
        ops:
          - {type: api, method: POST, path: /instruments/batch-import}
    api: {method: POST, path: /instruments/batch-import, params: [csv, media_zip]}
    related:
      - {method: GET, path: /instruments/batch-import/template}
      - {method: POST, path: /instruments/batch-import/preview}
      - {method: POST, path: /instruments/batch-import/media}
      - {method: GET, path: /instruments/import/template}
      - {method: POST, path: /instruments/import}
      - {method: GET, path: /instruments/export}
      - {method: POST, path: /instruments/batch-pricing}
      - {method: PUT, path: /instruments/:id/status}
      - {method: PUT, path: /instruments/:id/display-image}
      - {method: PUT, path: /instruments/:id/cover-image}
      - {method: DELETE, path: /instruments/:id/scrap}
---

# I-02 批量导入

## 前置条件
- 租户管理员/网点管理员
- CSV 模板 + 媒体文件包

## 流程
1. 下载模板 → 填写
2. 上传 CSV → 校验（重复/缺失高亮）
3. 在线纠错（双击单元格）
4. 上传媒体 ZIP（识别码_序号.jpg）
5. 确认导入 → 事务创建 → 成功/失败明细

## 关键规则
- 识别码重复阻止导入
- 部分成功支持单独重试

---
id: I-03
domain: instrument
flow: 游客浏览
steps:
  - seq: 1
    action: 打开首页
    frontend:
      - platform: [weapp, h5]
        page: /
        role: [guest]
        gate: "无需登录"
        reach: "打开链接或扫码"
        controls: [乐器卡片列表, 分类筛选, 网点筛选, 级别筛选, 可租状态筛选]
        displays: [乐器图片, 名称, 租金, 网点]
        ops:
          - {type: api, method: GET, path: /public/instruments}
          - {type: api, method: GET, path: /public/instruments/search}
          - {type: api, method: GET, path: /public/instruments/lookup}
          - {type: api, method: GET, path: /public/categories}
          - {type: api, method: GET, path: /public/banners}
          - {type: api, method: GET, path: /public/sites}
          - {type: api, method: GET, path: /public/merchants}
          - {type: api, method: GET, path: /public/config}
    api: {method: GET, path: /public/instruments, params: [tenant?, page, pageSize]}
  - seq: 2
    action: 浏览详情
    frontend:
      - platform: [weapp, h5]
        page: /instrument/:id
        role: [guest]
        gate: "无需登录"
        reach: "首页卡片 → 详情"
        controls: [乐器图片, 立即租赁按钮]
        displays: [租金政策, 级别选择, 网点位置, 服务权益]
        ops:
          - {type: api, method: GET, path: /public/instruments/:id}
          - {type: api, method: GET, path: /public/instruments/:id/pricing-v2}
          - {type: api, method: GET, path: /public/instruments/:id/display-media}
    api: {method: GET, path: /public/instruments/:id, params: []}
  - seq: 3
    action: 下单入口
    frontend:
      - platform: [weapp, h5]
        page: /instrument/:id
        role: [guest, customer]
        gate: "未登录跳登录，已登录进结算"
        reach: "详情 → 立即租赁"
        controls: [立即租赁按钮]
        displays: []
        ops:
          - {type: navigate, target: /login 或 /checkout}
---

# I-03 游客浏览

## 前置条件
- 未登录用户

## 流程
1. 打开首页（可带 tenant 参数 → 按商户过滤）
2. 浏览乐器列表/详情
3. 点立即租赁 → 未登录跳登录 / 已登录进结算

## 关键规则
- 无 tenant 参数：返回所有租户乐器
- 有 tenant 参数：仅该商户

---
id: I-04
domain: instrument
flow: 购物车管理
steps:
  - seq: 1
    action: 加入购物车
    frontend:
      - platform: [weapp, h5]
        page: /instrument/:id
        role: [guest, customer]
        gate: ""
        reach: "详情页 → 加入购物车"
        controls: [加入购物车按钮, 悬浮购物车图标, 数量角标]
        displays: [购物车件数角标]
        ops:
          - {type: interact}
  - seq: 2
    action: 购物车批量下单
    frontend:
      - platform: [weapp, h5]
        page: /cart
        role: [customer]
        gate: "已登录"
        reach: "购物车 → 去结算"
        controls: [复选框, 删除按钮, 去结算按钮]
        displays: [网点分组, 每项租金/押金, 网点小计, 合计]
        ops:
          - {type: api, method: POST, path: /user/orders/batch}
          - {type: navigate, target: /payment}
    api: {method: POST, path: /user/orders/batch, params: [items, delivery_address]}
---

# I-04 购物车管理

## 前置条件
- 游客或已登录用户

## 流程
1. 详情页加入购物车 → 角标更新
2. 购物车勾选 → 批量下单 → 支付

## 关键规则
- 登录后购物车合并（§1.6）
- 已失效乐器置灰 + 一键清理

---

*Model: deepseek/deepseek-v4-flash*
