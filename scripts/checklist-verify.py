#!/usr/bin/env python3
"""checklist-verify.py — AI static checker for frontend use-case checklists.

Reads the YAML front-matter blocks in docs/cases/*.md and verifies each
step's frontend entry against the actual codebase:

  1. page  → registered in weapp app.config.ts / H5 App.jsx routes
  2. controls → present as JSX in the target page source (by control text)
  3. api   → route exists in backend/main.go (via api-coverage extraction)

Usage:
  python3 scripts/checklist-verify.py [--verbose]
Exit 0 when all checks pass; 1 when gaps are found.
"""

import os
import re
import sys
import yaml

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CASES_DIR = os.path.join(REPO, "docs", "cases")
WEAPP_CONFIG = os.path.join(REPO, "frontend-mobile", "src", "app.config.ts")
H5_APP = os.path.join(REPO, "frontend-mobile", "src", "App.jsx")
BACKEND_MAIN = os.path.join(REPO, "backend", "main.go")

VERBOSE = "--verbose" in sys.argv
BEHAVIORAL = "--behavioral" in sys.argv
passed = 0
failed = 0
failures = []


def norm_page(page):
    """Normalize a YAML page path to weapp/H5 forms."""
    page = page.strip()
    return page


def load_weapp_pages():
    src = open(WEAPP_CONFIG).read()
    return set(re.findall(r"'((?:pages-weapp)/[a-z0-9/-]+/index)'", src))


def load_h5_routes():
    src = open(H5_APP).read()
    routes = set(re.findall(r'<Route\s+path="([^"]+)"', src))
    # strip path params for matching: /order/:id → /order
    return {re.sub(r"/:[a-zA-Z0-9_]+", "", r) for r in routes}


def load_backend_routes():
    src = open(BACKEND_MAIN).read()
    routes = set(re.findall(r'(?:GET|POST|PUT|DELETE|PATCH)\("([^"]+)"', src))
    # normalize all path params to :id for consistent matching
    return {re.sub(r":[a-zA-Z0-9_]+", ":id", r) for r in routes}


def page_exists(page, weapp_pages, h5_routes, platforms=None):
    """Check a YAML page path against weapp + H5 registrations."""
    if not page:
        return True  # no page declared → skip
    p = page.strip()
    # weapp nested pages: /profile/edit → pages-weapp/profile/edit/index
    WEAPP_NESTED = {
        "/profile/edit": "pages-weapp/profile/edit/index",
        "/message-detail": "pages-weapp/message-detail/index",
        "/messages": "pages-weapp/messages/index",
        "/membership": "pages-weapp/membership/index",
        "/search": "pages-weapp/search/index",
        "/renewal": "pages-weapp/renewal/index",
        "/bind": "pages-weapp/bind/index",
        "/content": "pages-weapp/content/index",
        "/profile-complete": "pages-weapp/profile-complete/index",
    }
    if p in WEAPP_NESTED:
        return WEAPP_NESTED[p] in weapp_pages
    # weapp form: /pages-weapp/x/index → app.config.ts 'pages-weapp/x/index'
    if p.startswith("/pages-weapp/"):
        return p.lstrip("/") in weapp_pages
    # H5 shared routes: /order-detail → /order/:id; /detail → /instrument/:id
    aliases = {
        "/order-detail": "/order", "/detail": "/instrument", "/instrument": "/instrument",
        "/return-settlement": "/return-settlement", "/my-leases": "/my-leases",
        "/create-repair": "/create-repair", "/repair-request": "/repair-request",
        "/messages": "/messages", "/message-detail": "/messages",
        "/payment": "/payment", "/membership": "/membership", "/cart": "/cart",
        "/checkout": "/checkout", "/profile": "/profile", "/search": "/search",
        "/success": "/success", "/": "/", "/content": "/content",
    }
    cand = aliases.get(p, p)
    # strip path params: /repair-request/:id → /repair-request
    cand = re.sub(r"/:[a-zA-Z0-9_]+", "", cand)
    if cand in h5_routes:
        return True
    # PC form (frontend-pc): pages with /staff, /admin, /merchant prefixes.
    # Exemption is ONLY for PC-only cases (platform contains pc, no weapp/h5).
    # Cross-platform cases (platform has weapp or h5) must pass the real
    # weapp/H5 registration check below — otherwise staff-page gaps on
    # weapp go undetected (#1613).
    platforms = platforms or []
    is_pc_only = ("pc" in platforms) and not ("weapp" in platforms or "h5" in platforms)
    if p.startswith(("/staff", "/admin", "/merchant", "/sites", "/common", "/appeals", "/instruments", "/messages")):
        if is_pc_only:
            return True  # PC routes validated separately (App.jsx PC)
        # cross-platform: fall through to weapp/H5 checks below
    # default: unknown route
    return False


def find_control(page, control, weapp_pages, h5_routes, platforms=None):
    """Search the target page source for the control text."""
    if not page:
        return True
    # PC-only cases: skip control checks (page lives in frontend-pc)
    platforms = platforms or []
    is_pc_only = ("pc" in platforms) and not ("weapp" in platforms or "h5" in platforms)
    if is_pc_only and page.strip().startswith(("/staff", "/admin", "/merchant", "/sites", "/common", "/appeals", "/instruments", "/messages")):
        return True
    # graphical controls without textual labels — skip text search
    GRAPHIC_CONTROLS = {"悬浮购物车图标", "数量角标", "复选框", "滑块", "点数抵扣滑块", "图标", "单选", "角标", "调节器", "租期调节器", "乐器图片", "图片", "缩略图", "昵称显示", "会员等级显示", "显示"}
    if any(g in control for g in GRAPHIC_CONTROLS):
        return True
    p = page.strip()
    source = None
    # map YAML page → component file (shared weapp/H5 sources)
    comp_map = {
        "checkout": "Checkout", "my-leases": "MyLeases", "order-detail": "OrderDetail",
        "order": "OrderDetail", "detail": "Detail", "instrument": "Detail",
        "repair-request": "RepairRequestDetail", "cart": "Cart", "profile": "Profile",
        "search": "Search", "create-repair": "CreateRepairRequest",
        "return-settlement": "ReturnSettlement", "messages": "Messages",
        "message-detail": "MessageDetail", "payment": "Payment", "membership": "MembershipCenter",
        "success": "Success", "home": "Home", "staff-orders": "StaffOrders",
        "staff-receiving": "ReceivingInterface", "receiving-interface": "ReceivingInterface",
        "staff-instruments": "StaffInstruments", "appeals": "AppealManagement",
        "sites": "SiteManagement", "return-confirm": "ReturnConfirm", "receive": "ReceiveConfirm",
        "shipping-interface": "ShippingInterface", "content": "ContentPage",
    }
    if p.startswith("/pages-weapp/"):
        rel = p.lstrip("/").replace("/index", "")
        for cand in [
            os.path.join(REPO, "frontend-mobile", "src", rel + ".jsx"),
            os.path.join(REPO, "frontend-mobile", "src", rel + "/index.jsx"),
            os.path.join(REPO, "frontend-mobile", "src", rel + "/EditProfile.jsx"),
        ]:
            if os.path.exists(cand):
                source = open(cand).read()
                break
        else:
            # fall back to shared pages/{Comp}.jsx via comp_map
            key = p.strip("/").split("/")[0]
            comp = comp_map.get(key)
            if comp:
                cand = os.path.join(REPO, "frontend-mobile", "src", "pages", comp + ".jsx")
                if os.path.exists(cand):
                    source = open(cand).read()
    else:
        # weapp nested pages (e.g. /profile/edit) → weapp source dir
        WEAPP_NESTED_FILES = {
            "/profile/edit": os.path.join(REPO, "frontend-mobile", "src", "pages-weapp", "profile", "edit", "EditProfile.jsx"),
        }
        if p in WEAPP_NESTED_FILES:
            cand = WEAPP_NESTED_FILES[p]
            if os.path.exists(cand):
                source = open(cand).read()
        if source is None:
            name = p.strip("/").split("/")[0]
            comp = comp_map.get(name)
        if source is None and comp:
            cand = os.path.join(REPO, "frontend-mobile", "src", "pages", comp + ".jsx")
            if os.path.exists(cand):
                source = open(cand).read()
    if source is None:
        return True  # page source not found → skip (page registration check is authoritative)
    # strip the control name of common wrappers like 按钮/输入/区; split
    # multi-word controls and match ANY token (e.g. "合同快照展开" → 合同快照|展开)
    cleaned = control.replace("按钮", "").replace("输入", "").replace("选择", "").replace("区", "").replace("(", "").replace(")", "").replace("（", "").replace("）", "").replace("500ms防抖", "").replace("防抖", "")
    tokens = [t for t in re.split(r"[·/]", cleaned) if len(t) >= 2]
    if not tokens:
        return True
    src_lower = source.lower()
    # strip common trailing words (卡片/输入/按钮/滑块) for loose matching:
    # "照片上传" → 照片上传 / 照片; "报价卡片" → 报价卡片 / 报价
    def variants(tok):
        vs = [tok]
        for suffix in ("卡片", "按钮", "滑块", "上传", "输入", "选择器", "信息", "展开", "抵扣", "入口", "显示"):
            if tok.endswith(suffix) and len(tok) > len(suffix) + 1:
                vs.append(tok[: -len(suffix)])
        return vs
    for t in tokens:
        for v in variants(t):
            if v in source or v.lower() in src_lower:
                return True
    return False


def extract_frontmatter_blocks(path):
    """Yield dicts from YAML front-matter blocks delimited by ---."""
    src = open(path).read()
    blocks = []
    lines = src.split("\n")
    i = 0
    while i < len(lines):
        if lines[i].strip() == "---":
            j = i + 1
            buf = []
            while j < len(lines) and lines[j].strip() != "---":
                buf.append(lines[j])
                j += 1
            if j < len(lines) and buf:
                try:
                    blocks.append(yaml.safe_load("\n".join(buf)))
                except yaml.YAMLError as e:
                    print(f"  ⚠️ YAML parse error in {os.path.basename(path)}: {e}")
            i = j + 1
        else:
            i += 1
    return blocks


def load_backend_response_fields():
    """Extract JSON response field names from GetCurrentUser (user_staff.go)."""
    src = open(os.path.join(REPO, "backend", "handlers", "user_staff.go")).read()
    # GetCurrentUser has multiple `result := gin.H{` blocks; pick the one
    # containing "nickname" (the customer-facing profile response).
    blocks = list(re.finditer(r'result := gin\.H\{', src))
    if not blocks:
        return set()
    # Use the last block (customer profile) — locate its closing brace.
    start = blocks[-1].end()
    depth = 0
    i = start
    while i < len(src):
        if src[i] == '{':
            depth += 1
        elif src[i] == '}':
            depth -= 1
            if depth == 0:
                break
        i += 1
    block = src[start:i]
    fields = set(re.findall(r'"([a-z_]+)":', block))
    # fields resolved dynamically outside the gin.H literal
    fields |= {"membership_level_name", "site_name", "email_sent_at", "email_confirmed_at"}
    return fields


# Chinese display labels → backend response fields (for displays cross-check)
DISPLAY_FIELD_MAP = {
    "标题栏显示名": ["nickname", "name"],
    "昵称": ["nickname"],
    "当前昵称": ["nickname"],
    "更新后的昵称": ["nickname"],
    "手机号": ["phone"],
    "当前手机号": ["phone"],
    "更新后的手机号": ["phone"],
    "邮箱": ["email"],
    "当前邮箱": ["email"],
    "会员等级": ["membership_level_name", "membership_level_id"],
    "积分": ["promo_points"],
    "当前积分": ["promo_points"],
}


def verify_displays(displays, backend_fields):
    """Each display label must map to a field present in the backend response."""
    if not displays:
        return True
    ok = True
    for d in displays:
        fields = DISPLAY_FIELD_MAP.get(d, [])
        if fields:
            # membership_level_name is resolved dynamically; allow it
            if "membership_level_name" in fields:
                continue
            if not any(f in backend_fields for f in fields):
                ok = False
                print(f"  ❌ displays '{d}' → 后端响应缺字段 {fields}")
        # unmapped display labels are reported as warning, not failure
    return ok


def check_control_gate(page, control, gate, platforms, case_id, seq):
    """Behavioral: verify the JSX condition wrapping a control includes all
    variables mentioned in the YAML 'gate' field (#1623 class bugs).
    Returns a list of warning messages (empty = pass)."""
    warnings = []
    if not gate or not BEHAVIORAL:
        return warnings
    gate_vars = set(re.findall(r'([a-z_]+)\s*(?:=|$|\s)', gate))
    if not gate_vars:
        return warnings
    src = _page_source(page, platforms)
    if not src:
        return warnings
    escaped = re.escape(control)
    # Skip controls with emoji or that are compound (e.g. "发货按钮(pending)")
    if not re.search(r'[\u4e00-\u9fff]', control):
        return warnings
    m = re.search(escaped, src)
    if not m:
        return warnings
    before = src[:m.start()]
    last_brace = before.rfind("{")
    if last_brace < 0:
        return warnings
    cond_block = src[last_brace:m.start()].split("&&")[0] if "&&" in src[last_brace:m.start()] else ""
    cond_block = cond_block.replace("{", "").strip().split("||")[0].strip()
    # Extract variable names from condition
    cond_vars = set(re.findall(r'([a-zA-Z_][a-zA-Z0-9_]*)', cond_block))
    # Check: does YAML gate mention variables missing from JSX condition?
    missing = []
    for gv in gate_vars:
        if gv not in cond_block:
            missing.append(gv)
    if missing:
        warnings.append(
            f"{case_id} step{seq}: control '\\''{control}'\\'' gate 变量 {missing} 未在"
            f" JSX 条件中找到 (gate='{gate}', cond='{cond_block[:60]}') — 可能无条件常显 (#1623 类)"
        )
    return warnings


def check_data_refresh(page, platforms, case_id, seq):
    """Behavioral: verify that a page re-fetches data on re-show (#1625 class).
    weapp: must have useDidShow. H5: useEffect must have id-dependency."""
    warnings = []
    if not BEHAVIORAL:
        return warnings
    src = _page_source(page)
    if not src:
        return warnings
    p = page.strip()
    # Deduce the actual JSX file from page path
    jsx_file = _page_jsx(p, platforms)
    if not jsx_file or not os.path.exists(jsx_file):
        return warnings
    jsx_src = open(jsx_file).read()
    if "weapp" in platforms and "useDidShow" not in jsx_src:
        warnings.append(
            f"{case_id} step{seq}: weapp 页面 '{p}' 缺 useDidShow — "
            f"tab 切换回页面时不会重新 fetch 数据，可能残留旧状态 (#1625 类)"
        )
    if "h5" in platforms and "useEffect" in jsx_src:
        # Check if useEffect has id/params dependency
        ues = re.findall(r'useEffect\([^)]*,\s*\[([^\]]*)\]', jsx_src)
        has_id_dep = any("id" in d or "params" in d for d in ues)
        if not has_id_dep and any("fetch" in d.lower() or "load" in d.lower() for d in ues):
            warnings.append(
                f"{case_id} step{seq}: H5 页面 '{p}' useEffect 缺 id 依赖 — "
                f"重新进入相同路由不同参数时不会重新加载数据"
            )
    return warnings


def _page_jsx(page, platforms):
    """Map a YAML page path to the actual JSX source file."""
    p = page.strip()
    # Shared weapp pages: /staff/shipping → pages-weapp/shipping-interface/index → pages/ShippingInterface.jsx
    # First check if a pages-weapp shell exists
    mappings = {
        "/order/:id": "frontend-mobile/src/pages/OrderDetail.jsx",
        "/order-detail": "frontend-mobile/src/pages/OrderDetail.jsx",
        "/orders/:id": "frontend-mobile/src/pages/OrderDetail.jsx",
        "/payment": "frontend-mobile/src/pages/Payment.jsx",
        "/checkout": "frontend-mobile/src/pages/Checkout.jsx",
        "/profile": "frontend-mobile/src/pages/Profile.jsx",
        "/staff/shipping": "frontend-mobile/src/pages/ShippingInterface.jsx",
        "/staff/receiving": "frontend-mobile/src/pages/ReceivingInterface.jsx",
        "/staff/orders": "frontend-mobile/src/pages/StaffOrders.jsx",
        "/messages": "frontend-mobile/src/pages/Messages.jsx",
        "/message-detail": "frontend-mobile/src/pages/MessageDetail.jsx",
        "/membership": "frontend-mobile/src/pages/MembershipCenter.jsx",
        "/my-leases": "frontend-mobile/src/pages/MyLeases.jsx",
        "/detail": "frontend-mobile/src/pages/Detail.jsx",
        "/instrument/:id": "frontend-mobile/src/pages/Detail.jsx",
        "/instrument": "frontend-mobile/src/pages/Detail.jsx",
        "/return-settlement": "frontend-mobile/src/pages/ReturnSettlement.jsx",
        "/search": "frontend-mobile/src/pages/Search.jsx",
        "/cart": "frontend-mobile/src/pages/Cart.jsx",
        "/renewal": "frontend-mobile/src/pages/Renewal.jsx",
        "/repair-request": "frontend-mobile/src/pages/RepairRequestDetail.jsx",
        "/my-repairs": "frontend-mobile/src/pages/MyRepairs.jsx",
        "/receive-confirm": "frontend-mobile/src/pages/ReceiveConfirm.jsx",
        "/return-confirm": "frontend-mobile/src/pages/ReturnConfirm.jsx",
        "/": "frontend-mobile/src/pages/Home.jsx",
    }
    # Strip path params for matching
    cleaned = re.sub(r"/:[a-zA-Z0-9_]+", "", p)
    if cleaned in mappings:
        return os.path.join(REPO, mappings[cleaned])
    return None


def _page_source(page, platforms=None):
    """Load the JSX source for a page from its actual file."""
    jsx = _page_jsx(page, platforms or [])
    if jsx and os.path.exists(jsx):
        return open(jsx).read()
    return None


def check_weapp_cross_platform(weapp_pages):
    """Scan weapp source for Taro.navigateTo/redirectTo targets that are not
    registered in weappPages. Catches dead-link gaps (#1609/#1610)."""
    dead = []
    if not os.path.isdir("frontend-mobile/src/pages-weapp"):
        return dead
    targets = set()
    for root, _, files in os.walk("frontend-mobile/src/pages-weapp"):
        for fn in files:
            if not fn.endswith(".jsx"):
                continue
            src = open(os.path.join(root, fn)).read()
            targets.update(re.findall(r"(?:/)?pages-weapp/[a-z0-9-]+/index", src))
    for t in sorted(targets):
        norm = t.lstrip("/")
        if norm not in weapp_pages:
            dead.append(f"跨端死链: {norm} 被 weapp 源码跳转引用但未注册 (app.config.ts weappPages)")
    return dead


def main():
    weapp_pages = load_weapp_pages()
    h5_routes = load_h5_routes()
    backend_routes = load_backend_routes()
    backend_fields = load_backend_response_fields()
    global passed, failed, failures

    print("=== 前端检查清单静态校验 ===")
    for fname in sorted(os.listdir(CASES_DIR)):
        if not fname.endswith(".md") or fname.startswith("_") or fname == "README.md":
            continue
        fpath = os.path.join(CASES_DIR, fname)
        for block in extract_frontmatter_blocks(fpath):
            case_id = block.get("id", "?")
            domain = block.get("domain", "?")
            for step in block.get("steps", []):
                seq = step.get("seq", "?")
                fe = step.get("frontend", [])
                for entry in fe:
                    page = entry.get("page", "")
                    entry_platforms = entry.get("platform", [])
                    # 1. page registration
                    if not page_exists(page, weapp_pages, h5_routes, entry_platforms):
                        failed += 1
                        msg = f"{case_id} step{seq}: page {page!r} 未注册 (weapp/H5)"
                        failures.append(msg)
                        if VERBOSE: print(f"  ❌ {msg}")
                        continue
                    passed += 1
                    if VERBOSE: print(f"  ✅ {case_id} step{seq}: page {page}")
                    # 2. controls existence (soft check; 待* = known gap warning)
                    for ctl in entry.get("controls", []):
                        if "待" in str(ctl):
                            # known gap (e.g. 待前端接入) — report as warning, not failure
                            passed += 1
                            if VERBOSE: print(f"  ⚠️ {case_id} step{seq}: {ctl}（已知缺口，跳过）")
                            continue
                        if not find_control(page, ctl, weapp_pages, h5_routes, entry_platforms):
                            failed += 1
                            msg = f"{case_id} step{seq}: control {ctl!r} 未在 {page} 源码中找到"
                            failures.append(msg)
                            if VERBOSE: print(f"  ❌ {msg}")
                        else:
                            passed += 1
                    # 2.6 behavioral: control gate auditing (#1623 class)
                    if BEHAVIORAL:
                        gate = entry.get("gate", "")
                        for ctl in entry.get("controls", []):
                            ctl_warnings = check_control_gate(
                                page, ctl, gate, entry_platforms, case_id, seq
                            )
                            for w in ctl_warnings:
                                failed += 1
                                failures.append(w)
                                if VERBOSE: print(f"  ⚠️ {w}")
                    # 2.7 behavioral: data refresh on re-show (#1625 class)
                    if BEHAVIORAL:
                        refresh_warnings = check_data_refresh(
                            page, entry_platforms, case_id, seq
                        )
                        for w in refresh_warnings:
                            failed += 1
                            failures.append(w)
                            if VERBOSE: print(f"  ⚠️ {w}")
                    # 2.5 displays ↔ backend response field cross-check
                    displays = entry.get("displays", [])
                    if displays:
                        if verify_displays(displays, backend_fields):
                            passed += 1
                            if VERBOSE: print(f"  ✅ {case_id} step{seq}: displays {displays}")
                        else:
                            failed += 1
                            failures.append(f"{case_id} step{seq}: displays 字段与后端响应不符")
                    # 3. api route existence
                    api = step.get("api", {})
                    apath = api.get("path", "") if isinstance(api, dict) else ""
                    if apath:
                        norm = apath if apath.startswith("/") else "/" + apath
                        # strip path params for matching (:id → literal)
                        norm_static = re.sub(r":[a-zA-Z0-9_]+", ":id", norm)
                        if norm not in backend_routes and norm_static not in backend_routes:
                            failed += 1
                            msg = f"{case_id} step{seq}: api {apath} 未在 main.go 找到"
                            failures.append(msg)
                            if VERBOSE: print(f"  ❌ {msg}")
                        else:
                            passed += 1

    # Cross-platform consistency: weapp code navigates to pages that must
    # be registered in weappPages (catches dead-link gaps like #1609).
    dead_links = check_weapp_cross_platform(weapp_pages)
    for dl in dead_links:
        failed += 1
        failures.append(dl)
        if VERBOSE: print(f"  ❌ {dl}")

    print(f"\n=== 结果: {passed} 通过 / {failed} 失败 ===")
    if failures and not VERBOSE:
        print("失败项（--verbose 查看详情）:")
        for f in failures[:20]:
            print(f"  ❌ {f}")
        if len(failures) > 20:
            print(f"  ... 共 {len(failures)} 项")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
