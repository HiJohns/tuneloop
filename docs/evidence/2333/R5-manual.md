# R5 真机回执（#2333 评测页流程 + 两态禁用行为）

executor: user
date: 2026-11-15
environment: weapp 体验版（预生产后端 1.0.85 `4aa951308`；mock payscore，`PAYSCORE_ENFORCE=true`）

## 页面流程（实测）

| 步骤 | 观测 | 结论 |
|---|---|---|
| 确认页提交 → 评测页 | 直接进入评测页，页面显示「评测标的金额」（金额「—」已由服务单补建修复，`33f352f24`） | ✅ |
| 拉起/模拟评测 | mock provider 自动 approved（Owner：「模拟 评测可以的」） | ✅ |
| 通过 → 返回支付 | 「返回继续支付」回到支付页，金额与确认页所选档位一致（差额 ¥30.30） | ✅ |
| 页面形态 | weapp 无自绘标题栏（`{!env.isMiniProgram && …}`），符合 `ui.md` | ✅ |

## 两态禁用行为

| 信用态 | 期望行为 | 依据 |
|---|---|---|
| `approved` | 支付可用，金额按订单档位 | 真机实测（差额 ¥30.30 支付页一致） |
| `pending` / `rejected` / `expired` | 支付被禁用：`creditGate()` 返回 `canPay=false`，支付页展示评测入口与原因；服务端 `AssertOrderPayableIfEnforced` 返回 403 / `40320` | 单测 `creditFlow.test.mjs`（8/8 含 rejected→`blocked`）+ 后端 `go test ./handlers/ -run '235[27]'` |

> Owner 裁定（2026-11-15）：全额档同样要求 approved，故三种信用非通过态一律禁用支付，文案不引导「改用全额」。

RESULT: PASS
