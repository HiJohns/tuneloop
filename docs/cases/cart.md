---
id: C-01
domain: cart
flow: 购物车管理
steps:
  - seq: 1
    action: 进入购物车
    frontend:
      - platform: [weapp, h5]
        page: /cart
        role: [customer, guest]
        gate: "已登录或游客均可（登录后自动合并游客购物车）"
        reach: "首页浮动购物车图标 / 详情页加入购物车后"
        controls: [乐器卡片, 选中框, 租期加减, 删除, 去结算]
        displays: [乐器图/SN/分类, 每日租金, 押金, 租期, 小计, 合计总额]
        ops:
          - {type: interact, feedback: "useDidShow 重新读取 storage（weapp 页面栈返回不重新 mount，必须刷新）"}
    api: {}
  - seq: 2
    action: 调整购物车项
    frontend:
      - platform: [weapp, h5]
        page: /cart
        role: [customer, guest]
        gate: ""
        reach: "购物车内操作"
        controls: [选中/取消选中, 租期加减, 删除按钮]
        displays: [合计总额实时更新, 已租出乐器灰显「已被租出」不可选]
        ops:
          - {type: interact, feedback: "所有变更即时写入 storage（getCartKey），并 eventBus.emit('cartUpdated') 同步首页角标"}
    api: {}
  - seq: 3
    action: 下单后购物车移除已下单项
    frontend:
      - platform: [weapp, h5]
        page: /checkout
        role: [customer]
        gate: "提交订单成功"
        reach: "确认订单页 → 确认支付 → batchCreate 成功"
        controls: []
        displays: []
        ops:
          - {type: api, method: POST, path: /user/orders/batch, gate: "成功 → storage 移除已下单项 + eventBus.emit('cartUpdated')"}
    api: {method: POST, path: /user/orders/batch, params: [items]}
---

# C-01 购物车管理

## 前置条件
- 购物车数据存本地 storage（key = `getCartKey()`：未登录 `cart` / 登录 `cart_<userId>`）

## 关键规则

### 数据刷新规则（静态检查点，来源 #1665 教训）
- **首页角标**：渲染时实时读 storage（`cartItemCount`）——下单后首页重新渲染即消失
- **购物车页**：weapp 页面栈返回（Checkout→支付→返回购物车）**不重新 mount**，必须 `useDidShow` 重新读 storage（Cart.jsx 已实现）
- **下单后同步**：Checkout `batchCreate` 成功 → 从 storage 移除已下单项 → `eventBus.emit('cartUpdated')` → 首页角标更新；购物车页下次 `useDidShow` 读到最新数据
- H5 每次进入重新 mount（useEffect 读 storage），行为等价

### 已租出乐器（#1659）
- 购物车挂载时逐个查询 `/public/instruments/:id` 的 `stock_status`，非 `available` → 卡片灰显「已被租出」+ 不可选中 + 从选中集合剔除
- 后端 CreateOrder 为硬边界（前端仅 UX）

### 登录合并（#1639）
- 登录后打开购物车自动合并游客 `cart` → `cart_<userId>`（去重：instrument_id + 租期），不再弹确认框
- 合并后选中集合必须同步（否则 grandTotal=0 → 去结算 disabled）

### 去结算（跨端导航，来源 #1665 教训）
- weapp 端跳转必须用完整路径 `/pages-weapp/checkout/index`（Taro shim 会把 `/checkout` 转成不存在的 `/pages/checkout/index`）
- 未登录 → 来源 B 分流（见 account-select.md P-05）

## 验收
- [ ] 下单成功后：首页角标消失 + 购物车页（返回/重进）不再显示已下单乐器
- [ ] weapp 从 Checkout 返回购物车页数据为最新（useDidShow）
- [ ] 已租出乐器灰显不可选，合计不包含
- [ ] 游客/登录购物车合并后选中同步，去结算可用

---

*Model: deepseek/deepseek-v4-pro*
