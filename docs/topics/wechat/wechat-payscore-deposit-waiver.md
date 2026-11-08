# 微信支付分免押金接入调研

> 版本: v1（2026-11-04）
> 状态: 调研完成，待商务确认与立项
> 关联: 免押金体系（`docs/spec/features.md` §免押金体系，#1867 / #1925）

## 1. 结论摘要

1. **微信不向商户开放个人信用分数值**。微信支付分官方 FAQ 明确：「商户有办法获取用户支付分分数吗？没有，商户无法获取用户分数。」商户只能获得**单笔订单的综合评估结果（通过 / 不通过）**，无法获得分数，也拿不到「分数不足」的明确标识。
2. 客户提出的「获取微信个人信用分作为免押标准」，可落地的等价能力是 **微信支付分「免押租借」场景**：由商户在微信侧配置最低分数门槛，微信逐单综合评估，通过即免押。**判定权在微信，不在我方**。
3. 这是**商务准入 + 技术接入**的组合。商务前置是最大不确定性：需以商户号向微信支付发邮件申请支付分权限，审核约 10 个工作日，拿到 `service_id` 后才能开发；再加开发、验收、上线，整体通常 1–2 个月。
4. 本项目已有微信支付 v3 基础设施（`backend/services/wechatpay/`），生产 `.env` 已配置商户号与证书，技术上是"新增一条支付分通道"，不是从零建设支付能力。

## 2. 名词澄清

| 名称 | 现状 | 能否取得分数 |
|------|------|-------------|
| **微信支付分** | 现行产品，财付通提供，用于免押租借 / 先用后付 | 否。商户只能获得单笔评估结果 |
| **腾讯信用分** | 2018 年公测一天即被监管叫停，已下线 | 否。不存在可用接口 |
| **芝麻信用分（支付宝）** | 另一体系，`zhima.credit.score.get` 可返回 `zm_score` | 需支付宝应用、芝麻产品签约、用户 `auth_zhima` 授权；与当前微信生态不一致 |
| **第三方风控分**（同盾 / 腾讯天御等） | 商业风控产品 | 返回的是风控评分，并非"微信信用分"，且需用户授权与合规评估 |

> 因此，若业务方要求「读取微信分数并按 600 分判定」，该需求在微信侧不成立，必须改为「支付分评估通过即免押」，或改用其他征信 / 风控数据源。

## 3. 微信支付分「免押租借」能力边界

官方产品介绍：<https://pay.weixin.qq.com/doc/v3/merchant/4012587050>

| 维度 | 说明 |
|------|------|
| 产品模式 | **需确认模式**：每笔订单跳转支付分小程序，用户授权确认，微信实时综合评估 |
| 先免模式 | 评估通过则免押金；不通过则用户缴押金，服务结束后退还剩余 |
| 先享模式 | 评估通过则先服务后付款；不通过则不能使用 |
| 商户可配置 | 服务 ID 的**最低分数门槛**（标准 550，可向运营申请调整）、**风险金额上限** |
| 单笔可传 | `risk_fund.amount`（本单押金 / 风险金额，须不超过服务 ID 上限）、服务描述、后付费项目 |
| 用户侧入口 | 微信 > 我 > 服务 > 钱包 > 支付分（可查看分数、授权服务、订单记录） |
| 费用 | 接入不收费，仅承担正常微信支付交易手续费 |
| 免确认模式 | **灰度测试、暂未开放**（官方页面更新于 2026-10-22），目前只能逐单确认 |

**评估机制**：综合评估 = 用户画像（支付分、历史 / 待支付订单、信用记录、实名稳定性）+ 商户参数（`risk_fund.amount`、服务描述）。即使达到门槛也可能「综合评估未通过」，业务必须有押金兜底分支，且不得把用户截图分数作为系统放行依据。

## 4. 商务前置（必须先完成）

官方接入准备：<https://pay.weixin.qq.com/doc/v3/merchant/4012587112>

1. **商户号**：可使用现有微信支付商户号（生产已配置 `WECHAT_PAY_MCH_ID`）。
2. **经营类目**：营业执照需包含与"物品租赁 / 乐器租赁"相关类目；申请什么场景就只能用于什么场景，不可跨场景。
3. **客服电话认证**：需通过微信支付认证并展示在账单详情页。
4. **无违规、客诉率达标**。
5. **发送申请邮件**：`weixinpay_scoreBD@tencent.com`
   - 标题格式：`【申请微信支付分】-乐器租赁免押-<商户号>-<主体名称>-普通商户-<日期>-是否需要人脸能力`
   - 正文使用官方「支付分权限申请邮件模板」。
6. 审核通过后在商户平台签署协议，运营提供 **`service_id`（32 位）**、**分数门槛**、**风险金额上限**。
7. 在商户平台配置**测试微信号白名单**（上线前仅白名单可用）。
8. 开发完成后提交**验收材料**：第一人称流程录屏、第三方视角设备 / 场景录屏、三种订单状态详情截图；验收通过才开放线上用户。

需重点向微信运营确认：

- 乐器租赁是否属于可接入的「免押租借」类目；
- 单笔风险金额上限能否覆盖乐器押金（这是最容易卡住的点）。

## 5. 技术实现方案

### 5.1 接入时序（需确认模式）

```
用户在下单页选择「微信支付分免押」
        |
        v
后端 POST /v3/payscore/serviceorder
   out_order_no / appid / service_id
   risk_fund = { name: "DEPOSIT", amount: 押金分 }
   post_payments（计费规则 / 预估费用）
   time_range / notify_url / need_user_confirm=true
        |
        v 返回 package（1 小时内有效）
小程序：wx.openBusinessView({ businessType: 'wxpayScoreUse', extraData: { package } })
H5（微信内）：jweixin-1.5.0 -> wx.invoke('openBusinessView', { businessType, queryString })
        |
        v 用户确认 -> 微信综合评估
   +- 通过：state=DOING / collection.state=USER_CONFIRM
   |        回调通知 / 主动查单 -> 标记本单「支付分免押已生效」，开始履约
   +- 不通过：用户返回，走缴押金正常下单
        |
        v 归还 / 结算
后端 POST /v3/payscore/serviceorder/{out_order_no}/complete
   total_amount（实际应收：租金 / 逾期 / 赔偿，须不超过 risk_fund.amount）
        |
        v 微信自动扣款（零钱 / 零钱通 / 银行卡）
   支付成功回调 -> 记录 transaction_id -> 对账
   未扣款成功：可查询、可下调金额、可取消、可同步已支付状态
```

### 5.2 关键接口（仅 API v3）

| 用途 | 接口 |
|------|------|
| 创建支付分订单 | `POST /v3/payscore/serviceorder` |
| 查询订单 | `GET /v3/payscore/serviceorder?out_order_no=` |
| 完结订单（触发扣款） | `POST /v3/payscore/serviceorder/{out_order_no}/complete` |
| 修改金额（仅下调） | `POST /v3/payscore/serviceorder/{out_order_no}/modify` |
| 取消订单 | `POST /v3/payscore/serviceorder/{out_order_no}/cancel` |
| 同步已支付 | `POST /v3/payscore/serviceorder/{out_order_no}/sync` |
| 退款 | `POST /v3/refund/domestic/refunds`（使用 `transaction_id`，不能用 `out_order_no`） |
| 回调 | 确认订单回调、支付成功回调（APIv3 验签 + AES-GCM 解密） |

- 创建订单：<https://pay.weixin.qq.com/doc/v3/merchant/4012587900>
- 开发指引：<https://pay.weixin.qq.com/doc/v3/merchant/4012587166>
- 常见问题（明确无法获取分数）：<https://pay.weixin.qq.com/doc/v3/merchant/4012587200>
- 接入易错点：<https://pay.weixin.qq.com/doc/v3/merchant/4054257620>

### 5.3 需要新增 / 改造的数据与状态

建议把「免押」从布尔值升级为**担保方式枚举**，否则现有 `deposit_waived` 无法区分「内部免押」与「支付分免押」：

```
orders 新增（或独立 order_payscore 表）：
  deposit_guarantee_type   cash | internal_waiver | wechat_payscore
  payscore_out_order_no
  payscore_state           CREATED / DOING / DONE / REVOKED / EXPIRED
  payscore_collection_state
  payscore_risk_amount_cents
  payscore_package_expire_at
  payscore_user_confirmed_at
  payscore_transaction_id
  payscore_charged_cents
```

订单状态需增加「等待支付分用户确认」的中间态。现有 `reserved` 是同步下单即成立，支付分是异步确认，不能混用。

### 5.4 与本项目现有实现的对接点

| 位置 | 现状 | 改造 |
|------|------|------|
| `backend/handlers/deposit_waiver.go` | 同步判定：实名 + 学生 / 教师 + 内部信用分 + 担保人 + 推荐信 | 增加「支付分通道」作为**并列标准（OR）**；`EvaluateDepositWaiverEligibility` 不能只看本地分数 |
| `backend/handlers/user_rental.go:274` | 下单前同步 gate，`deposit_waived=true` 直接押金置 0 | 支付分通道改为**两阶段**：先建单 + 建支付分订单，确认回调后再置免押 |
| `backend/services/wechatpay/` | 已有 v3 签名、回调解密、JSAPI / Native / H5、退款 | 新增 `payscore.go`（创建 / 查询 / 完结 / 取消 / 修改 / 同步）+ 回调事件路由，复用 `signer.go` / `callback_crypto.go` |
| `frontend-mobile/src/pages/Checkout.jsx` | 一个「免押金」复选框 | 改为渠道选择：内部免押 / 微信支付分免押；支付分需拉起确认页并轮询 / 查单 |
| `docs/spec/features.md` | 明确「外部信用分接入 out of scope」 | 若立项，需补 `## Core Update` 并同步 API / DB / UI 文档（中英） |

> 现有 `DepositMode` 字段已标注「已弃用，勿写入新值」，不要复用它承载支付分。

### 5.5 H5 与小程序差异（必须两端验证）

| 端 | 拉起方式 | 额外要求 |
|----|---------|---------|
| 微信小程序 | `wx.openBusinessView({ businessType: 'wxpayScoreUse', extraData: { package } })` | 小程序版本库 >= 2.6.0；微信 >= 7.0.3 |
| H5（微信内浏览器） | 引入 `jweixin-1.5.0.js`，`wx.invoke('openBusinessView', { businessType, queryString })` | 微信 >= 7.0.5；`queryString` 签名细节存在文档版本差异（旧版要求 `mch_id / package / timestamp / nonce_str / sign_type / sign`，HMAC-SHA256，需 **APIv2 密钥**），接入时以联调为准 |
| 微信外浏览器 | 不支持支付分拉起 | 只能引导到小程序，或走普通押金 |

当前仓库缺口：`.env` 只有 APIv3 相关配置，**没有 APIv2 密钥**；H5 若按旧版签名规范需补 `WECHAT_PAY_API_V2_KEY`。小程序端不需要 v2 密钥。

### 5.6 结算与退款链路影响

- 免押订单不再有「押金退款」，改为：归还时若有损坏 / 逾期，调用**完结订单**收取（不超过风险金额），剩余风险额度不扣款。
- 支付分扣款成功生成的是微信支付交易（账单交易类型 `AUTH`），退款必须使用 `transaction_id`。
- 现有 `deposit_refund_scheduler`、结算公式、`deposit_refunded` 语义都要区分「现金押金」与「支付分信用担保」，否则会出现「免押却走押金退款」的错误分支。
- 回调丢失时必须**主动查单兜底**（官方要求定时查询 `DOING` 订单）。

## 6. 风险与合规

1. **拿不到分数**：产品 / 运营若承诺「按分数判定」会与微信实际能力冲突；页面也不得写死「550 分可用」（官方接入易错点明确禁止）。
2. **评估不通过是常态**：风险金额越高通过率越低；必须有缴押金兜底，且不能中断交易。
3. **同一实名进行中订单一般不超过 3 笔**；超时未支付订单过多会限制创单，需做前置检查与提示。
4. **免确认模式未开放**：每单都要跳转确认，体验弱于「一次授权长期有效」。
5. **合规**：不得收集 / 保存用户支付分截图作为自动放行依据；用户授权与个人信息处理需符合《个人信息保护法》。
6. **风险金额上限**：乐器押金可能数千元，需确认服务 ID 上限；超限只能降低风险金额或改用其他担保。
7. **双端差异**：H5 若依赖 v2 密钥与 jweixin，需额外配置并做真机验证。

## 7. 工作量与周期（粗估）

| 阶段 | 内容 | 周期 |
|------|------|------|
| 商务申请 | 邮件、类目 / 客服认证、10 工作日审核、签约、获取 service_id | 3–6 周（不可控） |
| 后端 | payscore client、回调、状态机、两阶段下单、结算 / 退款改造、对账、单测 | 8–12 人日 |
| 前端 | Checkout 渠道选择、小程序拉起、H5 拉起、查单轮询、失败兜底 | 4–6 人日 |
| 联调验收 | 测试号白名单、三种状态、录屏验收、上线配置 | 1–2 周 |
| 合计 | | 约 1.5–2 个月（商务顺利时） |

> 按项目规则，改动超过 3 个文件应拆分。建议拆为「商务 / 配置」「后端支付分通道」「前端双端拉起」「结算与退款改造」「验收上线」5 个子任务。

## 8. 方案对比与建议

| 方案 | 说明 | 建议 |
|------|------|------|
| A. 微信支付分免押 | 作为内部免押之外的并列标准；通过即免押，不通过缴押金 | 推荐。先发商务申请，确认类目 / 风险上限，同时做技术预研 |
| B. 维持内部信用分 | 现有 `credit_score` 体系 | 若客户只是想要「多一个标准」，可先说明微信不开放分数 |
| C. 芝麻信用分 | 能取得分数，但需支付宝生态 + 签约 + 授权 | 与当前微信小程序 / H5 主线不一致，不建议 |
| D. 第三方风控分 | 可取得评分，但非「微信信用」 | 需合规评估，作为兜底备选 |

**下一步**：先由业务方确认两件事：

1. 乐器租赁能否以「免押租借」类目申请支付分；
2. 单笔可接受的风险金额上限。

确认后再立项做技术接入。技术侧可先并行完成 `wechatpay/payscore` 客户端与回调骨架（不依赖 `service_id` 也可用 mock 测试）。

## 9. 参考链接

- 产品介绍：<https://pay.weixin.qq.com/doc/v3/merchant/4012587050>
- 开发指引：<https://pay.weixin.qq.com/doc/v3/merchant/4012587166>
- 开发接入准备（权限申请）：<https://pay.weixin.qq.com/doc/v3/merchant/4012587112>
- 创建支付分订单：<https://pay.weixin.qq.com/doc/v3/merchant/4012587900>
- 常见问题（无法获取分数）：<https://pay.weixin.qq.com/doc/v3/merchant/4012587200>
- 接入易错点：<https://pay.weixin.qq.com/doc/v3/merchant/4054257620>
- JSAPI 调起支付分确认订单页：<https://pay.weixin.qq.com/doc/v3/merchant/4012587945>
- 小程序调起支付分-确认订单：<https://pay.weixin.qq.com/doc/v3/merchant/4012587949>
- 免确认模式（灰度未开放）：<https://pay.weixin.qq.com/doc/v3/merchant/4012646720>
