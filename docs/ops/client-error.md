# 客户端 JS 错误上报（H5）

> 关联：#2320。目标：把生产 H5 的未捕获 JS 错误变成可检索信号，避免只靠用户反馈发现崩溃。
> 范围：仅 H5。微信小程序 `Taro.onError` 另立 Issue。

## 1. 数据流

```
H5 window.onerror / unhandledrejection
  → frontend-mobile/src/App-H5.jsx（脱敏 + 同 message+route 每分钟 1 次）
  → navigator.sendBeacon 优先，fetch keepalive 兜底
  → POST /api/client-error（userOptionalAuth，匿名可调）
  → audit_logs（action=client_error, resource_type=frontend）
  → journalctl 同时输出 [client-error]
```

## 2. 请求契约

`POST /api/client-error`

```json
{
  "message": "TypeError: t is not a function",
  "stack": "at Checkout (Checkout.jsx:521)",
  "route": "/checkout?id=xxx",
  "version": "1.0.78",
  "user_agent": "Mozilla/5.0 ..."
}
```

| 字段 | 上限 | 说明 |
|------|-----:|------|
| `message` | 1 KB | 必填；为空/非 JSON → 400 |
| `stack` | 8 KB | 超限 → 400 |
| `route` | 512 B | 超限 → 400 |
| `version` | 64 B | 超限 → 400 |
| `user_agent` | 500 B | 超限 → 400 |

- 成功/限流：`204 No Content`（不回显堆栈）。
- 无效/超长：`400`，`{"code":40001,"message":"..."}`。
- 落库失败：`500`，仅记录服务端日志。
- 请求体总大小上限 16 KB。

## 3. 限流

- 进程内（单实例）按 `ip + sha256(message)` 计数，窗口 1 分钟，最多 1 条。
- 超限静默丢弃（仍返回 204），避免前端重试放大。
- 多实例部署时各实例独立计数；如需全局限流，后续加 Redis/网关层。

## 4. 落库字段（audit_logs）

| 列 | 值 |
|----|----|
| `action` | `client_error` |
| `resource_type` | `frontend` |
| `resource_id` | route（截断 100 字符） |
| `status` / `status_code` | `error` / `500` |
| `details` | JSON：`{route, version, message, stack}` |
| `tenant_id` / `user_id` | 上下文值；匿名用零 UUID `00000000-0000-0000-0000-000000000000` |
| `org_id` | 上下文有则写 |
| `ip_address` / `user_agent` | 请求来源 |

**脱敏**：`message`/`stack` 在落库/日志前过滤 Bearer token、`token/access_token/refresh_token/phone/id_card/id_card_no/password=...`、11 位手机号、18 位身份证号。前端同样先过滤一次；后端不接收 token/phone 等额外字段。

## 5. 排查命令

```bash
# 最近 24h 客户端错误
ssh cadenza "sudo journalctl -u tuneloop --since '24 hours ago' --no-pager | grep '\[client-error\]' | tail -50"

# 按版本定位
ssh cadenza "sudo journalctl -u tuneloop --since '24 hours ago' --no-pager | grep '\[client-error\]' | grep 'version=1.0.78'"

# 查库（生产 tuneloop；预生产 tuneloop_pre_snapshot）
ssh cadenza "docker exec <pg-container> psql -U tuneloop_user -d tuneloop -c \"SELECT created_at, resource_id, details, user_agent FROM audit_logs WHERE action='client_error' ORDER BY created_at DESC LIMIT 20\""
```

`details->>'message'` 用于聚合同一错误；`details->>'version'` / `resource_id` 用于定位发布版本与页面。

## 6. 已知边界

- 只覆盖 H5；小程序端错误需 `Taro.onError`（另立）。
- 不替代 Sentry/APM；无 sourcemap 还原，`stack` 为压缩后的位置。
- 前端每分钟同 message+route 只发一次，短时间内大量不同错误仍可能产生多行。
