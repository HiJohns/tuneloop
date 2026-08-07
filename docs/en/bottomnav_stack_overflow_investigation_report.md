# BottomNav Page Stack Overflow Investigation Report

> Issue: #1558 | Date: 2026-08-07 | Role: Investigation Lead

## 1. Overview

- **Target module**: Custom bottom navigation (BottomNav) and page routing in the WeChat mini-program
- **Symptom**: After tapping the bottom 'Rent' or 'Home' tab 9 times in a row, tapping an instrument card stops working (no navigation to detail page)
- **Root cause**: Page stack overflow — each tap calls `Taro.navigateTo` which pushes one page onto the stack; WeChat mini-program limits the stack to **10 pages**, and `navigateTo` silently fails once full

## 2. Reproduction Path & Evidence Chain

### 2.1 Key Code Paths

**`frontend-mobile/src/app.config.ts`** — no `tabBar` configured:

```typescript
export default {
  pages: isWeapp ? weappPages : h5Pages,   // 17 pages, all regular pages
  window: { ... },                          // NO tabBar field!
}
```

Home, Rent, Service, Profile are **all regular pages**, not tabBar pages → only `navigateTo` is available, not `switchTab`.

**`frontend-mobile/src/pages-weapp/Home.jsx:116`** — tab navigation uses `navigateTo`:

```jsx
const nav = (url) => { Taro.navigateTo({ url }) }
...
{ key: 'home', icon: '🏪', label: '首页', onClick: () => nav('/pages-weapp/home/index') },
{ key: 'rent', icon: '🪕', label: '租赁', onClick: () => nav('/pages-weapp/my-leases/index') },
```

**`frontend-mobile/src/pages-weapp/MyLeases.jsx:55,294-297`** — same `navigateTo` pattern.

**`frontend-mobile/src/pages-weapp/Profile.jsx:83,329-332`** — same `navigateTo` pattern.

**`frontend-mobile/src/pages-weapp/Home.jsx:414`** — instrument card navigation:

```jsx
onClick={() => { const url = tenant ? `/pages-weapp/detail/index?id=${instrument.id}&tenant=${tenant}` : `/pages-weapp/detail/index?id=${instrument.id}`; nav(url) }}
```

### 2.2 Stack Depth Math (exactly matches "9 taps")

| Action | Page stack | Depth |
|--------|-----------|:---:|
| Launch into Home | [Home] | 1 |
| Tap 'Rent' | [Home, MyLeases] | 2 |
| Tap 'Home' | [Home, MyLeases, Home] | 3 |
| Tap 'Rent' | [..., MyLeases] | 4 |
| ... | ... | ... |
| **9th tab tap** | **[...]** | **10 = limit** |
| **Tap instrument card** | Need to push Detail → **11th layer → FAIL** | **no response** |

WeChat mini-program docs: `wx.navigateTo` page stack is limited to **10 pages**; exceeding the limit fails the navigation (fail callback, no user-visible message). The user's reported "9 taps" exactly matches 1+9=10.

### 2.3 Why "silent" failure

`Taro.navigateTo({ url })` passes no `success/fail` callback; WeChat silently drops over-limit navigations (only logs `navigateTo:fail webview count limit exceed` in console), invisible to the user on real devices.

## 3. Affected Files

| File | Lines | Problem |
|------|------|------|
| `frontend-mobile/src/app.config.ts` | 63-70 | Missing `tabBar` config; tab pages are regular pages |
| `frontend-mobile/src/pages-weapp/Home.jsx` | 116, 439-442 | `navigateTo` for tabs; card nav at 414 fails on full stack |
| `frontend-mobile/src/pages-weapp/MyLeases.jsx` | 55, 294-297 | `navigateTo` for tabs |
| `frontend-mobile/src/pages-weapp/Profile.jsx` | 83, 329-332 | `navigateTo` for tabs |

## 4. Related Findings (not the root cause, but relevant)

1. **Service tab points to unregistered page**: MyLeases.jsx:296 / Profile.jsx:331 target `/pages-weapp/my-repairs/index`, but `pages-weapp/` has **no** my-repairs page and `app.config.ts` does not register it → tapping Service tab always fails (affects every page with the Service tab).
2. **Re-pushing the same page**: Tapping 'Home' while on Home pushes a new Home instance (Home.jsx:439), accelerating stack exhaustion; Taro does not dedupe same-page navigateTo.
3. **H5 side (pages/)**: Uses react-router `navigate()`, not subject to the 10-page limit, but repeated `navigate('/')` accumulates history entries infinitely — same pattern, low severity.

## 5. Fix Recommendations (for later /analyze)

Ordered by recommendation priority:

### Option A: Native tabBar (recommended, WeChat standard)
- Add `tabBar` config to `app.config.ts` (list: Home/Rent/Service/Profile)
- Switch all tab navigations to `Taro.switchTab` (does not consume the page stack)
- Keep custom styling: `tabBar.custom: true` + `custom-tab-bar` component (requires migrating the custom UI)
- Note: `switchTab` does not support query params (tenant/category must go via storage/global state)

### Option B: Use `Taro.reLaunch` for tab jumps (minimal change)
- Change all tab onClick handlers to `Taro.reLaunch({ url })` — clears the stack then navigates
- Cost: full page rebuild on every tab switch (state/scroll position lost); reLaunch cannot push params

### Option C: Stack-depth guard (safety net; combine with A or B)
- Wrap `safeNav(url)`: `const pages = Taro.getCurrentPages(); if (pages.length >= 9) Taro.reLaunch({url}) else Taro.navigateTo({url})`
- Or `navigateBack` to the target tab if already in the stack (delta calculation)

**Recommendation**: A + C combined (native tabBar + stack-depth guard). If touching the tabBar structure is not desired short-term, at minimum implement C to stop the bleeding.

## 6. Appendix

### Sequence Diagram (normal vs overflow)

```
Normal:  Home ──navigateTo──▶ Detail           ✅
              (depth 1→2, within limit)

Overflow: Home →MyLeases→Home→MyLeases→...→Home  (depth 10)
              │
              └─navigateTo Detail ──▶ FAIL (silent) ❌ card unresponsive
```

### Fix Verification Checklist
- [ ] After 20+ rapid tab taps, instrument cards still navigate to detail
- [ ] Service tab navigates correctly (Option A requires registering the page)
- [ ] Verify on real device (simulators may not enforce the stack limit strictly)

---

*Model: deepseek/deepseek-v4-flash*
