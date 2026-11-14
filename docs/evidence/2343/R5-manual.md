# R5 — 长列表探底对齐（人工）

- executor: user
- environment: 微信体验版 1.0.79（预生产 API）
- date: 2026-11-13
- 用户原话（Issue #2343 comment）：「三者我都确认了，没问题。截图就不上传了，特别是加载中动作很快，捕捉不到。」
- steps: 首页加载多页乐器 → 连续上滑到列表末尾 → 继续上滑
- expected: 最后一条卡片/「已全部加载」止于 tabBar 上沿，无法继续上滑
- actual: 符合预期
- screenshot: 用户明确豁免（见 Issue comment）；不伪造截图

RESULT: PASS
