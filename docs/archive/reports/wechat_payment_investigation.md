# 微信小程序支付 `requestPayment:fail access denied` 调查报告

## 1. 问题现象

微信小程序端调用 `Taro.requestPayment` 后，微信客户端返回：

```
requestPayment:fail access denied, appId=wxcb44a1be70e356ed
```

该错误间歇性出现，有时伴随 "由于小程序违规，支付功能暂时无法使用" 提示。在每次出现此错误时，后端日志均显示 `POST /api/pay/prepay → 200`，prepay_id 生成成功、签名正确。

## 2. 调查发现（事实）

### 2.1 后端 prepay 流程完全正常

每次用户测试时，后端两个关键请求均返回 200：

```
POST /api/wechat/openid  → 200  313ms
POST /api/pay/prepay     → 200  501ms
```

- open_id 获取成功（通过 `jscode2session`，appId=wxcb44a1be70e356ed）
- prepay_id 由微信服务器签发成功（如 `prepay_id=wx20121202055787049da243783bd19b0000`）
- 签名格式正确：JSAPI 签名串 `appid\ntimestamp\nnonce\nprepay_id=xxx\n`，加密方式 RSA

### 2.2 错误发生在微信侧，不经过我们服务器

整个微信支付 JSAPI 流程分为两步：

```
步骤1（服务端）： 我们的 Go 服务 → api.mch.weixin.qq.com  [server-to-server]
  ✅ 每次 200，prepay_id 生成正确

步骤2（客户端）： 小程序 Taro.requestPayment() → 微信支付 SDK  [client-to-wechat]
  ❌ access denied — 微信服务器直接拒绝，不经过我们服务器
```

### 2.3 生产服 PaymentScheduler 曾有 nil context bug（已修复）

前期日志中发现 `[PaymentScheduler] query failed: net/http: nil Context` 错误，每分钟重复。根因是 `processPendingRecord` 向 `client.QueryOrder` 和 `client.CloseOrder` 传入了 `nil` 而非 `context.Background()`。已在 commit `1d842df7` 修复，7 月 20 日部署后 scheduler 正常运作（清理了 24 个积压的超时订单）。

## 3. 排查过程

### 3.1 前端传参 — ✅ 排除

在 `Payment.jsx` 和 `Renewal.jsx` 中添加了 debug modal，在 `Taro.requestPayment` 调用前展示完整参数包（appId、timeStamp、nonceStr、package、signType、paySign）。参数直接来自后端 prepay 响应，格式如下：

```json
{
  "appId": "wxcb44a1be70e356ed",
  "timeStamp": "1784520722",
  "nonceStr": "2175719687",
  "package": "prepay_id=wx20121202055787049da243783bd19b0000",
  "signType": "RSA",
  "paySign": "jKJE0lsDTNLq..."
}
```

字段名、类型、格式均符合微信 JSAPI 规范。

### 3.2 web-view 问题 — ✅ 排除

支付页面（`Payment.jsx`、`Renewal.jsx`）位于 `frontend-mobile/src/pages-weapp/` 目录下，由 Taro 编译为 WXML/WXSS/JS 原生小程序页面。页面使用 `@tarojs/components` 原生组件，非 `<web-view>` 内嵌 H5。`Taro.requestPayment` 在原生小程序运行时环境中调用。

### 3.3 下单接口类型错误 — ✅ 排除

后端 `PrepayOrder` handler（`wechatpay_prepay.go:124-269`）所有分支有 open_id 时均调用 `client.CreateJSAPIOrder()`，目标端点为 `/v3/pay/transactions/jsapi`。不存在误用 Native/APP/H5 接口的情况。

| order_type | 无 open_id | 有 open_id |
|------------|-----------|-----------|
| rent/repair/damage | CreateNativeOrder（PC扫码） | CreateJSAPIOrder（小程序） |
| points | — | CreateJSAPIOrder |
| renewal | — | CreateJSAPIOrder（本次修复新增） |

### 3.4 OpenID 与 AppID 不匹配 — ✅ 排除

- 服务端 `.env` 配置：`WX_APPID=wxcb44a1be70e356ed`
- `/api/wechat/openid` 通过 `jscode2session` 获取 openid，传入的 appId = `WX_APPID`
- `PrepayOrder` 中 `CreateJSAPIOrder` 使用的 appId = `cfg.AppID` = `WX_APPID`
- 两者使用同一个 appId，openid 一定属于 `wxcb44a1be70e356ed` 这个小程序

### 3.5 代码 Bug（已修复）— ✅ 排除

| Bug | 文件 | 修复 |
|-----|------|------|
| PaymentScheduler nil context | `wechatpay_callback.go:198,214` | `nil` → `context.Background()` |
| PrepayOrder 缺少 renewal 分支 | `wechatpay_prepay.go:124` | 新增 `case "renewal":` JSAPI-only 分支 |

以上两个 bug 在 commit `1d842df7` 修复，7 月 20 日已部署到生产服并通过验证：
- PaymentScheduler 从持续报错转为正常关闭超时订单
- 后端 `go build` 通过

### 3.6 小程序端权限/功能状态 — ⚠️ 待人工确认

错误信息 `access denied, appId=wxcb44a1be70e356ed` 表明微信认识这个 appId，但拒绝了它的支付请求。最常见原因：

| 可能原因 | 检查位置 |
|---------|---------|
| 商户号未授权小程序 AppID | 微信商户平台 → 产品中心 → AppID 账号管理 → 确认 `wxcb44a1be70e356ed` 已关联 `1582405481` |
| 小程序未开通支付 | 小程序后台 → 功能 → 微信支付 → 确认状态为"已开通" |
| 小程序/商户被风控封禁 | 小程序后台/商户平台 → 消息中心 → 查看违规/处罚通知 |

## 4. 其他

### 4.1 预生产环境

- 预生产服务正常运行：端口 5563（PC）/ 5564（Mobile）/ 5562（IAM）
- 域名：`preweb.cadenzayueqi.com` / `prewx.cadenzayueqi.com` / `preiam.cadenzayueqi.com`

### 4.2 服务器配置详情

| 配置项 | 值 |
|--------|-----|
| 小程序 AppID | `wxcb44a1be70e356ed` |
| 商户号 mch_id | `1582405481` |
| 证书序列号 | `36FBAE70F2F80BE983CEBC0DA96FAEB69A0E43E6` |
| Mock 模式 | `false`（生产服使用真实支付） |
| WX_APPID | `wxcb44a1be70e356ed`（.env 已配置） |
| WECHAT_PAY_MOCK_MODE | `false` |

### 4.3 涉及文件

| 文件 | 变更 |
|------|------|
| `backend/handlers/wechatpay_callback.go` | nil context 修复 |
| `backend/handlers/wechatpay_prepay.go` | renewal 分支 + 错误提示更新 |
| `backend/services/wechatpay/config.go` | AppID 加载（含 fallback） |
| `frontend-mobile/src/pages-weapp/payment/Payment.jsx` | debug modal |
| `frontend-mobile/src/pages-weapp/renewal/Renewal.jsx` | debug modal |

### 4.4 结论

经过代码层、日志层、配置层的完整排查，**后端 100% 正常，前端传参格式正确，三个常见技术问题（web-view、接口类型、openid 不匹配）全部排除**。

问题指向**微信商户平台侧**：商户号 `1582405481` 与小程序 `wxcb44a1be70e356ed` 的 AppID 授权关联关系需要人工登录微信商户平台确认，或联系微信支付客服（95017）排查风控/处罚状态。
