# WeChat Pay 集成架构设计

> 版本: v1 (2026-07-13)
> 状态: 设计稿，待评审

---

## 一、手动配置清单（微信开发平台）

### 1.1 商户平台 `pay.weixin.qq.com`

| # | 配置项 | 平台路径 | 用途 |
|---|--------|---------|------|
| 1 | **商户号 mchID** | 账户中心 → 商户信息 → 微信支付商户号 | 商户身份标识 |
| 2 | **APIv3 密钥** | 账户中心 → API 安全 → APIv3 密钥 → 设置 | 回调签名验证 |
| 3 | **商户证书** | 账户中心 → API 安全 → API 证书 → 下载证书 | API 请求签名 |
| 4 | **证书序列号** | 证书下载页显示（或用 `openssl x509 -in apiclient_cert.pem -noout -serial`） | Authorization 头 |
| 5 | **支付回调域名（域名白名单）** | 产品中心 → JSAPI 支付 → 开发配置 → 支付回调域名 | 填入 `wx.cadenzayueqi.com`（仅域名，非完整 URL）。**
完整回调 URL 由代码在调用微信统一下单/退款 API 时通过 `notify_url` 参数动态传入，不需也无法在平台预配置。** |
| 7 | **产品授权：JSAPI 支付** | 产品中心 → JSAPI 支付 → 申请开通 | 小程序内支付 |
| 8 | **产品授权：H5 支付** | 产品中心 → H5 支付 → 申请开通 | 微信外浏览器支付 |
| 9 | **产品授权：Native 支付** | 产品中心 → Native 支付 → 申请开通 | PC 端扫码支付 |

### 1.2 小程序后台 `mp.weixin.qq.com`

| # | 配置项 | 平台路径 | 说明 |
|---|--------|---------|------|
| 1 | **开通微信支付** | 功能 → 微信支付 → 开通 | 基础功能开关 |
| 2 | **关联商户号** | 功能 → 微信支付 → 关联商户号 | 输入 mchID 绑定 |
| 3 | **配置 request 合法域名** | 开发 → 开发设置 → 服务器域名 → request 合法域名 | 添加 `https://wx.cadenzayueqi.com`（已有） |

### 1.3 开放平台 `open.weixin.qq.com`（如涉及 H5 支付）

| # | 配置项 | 平台路径 | 说明 |
|---|--------|---------|------|
| 1 | **AppID 与商户号关联** | 管理中心 → 网站应用 → 开发 → 微信支付 → 关联商户号 | H5 支付需要开放平台 AppID 而非小程序 AppID |

### 1.4 产品授权（视业务需要）

| # | 产品 | 用途 | 是否需要额外申请 |
|---|------|------|:---:|
| 1 | JSAPI 支付 | 小程序内：租赁支付、报修支付、买点 | ✅ |
| 2 | H5 支付 | 移动端 H5：租赁支付 | ✅ |
| 3 | Native 支付 | PC 端扫码 | ✅ |
| 4 | **委托代扣** | 逾期自动扣款 | ✅ 需额外签约（人工审核） |
| 5 | **退款** | 押金退款、结算退款 | ✅ |

---

## 二、服务器端配置

新增 `.env` 变量（回调 URL 由 tuneloop 代码固定，不在 .env 中）：

```bash
# WeChat Pay API v3
WECHAT_PAY_MCH_ID=            # 商户号
WECHAT_PAY_API_V3_KEY=        # APIv3 密钥 (32位)
WECHAT_PAY_CERT_SERIAL_NO=    # 证书序列号
WECHAT_PAY_PRIVATE_KEY_PATH=  # 商户私钥路径 (apiclient_key.pem)
# AppID 复用现有 WX_APPID，不新增变量
```

---

## 三、架构总图

### 3.1 小程序支付流程（JSAPI — 租赁/报修/买点）

```
[用户] → 点击"支付"按钮
    │
    ▼
[前端] → POST /api/pay/prepay { order_id, type(rent|repair|points), amount }
    │
    ▼
[后端] → 校验订单状态、金额
    │ → 生成商户订单号 (out_trade_no)
    │ → 调用 WeChat Pay API: POST /v3/pay/transactions/jsapi
    │ → 接收 prepay_id
    │ → 签名生成 package 串
    │ → 返回 { prepay_id, nonceStr, timeStamp, signType, paySign }
    │
    ▼
[前端] → wx.requestPayment({ timeStamp, nonceStr, package, signType, paySign })
    │
    ▼
[微信客户端] → 弹出支付确认 → 用户确认 → 扣款成功
    │
    ▼
[微信支付服务端] → POST 回调到 https://wx.cadenzayueqi.com/api/wechatpay/notify
    │
    ▼
[后端] → 验签 → 更新订单状态 paid → 更新余额 → 记录 transaction_id
```

### 3.2 PC 扫码支付流程（Native）

```
[PC 前端] → 展示支付二维码（调用后端获取 code_url）
    │
[用户] → 用微信扫描二维码 → 支付
    │
[微信] → 回调同上
```

### 3.3 退款流程（押金退款 / 结算退款）

```
[后端] → POST /v3/refund/domestic/refunds
    │     { out_trade_no, out_refund_no, amount, reason }
    │
[微信] → 异步处理退款
    │
[微信] → POST 回调到 /api/wechatpay/refund-notify
    │
[后端] → 验签 → 更新退款状态
```

### 3.4 逾期自动扣款流程（委托代扣 — 远期）

```
[用户] → 首次使用前，需完成代扣签约
    │     wx.navigateToMiniProgram({ appId: 'wxbd687630cd02ce1d', path: 'pages/index/index' })
    │
[微信] → 签约回调 → 后端存储 contract_id
    │
[定时任务] → 扫描逾期订单
    │ → POST /v3/papay/transactions/apply-contract (委托代扣)
    │
[微信] → 扣款结果通知
```

---

## 四、代码模块规划

### 4.1 新增文件

```
backend/services/wechatpay/
├── client.go           — WeChat Pay HTTP Client（签名、请求、验签）
├── config.go           — 从 .env 加载配置
├── orders.go           — 统一下单（JSAPI / H5 / Native）
├── refunds.go          — 退款
└── notify.go           — 回调验签 + 解密

backend/handlers/
├── wechatpay_prepay.go      — POST /api/pay/prepay（前端获取支付参数）
└── wechatpay_callback.go    — POST /api/wechatpay/notify（支付回调）
                              — POST /api/wechatpay/refund-notify（退款回调）
```

### 4.2 新增数据库表

```sql
CREATE TABLE order_payment_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,             -- 多租户隔离
    org_id UUID,                         -- 网点隔离
    user_id UUID NOT NULL,               -- 支付用户
    order_id UUID,                       -- 关联的 order / repair_request / damage_report id
    order_type VARCHAR(20) NOT NULL,     -- 'rent' | 'repair' | 'points' | 'damage'
    out_trade_no VARCHAR(32) UNIQUE,     -- 商户订单号
    transaction_id VARCHAR(64),          -- 微信支付订单号（回调后填入）
    amount DECIMAL(10,2) NOT NULL,       -- 支付金额（元，调用微信 API 时 ×100 转分）
    type VARCHAR(20) NOT NULL,           -- 'payment' | 'refund' | 'auto_debit'
    status VARCHAR(20) DEFAULT 'pending',-- 'pending' | 'paid' | 'refunding' | 'refunded' | 'failed' | 'closed'
    method VARCHAR(20),                  -- 'jsapi' | 'h5' | 'native' | 'mock'
    prepay_id VARCHAR(64),               -- JSAPI prepay_id
    code_url TEXT,                       -- Native 二维码 URL
    fail_reason TEXT,                    -- 失败原因（审计追溯）
    raw_response JSONB,                  -- 微信回调原始 JSON（纠纷追溯）
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);

CREATE TABLE order_refund_records (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL,
    payment_record_id UUID NOT NULL,     -- FK → order_payment_records.id
    out_refund_no VARCHAR(32) UNIQUE,    -- 商户退款单号
    refund_id VARCHAR(64),               -- 微信退款单号
    amount DECIMAL(10,2) NOT NULL,       -- 退款金额（元）
    reason VARCHAR(200),                 -- 退款原因
    status VARCHAR(20) DEFAULT 'pending',-- 'pending' | 'refunding' | 'refunded' | 'failed'
    fail_reason TEXT,
    raw_response JSONB,                  -- 退款回调原始 JSON
    created_at TIMESTAMP,
    updated_at TIMESTAMP
);
```

### 4.3 需要修改的现有端点

| 端点 | 当前 | 改为 |
|------|------|------|
| `POST /api/user/orders` (CreateOrder) | 直接标 `status=paid` | 标 `status=reserved`，返回 `prepay_id` 让前端调支付 |
| `POST /api/orders/:id/pay` (PayOrder) | 模拟状态变更 | 调用 `/api/pay/prepay` 获取支付参数 |
| `POST /api/repair-requests/:id/pay` | 模拟状态变更 | 同上 |
| `POST /api/user/points/purchase` | 直接加余额 | 先创建支付记录，回调后再加余额 |
| `POST /api/appeals/:id/agree` (AgreeDamage) | damage ≥ deposit 直接标 completed | damage ≥ deposit 时创建支付记录，进入支付流程 |
| `PUT /api/appeals/:id/resolve` (ResolveAppeal) | 调整金额后直接完成 | 调整后若金额 > 押金，通知用户支付差额 |
| `deposit_refund_scheduler.go` | 改 DB 状态 | 调用 `POST /v3/refund/domestic/refunds` |
| `user_settlement.go` | 记录 `refund_status=pending` | 发起真实退款 |

### 4.4 路由注册注意事项

- `POST /api/wechatpay/notify` 和 `POST /api/wechatpay/refund-notify` 必须注册在 **`public` 路由组**（无鉴权中间件），因为微信服务器无法携带我们的 JWT
- `POST /api/pay/prepay` 注册在 `userOptionalAuth` 路由组（CUSTOMER token 可访问）
- `GET /api/admin/payments` 注册在 `authRequired` 路由组 + `RequireCusPerm("payment:read")`

### 4.5 SDK 选择

使用官方 `github.com/wechatpay-apiv3/wechatpay-go` SDK，而非从零实现签名/验签。SDK 提供：
- 自动签名 + Authorization 头构造
- 平台证书自动下载与轮换
- 回调通知验签 + AES-256-GCM 解密
- 统一下单 / 查询 / 关单 / 退款 封装

### 4.6 与 #1330 的冲突

#1330 移除了 MyLeases 中的 `reserved`（未支付）筛选器和"立即支付"按钮。WeChat Pay 集成后订单创建时状态为 `reserved`（等支付回调才变 `paid`），该状态回来了。**Phase 1d 必须回退 #1330 中对 MyLeases 的改动**，恢复 `reserved` 筛选器和支付按钮。

---

## 五、8 个对接点的支付方式选择

| # | 对接点 | 支付场景 | 方式 | 备注 |
|---|--------|----------|------|------|
| 1 | 租赁费用 | 小程序内 / H5 / PC | JSAPI / H5 / Native | 三端均可触发 |
| 2 | 报修费用 | 小程序内 | JSAPI | 仅小程序 |
| 3 | 逾期扣款 | 后台自动 | 委托代扣 | 需用户签约（远期） |
| 4 | 购买预付点 | 小程序内 | JSAPI | 注册引导 + 会员中心 |
| 5 | 押金退款 | 后台自动 | 退款 API | 原路退回 |
| 6 | 结算退款 | 后台自动 | 退款 API | 原路退回 |
| 7 | 报修增补差价 | 小程序内 | JSAPI | renegotiation quote |
| 8 | **定损赔偿** | 小程序内 | JSAPI | 押金不足覆盖时，用户支付差额；申诉调整后再次触发 |

### 5.8 定损赔偿支付流程（对接点 #8）

```
[员工] → 定损，输入赔偿额 damage_amount
    │
    ▼
[系统] → 通知用户（照片、评论、金额）
    │
    ▼
[用户] → 查看定损详情
    ├── 点击「同意」
    │   ├── damage_amount ≤ deposit → 自动扣除押金，退还差额 → deposit_refunding
    │   └── damage_amount > deposit → 进入支付确认页
    │       ├── 展示：定损金额、押金抵扣、应补差额
    │       ├── 允许输入预付点抵扣（与租赁结算页一致）
    │       ├── 现金差额 = damage_amount - deposit - prepaid_points_used
    │       └── 调用 WeChat Pay JSAPI 支付现金差额
    │           ├── 支付成功 → order.status = completed, 押金全额扣除
    │           └── 支付失败/超时 → 可重试，超时未完成转申诉
    │
    └── 点击「申诉」→ 输入理由 → 等待处理
        │
        ▼
    [网点/商户管理员] → 审核申诉
        ├── 「无损坏」→ 取消赔款，退还全部押金
        ├── 调整赔偿金额 → new_damage_amount
        │   ├── new_damage_amount ≤ deposit → 自动扣除，退还差额
        │   └── new_damage_amount > deposit → 通知用户支付新差额
        │       └── 用户收到最终定损通知 + 支付按钮 → 同上支付流程
        └── 确定维修 → 乐器进入维修状态
```

**关键设计点**：
- 支付确认页复用租赁结算页组件（费用明细 + 预付点输入 + 现金差额）
- `order_payment_records.order_type = 'damage'`，`order_id` 指向 DamageReport.ID
- 申诉调整后可能产生第二次支付（原始定损支付已发生的不可逆，新的按差额补/退）

---

## 六、.env 配置设计（含测试模式兼容）

### 6.1 环境变量清单

```bash
# ─── WeChat Pay API v3 ───
WECHAT_PAY_MCH_ID=                # 商户号，测试环境留空则走模拟模式
WECHAT_PAY_API_V3_KEY=            # APIv3 密钥，32 位随机字符串
WECHAT_PAY_CERT_SERIAL_NO=        # 商户证书序列号
WECHAT_PAY_PRIVATE_KEY_PATH=      # 商户私钥文件路径（apiclient_key.pem）
# AppID 复用现有 WX_APPID，不新增变量

# ─── 测试模式 ───
WECHAT_PAY_MOCK_MODE=true         # 测试环境下设为 true，走模拟支付流程
```

> **回调 URL 不是微信平台配置项**：完整 URL（如 `https://wx.cadenzayueqi.com/api/wechatpay/notify`）由 tuneloop 代码在调用微信统一下单和退款 API 时通过 `notify_url` 参数动态传递。微信商户平台只需要配置 **域名白名单**（仅 `wx.cadenzayueqi.com`，不加路径），见 §一。
```

### 6.2 测试模式设计

当 `WECHAT_PAY_MOCK_MODE=true` 或 `WECHAT_PAY_MCH_ID` 为空时：

- `POST /api/pay/prepay` → 直接返回 `{ mock: true, order_status: "paid" }`
- 前端收到 `mock: true` → 跳过 `wx.requestPayment`，直接跳转支付成功页
- 后端直接更新订单状态为 `paid`，写入 `order_payment_records`（标记 `method='mock'`）
- 回调端点 `/api/wechatpay/notify` 不启动

切换方式：
```
开发/测试环境: WECHAT_PAY_MOCK_MODE=true  （不设 MCH_ID 也行）
预生产/生产:   WECHAT_PAY_MOCK_MODE=false + 完整商户配置
```

---

## 七、支付核查体系

### 7.1 数据溯源链路

```
订单创建时:
  pricing_breakdown (JSONB)  → 每段阶梯、折扣、押金计算依据
  points_policy_snapshot      → 当时生效的点数上限政策

支付发生时:
  order_payment_records       → out_trade_no → transaction_id → amount → status

退款发生时:
  order_payment_records       → refund_id → refund_amount → refund_status

核对公式:
  total_amount (pricing_breakdown) = Σ order_payment_records.amount (type='payment')
                                  - Σ order_payment_records.amount (type='refund')
```

### 7.2 支付明细页面设计

**路由**: `/admin/payments`（PC 端）

**筛选条件**:
| 筛选项 | 类型 | 可选值 |
|--------|------|------|
| 时间范围 | date range | 开始日期 ~ 结束日期 |
| 支付类别 | multi-select | 租赁支付 / 报修支付 / 点数购买 / 退款 / 逾期扣款 |
| 支付方式 | select | 全部 / JSAPI / H5 / Native / 模拟（测试） |
| 支付状态 | select | 全部 / 待支付 / 已支付 / 退款中 / 已退款 / 失败 |
| 商户订单号 | text input | out_trade_no 精确搜索 |
| 微信交易号 | text input | transaction_id 精确搜索 |

**列表字段**:
| 列 | 来源 | 说明 |
|----|------|------|
| 时间 | `created_at` | 支付发起时间 |
| 商户订单号 | `out_trade_no` | 可点击跳转订单详情 |
| 微信交易号 | `transaction_id` | 回调后填入 |
| 类别 | `order_type` | 租赁 / 报修 / 点数 |
| 金额 | `amount` | ¥ |
| 方式 | `method` | JSAPI / Native / mock |
| 状态 | `status` | 颜色标签 |
| 操作 | — | 查单（调用微信查询接口） / 查看关联订单 |

### 7.3 对账能力

- 后端提供 `GET /api/admin/payments/export` 导出 CSV
- 字段：时间、商户单号、微信单号、类别、金额、方式、状态、关联订单 ID
- 支持按日期 + 类别筛选导出
- 与微信商户平台对账单交叉比对

---

## 八、支付失败处置

### 8.1 前端侧

```
wx.requestPayment 失败回调:
  → Taro.showModal({ title: '支付失败', content: errMsg })
  → 订单状态保持在 reserved，不跳转支付成功页
  → 用户可选择重试或取消

支付超时处理:
  → 30 秒无响应 → 提示"支付处理中，请稍后在订单详情中查看"
  → 提供"查询支付状态"按钮 → 调用 POST /api/pay/query
```

### 8.2 后端侧 — 防漏收

```
创建订单时:
  status = 'reserved'（不是 'paid'）

支付回调验签规则:
  1. 验签失败 → 拒绝更新，记录日志，状态保持 reserved
  2. 金额不匹配 → 拒绝更新，记录异常日志，触发告警
  3. 订单已支付（重复回调）→ 幂等处理，不重复入账
  4. 订单已取消 → 记录异常，发起自动退款

回调超时处理（定时任务，每分钟）:
  扫描 status=reserved 且 created_at > 30 分钟前的订单
  → 调用 POST /v3/pay/transactions/out-trade-no/{out_trade_no} 查询微信侧状态
  → 已支付 → 补更新状态
  → 未支付 → 标记 closed（超时关闭）
```

### 8.3 逾期扣款失败处置

```
扣款失败（委托代扣返回失败 / prepaid 余额不足）:
  1. 创建 OverdueCharge 记录 status='failed'
  2. 通知商户管理员（merchant_admin）：
     - 站内通知 + 邮件："订单 #{id} 逾期扣款失败，金额 ¥{amount}"
  3. 通知网点管理员（site_admin）：同上
  4. 在 OverdueAlerts 页面中标红显示
  5. 用户侧：
     - 小程序消息模板推送："您的租约已逾期，请尽快处理"
     - 订单详情页显示逾期金额 + "立即还款"按钮
```

---

## 九、分阶段实施建议

### Phase 1 — 核心支付（1-2 周）
- [ ] 搭建 `wechatpay` 服务模块（client + 签名 + 统一下单 + 回调验签）
- [ ] 完成 1. 租赁费用支付（JSAPI 小程序内 + Native PC 扫码）
- [ ] 支付回调 → 更新订单状态
- [ ] `order_payment_records` 建表

### Phase 2 — 报修 + 买点（1 周）
- [ ] 2. 报修费用支付
- [ ] 4. 购买预付点支付（会员中心 + 注册后引导）
- [ ] `POST /api/pay/prepay` 支持 `order_type=points`

### Phase 3 — 退款（1 周）
- [ ] 5. 押金退款
- [ ] 6. 结算退款
- [ ] 退款回调处理

### Phase 4 — 核查与告警（1 周）
- [ ] 4a. 支付明细页面（PC 端 `/admin/payments`）
- [ ] 4b. 支付失败前端处理
- [ ] 4c. 逾期扣款失败告警

### Phase 5 — 逾期代扣（远期，需额外商务申请）
- [ ] 5a. 委托代扣签约流程
- [ ] 定时任务集成自动扣款
- [ ] 余额不足通知 + 手动补缴入口

### Phase 6 — 小程序购物订单接入（1 周）
> 详见 Issue [#1404](https://github.com/HiJohns/tuneloop/issues/1404)

微信官方提供「小程序购物订单」功能，用户可在「我-小店与卡包-小程序购物订单」查看购物订单。

**步骤**:
1. **人工**：签署订单中心授权协议（微信公众平台）
2. **后端**：调用 `POST /wxa/sec/order/update_order_detail_path`，注册跳转路径
3. **后端**：CLI flag `--set-order-detail-path`，PATH=`pages-weapp/order-detail/index?out_trade_no=${商品订单号}`
4. **后端**：新增 `GET /api/orders/by-trade-no/:out_trade_no` 反查接口
5. **前端**：WeApp `OrderDetail.jsx` 支持 `out_trade_no` 参数
6. **所有**：支付 `description` 改为中文（已随此文档完成）

**验证**：
```bash
# 设置跳转路径
./service/tuneloop --set-order-detail-path "pages-weapp/order-detail/index?out_trade_no=${商品订单号}"

# 验证
curl POST "https://api.weixin.qq.com/wxa/sec/order/get_order_detail_path?access_token=$(your_token)"
```

---

## 八、2026-07-17 生产上线调试日志

### 8.1 平台证书下载失败

**问题**: `--download-platform-cert` 返回 `RESOURCE_NOT_EXISTS`

**原因**: 微信支付公钥模式下，`/v3/certificates` 端点不返回证书。商户需要在 `pay.weixin.qq.com` → API安全 → 申请微信支付公钥，上传商户自己的公钥。响应验签使用微信平台公钥（需从商户后台单独下载）。

**解决**: 
1. 从微信商户后台下载平台公钥 PEM 文件
2. 保存到 `WECHAT_PAY_PLATFORM_CERT_PATH` 指向的路径（默认 `/tmp/wechatpay_platform_cert.pem`）
3. 修改 `getPlatformCert()` 同时支持 X509 CERTIFICATE 和 PUBLIC KEY 两种 PEM 格式

### 8.2 平台证书 PEM 格式解析失败

**错误**: `x509: malformed serial number`

**原因**: 从微信下载的平台公钥是 `-----BEGIN PUBLIC KEY-----` 格式（PKIX），非 `-----BEGIN CERTIFICATE-----` 格式。`x509.ParseCertificate()` 无法解析原始公钥。

**解决**: 修改 `backend/services/wechatpay/signer.go:getPlatformCert()`，检查 PEM block type：
- `PUBLIC KEY` → `x509.ParsePKIXPublicKey()` 解析，包装为 `x509.Certificate{PublicKey: ...}`
- `CERTIFICATE` → 沿用 `x509.ParseCertificate()`

### 8.3 响应签名验证失败

**错误**: `crypto/rsa: verification error`

**原因**: 初次将商户自己的公钥（从 `apiclient_cert.pem` 提取）作为平台公钥使用。商户公钥用于 API 请求签名，微信平台公钥用于响应签名验证，两者不同。

**解决**: 将商户后台下载的微信平台公钥部署为 `WECHAT_PAY_PLATFORM_CERT_PATH`。

### 8.4 支付方式始终为 Native（非 JSAPI）

**现象**: `order_payment_records.method = 'native'`, `prepay_id` 为空

**原因**: 微信小程序支付需要 `open_id` 参数。前端 `POST /api/pay/prepay` 未传 `open_id`，后端走 Native 扫码支付路径。

**解决**:
1. **后端**: 新增 `POST /api/wechat/openid` 端点，调用 `https://api.weixin.qq.com/sns/jscode2session` 用 `wx.login()` 的 code 换取 openid。注册在 `backend/main.go:211`
2. **前端**: `frontend-mobile/src/pages-weapp/payment/Payment.jsx` 的 `handlePay` 函数中，先调用 `Taro.login()` 获取 code，再调 `/api/wechat/openid` 换 openid，传入 prepay 请求的 `open_id` 字段

### 8.5 requestPayment 报 access denied

**错误**: `requestPayment:fail access denied, appId=wxcb44a1be70e356ed`

**排查过程**:
1. 检查 prepay 返回参数：`prepay_id`, `time_stamp`, `nonce_str`, `package`, `sign_type`, `pay_sign` 全部有值 ✅
2. 检查小程序与商户关联：`mp.weixin.qq.com` → 微信支付 → 已关联 ✅
3. 检查 JSAPI 支付产品：`pay.weixin.qq.com` → 产品中心 → 已开通 ✅
4. 检查 AppID 账号管理：`pay.weixin.qq.com` → JSAPI支付 → AppID账号管理 → `wxcb44a1be70e356ed` 状态为"已关联"，账号类型为"小程序" ✅
5. 检查签名算法：RSA-SHA256，签名原文格式 `appId\ntimeStamp\nnonceStr\npackage\n` ✅
6. 检查 appId 一致性：`project.config.json` 和 `.env` 中的 `WX_APPID` 均为 `wxcb44a1be70e356ed` ✅

**状态**: 截至 2026-07-17，商户侧全部配置已确认正确，`requestPayment` 仍报 access denied。可能原因：
- 微信支付商户账户有待完成的验证或签约流程
- 公钥模式下的兼容性问题

### 8.6 关键代码位置

| 功能 | 文件 | 行号 |
|------|------|------|
| OpenID 交换端点 | `backend/handlers/wechat_bind.go` | `GetOpenID()` |
| OpenID 路由注册 | `backend/main.go` | 211 |
| JSAPI 签名生成 | `backend/services/wechatpay/real.go` | 64-81 |
| 平台证书解析 | `backend/services/wechatpay/signer.go` | 29-54 |
| 公钥提取 + 部署 | `docs/wechat-20260717.md` | 步骤 1 |
| 前置 prepay open_id | `frontend-mobile/src/pages-weapp/payment/Payment.jsx` | `handlePay()` |
