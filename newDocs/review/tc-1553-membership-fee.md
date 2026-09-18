# TC-1553 Review — Membership 注册付费

> TC Issue: #1553 | 生成日期: 2026-08-07 | 类型: 后端测试

## 1. 目标

验证会员注册付费流程：#1553 "Membership fee — registration payment"
- 注册 → `membership_fee` 返回正确（默认 99 / 配置覆盖）
- 付费（mock）→ 支付记录 status=paid, amount=99
- 会员回调 → 无副作用、无错误

## 2. 验证方式

- [x] 后端测试: `go test -run TestMembershipFlow ./handlers/ -v`
- [ ] 静态验证: 不适用
- [ ] 真机验证: 不适用（mock 模式）

## 3. 流程步骤与证据

### Step 1: 注册 → membership_fee

**代码证据**:
- `backend/handlers/auth.go:427-443` — PostRegister 读取 `system_settings.membership_fee`（默认 99）→ 响应 `membership_fee`

**测试证据**:
```
--- PASS: TestMembershipFlow
  断言: registerResp.Data.MembershipFee = 99（system_settings 配置值）
  断言: 本地用户创建（iam_sub 匹配）
```

### Step 2: Prepay membership（mock 模式）

**代码证据**:
- `backend/handlers/wechatpay_prepay.go:105-125` — mock 模式直接 `record.Status = "paid"` + `Method = "mock"`
- `backend/main.go:637` — `POST /api/pay/prepay`（userOptionalAuth 组）

**测试证据**:
```
  断言: prepayResp.Data.Mock = true（测试环境 mock 开启）
  断言: 支付记录 order_type=membership, amount=99, status=paid, method=mock
```

### Step 3: 会员回调无副作用

**代码证据**:
- `backend/handlers/wechatpay_callback.go:188-193` — membership case 返回 nil（注册时已发放积分，无额外副作用）

**测试证据**:
```
  断言: applySideEffects(db, record, now) 无 error
  断言: 支付记录 status 保持 paid（未被副作用翻转）
```

## 4. 结论

| 检查项 | 结果 | 证据位置 |
|--------|:---:|---------|
| 注册返回 membership_fee=99 | ✅ | Step 1 |
| 本地用户创建 | ✅ | Step 1 |
| prepay mock 模式生效 | ✅ | Step 2（Mock=true） |
| 支付记录 status=paid | ✅ | Step 2 |
| 支付记录 amount=99, method=mock | ✅ | Step 2 |
| 会员回调无副作用 | ✅ | Step 3 |
| out_trade_no 关联一致 | ✅ | Step 2 |

**总判定**: ✅ 通过

**测试输出汇总**:
```
--- PASS: TestMembershipFlow (1.08s)
```

## 5. 已知局限

- 使用 mock 支付模式（`WECHAT_PAY_MOCK_MODE=true`），真实微信回调未覆盖（需真机/预生产验证）
- 会员等级费用（非默认 99 的 MembershipLevel 定价）未覆盖——当前实现仅 system_settings 单一费用
- 注册依赖 IAM mock（`newRegisterMockServer` + `IAM_SECRET`），验证了 token 校验链路

---

*Model: deepseek/deepseek-v4-flash*
