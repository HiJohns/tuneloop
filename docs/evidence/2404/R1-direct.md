# R1-direct — #2404 runtime evidence (dev, headless)

executor: dev
component-call-site: src/pages/DirectMemberManagement.jsx

```text
RUNTIME PASS: title rendered exactly once (已创建 · 初始密码仅显示一次); password row rendered
SOURCE PASS: src/pages/DirectMemberManagement.jsx exactly one title (已创建 · 初始密码仅显示一次)
SOURCE PASS: src/components/MerchantMemberManagement.jsx exactly one title (已创建 · 初始密码仅显示一次)
SOURCE PASS: src/components/SiteMemberManagement.jsx exactly one title (已创建 · 初始密码仅显示一次)
SOURCE PASS: src/pages/StaffManagement.jsx exactly one title (已创建 · 初始密码仅显示一次)
```

断言：弹窗标题「已创建 · 初始密码仅显示一次」在 headless Chromium DOM 中出现且仅 1 次；调用点恰好一个 title 属性且等于统一文案。

RESULT: PASS
