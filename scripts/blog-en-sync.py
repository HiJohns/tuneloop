#!/usr/bin/env python3
"""Keep docs/en/blog.md in sync with docs/blog.md (bilingual parity guard).

`docs/blog.md` is an append-only build log: the batch harness appends one
Chinese bullet per run. Nothing regenerates the English mirror automatically,
so it drifts unless this script is run.

Usage:
    python3 scripts/blog-en-sync.py            # regenerate the English mirror
    python3 scripts/blog-en-sync.py --check    # verify parity, write nothing
    python3 scripts/blog-en-sync.py --src A --dst B   # alternate paths (tests)

Behaviour:
    * Harness template lines ("批量处理完成，包含任务:") are translated
      mechanically.
    * Every other line that still contains CJK must have an explicit entry in
      TRANSLATIONS. Otherwise the script lists the offending lines and exits 1
      -- it never silently emits untranslated output.

Exit codes:
    0  success / parity holds
    1  untranslated lines found, or (with --check) the mirror is out of date
"""

from __future__ import annotations

import argparse
import re
import sys
from pathlib import Path

REPO_ROOT = Path(__file__).resolve().parent.parent
DEFAULT_SRC = REPO_ROOT / "docs" / "blog.md"
DEFAULT_DST = REPO_ROOT / "docs" / "en" / "blog.md"

PLACEHOLDER = "批量处理完成，包含任务:"
PLACEHOLDER_EN = "Batch run completed; tasks included:"

# Lines that carry content beyond the harness template. Keys may omit trailing
# whitespace. Keep entries in chronological order to make review easy.
TRANSLATIONS = {
    "- 2026-09-11: 批量处理完成，包含任务: Resolved Issues: #1872（weapp 选图后参数丢失热修）":
        "- 2026-09-11: Batch run completed; tasks included: Resolved Issues: #1872 (hotfix: weapp loses params after image selection)",

    "- 2026-09-11: 批量处理完成，包含任务: Resolved Issues: #1873; Rejected: #1874（日期迁移遗漏 LeaseInfo/Checkout/weapp renewal，待返工）":
        "- 2026-09-11: Batch run completed; tasks included: Resolved Issues: #1873; Rejected: #1874 (date migration missed LeaseInfo/Checkout/weapp renewal; rework pending)",

    "- 2026-09-11: 批量处理完成，包含任务: Resolved Issues: #1874（返工补齐 4 处遗漏，复审 PASS）":
        "- 2026-09-11: Batch run completed; tasks included: Resolved Issues: #1874 (rework filled 4 missed spots; re-audit PASS)",

    "- 2026-09-11: 批量处理完成，包含任务: Resolved Issues: #1875（维修完成 jsonb 阻塞 + weapp 裸 alert 全局修复 47 处）":
        "- 2026-09-11: Batch run completed; tasks included: Resolved Issues: #1875 (repair-completion jsonb blocker + global fix of 47 raw alerts in weapp)",

    "- 2026-09-11: 批量处理完成，包含任务: Resolved Issues: #1876（weapp 维修记录照片相对路径补 origin）":
        "- 2026-09-11: Batch run completed; tasks included: Resolved Issues: #1876 (weapp repair-record photos: prepend origin to relative paths)",

    "- 2026-09-12: 批量处理完成，包含任务: Resolved Issues: #1877, #1878, #1879, #1880, #1881, #1882, #1883, #1884, #1885, #1886, #1887, #1888, #1889（维修域 13 项：站点/角色/属主校验 + current_site_id + reject 原因 + 文档/legacy 清理 + PC/weapp 修复；#1879 返工复审 PASS）":
        "- 2026-09-12: Batch run completed; tasks included: Resolved Issues: #1877, #1878, #1879, #1880, #1881, #1882, #1883, #1884, #1885, #1886, #1887, #1888, #1889 (13 repair-domain items: site/role/owner validation + current_site_id + reject reason + docs/legacy cleanup + PC/weapp fixes; #1879 rework re-audit PASS)",

    "- 2026-09-12: 批量处理完成，包含任务: Resolved Issues: #1890, #1891（真机反馈：维修状态中文化 + weapp 详情状态徽标；#1889 回填遗漏 org 唯一站点推导；另移除维修页顶条 738d6df5）":
        "- 2026-09-12: Batch run completed; tasks included: Resolved Issues: #1890, #1891 (device feedback: localize repair statuses + status badge on weapp detail; #1889 backfilled the missed unique-site derivation for org; also removed the repair page top bar 738d6df5)",

    "- 2026-09-12: 批量处理完成，包含任务: Resolved Issues: #1892（网点员工待验收乐器入口：新增 GET /repair/acceptance + MyRepairs 卡片；ui.md §2.9.1 + api.md §7.11.9）":
        "- 2026-09-12: Batch run completed; tasks included: Resolved Issues: #1892 (site-staff entry for instruments awaiting acceptance: new GET /repair/acceptance + MyRepairs card; ui.md §2.9.1 + api.md §7.11.9)",

    "- 2026-09-12: 批量处理完成，包含任务: Resolved Issues: #1893（乐器管理页两级分类筛选 + 封面 thumbnail 链 + OptionSheet 复用改造）":
        "- 2026-09-12: Batch run completed; tasks included: Resolved Issues: #1893 (instrument management: two-level category filter + cover thumbnail chain + OptionSheet reuse refactor)",

    "- 2026-09-12: 批量处理完成，包含任务: Resolved Issues: #1894（乐器 org 对齐站点 org 回填迁移 204 行 + 批量导入补写 org；根因：站点员工 ApplyOrgScope 按 org 过滤导致漏显示）":
        "- 2026-09-12: Batch run completed; tasks included: Resolved Issues: #1894 (204-row backfill migration aligning instrument org to site org + batch import now writes org; root cause: site-staff ApplyOrgScope filtered by org and hid rows)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1897（平台员工改用操作员 oid）, #1900（赠点策略数据归一 5/10/15% + 返点 0.5/1/2% + 护栏 + legacy 旁路）, #1901（OREZ/ENO points_basis + 非生产全额开关）；预生产已部署 ec3fab35 并开启 TEST_COUPON_GROSS_POINTS":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1897 (platform staff use operator oid), #1900 (gift-point policy normalization 5/10/15% + rebate 0.5/1/2% + guardrails + legacy bypass), #1901 (OREZ/ENO points_basis + non-production full-amount switch); prerelease deployed ec3fab35 with TEST_COUPON_GROSS_POINTS enabled",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1898（警告邮件通知：SMTP 服务/三级配置页/即时触发/冷却/不静默；SMTPS 465 兼容）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1898 (warning email notifications: SMTP service / three-tier config page / immediate trigger / cooldown / no silent failures; SMTPS 465 support)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1899（方案 A：移除系统折扣政策+返点配置 页面/路由/后端模块，表保留；返点统一到赠点策略）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1899 (Option A: removed system discount policy + rebate config pages/routes/backend modules, tables retained; rebates unified into the gift-point policy)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1897 返工（平台员工列表按 role=staff 过滤 + Create 持久化 STAFF；用例 #1795 T6.1/T6.2）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1897 rework (platform staff list filtered by role=staff + Create persists STAFF; cases #1795 T6.1/T6.2)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1897 返工 2（平台员工管理仅系统管理员：后端 403 门控 + 前端 bit5；不设 PLATFORM_ROOT_ORG_ID）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1897 rework 2 (platform staff management restricted to system admin: backend 403 gate + frontend bit5; no PLATFORM_ROOT_ORG_ID)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1904（beaconiam fn_roles claim 未解析 → 系统管理员被判为商户管理员 → 平台员工管理 403；修正 claim 映射 + 前端 fn_roles 识别）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1904 (beaconiam fn_roles claim unparsed → system admin misjudged as merchant admin → 403 on platform staff management; fixed claim mapping + frontend fn_roles detection)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1906（平台员工新建密码设置对齐：默认自动生成+一次性展示+强制改密+冲突 409）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1906 (align new-platform-staff password setup: auto-generated by default + one-time display + forced change + 409 on conflict)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1908（警告配置合并为单一配置 warning_notification，迁移合并旧三级键；前端单卡片）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1908 (warning config merged into a single warning_notification key, migrating the old three-tier keys; single frontend card)",

    "- 2026-09-13: 批量处理完成，包含任务: Resolved Issues: #1910（SMTP 配置程序内化：界面配置 DirectMail + 回落个人邮箱、密码加密、系统管理员专属、测试发送）":
        "- 2026-09-13: Batch run completed; tasks included: Resolved Issues: #1910 (in-app SMTP configuration: DirectMail UI config + fallback to personal mailbox, password encryption, system-admin only, test send)",

    "- 2026-09-14: 批量处理完成，包含任务: Resolved Issues: #1909（企业微信 Webhook：平台/商户两级配置、加密与 SSRF 校验、测试发送、失败回落站内通知）":
        "- 2026-09-14: Batch run completed; tasks included: Resolved Issues: #1909 (WeCom webhook: platform/merchant two-tier config, encryption + SSRF validation, test send, fallback to in-app notification on failure)",

    "- 2026-09-14: 批量处理完成，包含任务: Resolved Issues: #1911（维修师傅多网点：乐器列表/筛选改按全部成员站点，兼容管理员 org 作用域）":
        "- 2026-09-14: Batch run completed; tasks included: Resolved Issues: #1911 (multi-site repair technicians: instrument list/filter now spans all member sites, compatible with admin org scope)",

    "- 2026-09-14: 紧急处置：生产 tuneloop 崩溃循环 29h（DB schema 超前于部署快照）→ 正向发布 1.0.4，迁移 20260829003 → 20260912006，服务恢复；cadenza 磁盘回收 20G + deploy.sh 30 天保留策略（#1912/#1913）":
        "- 2026-09-14: Emergency response: production tuneloop crash loop for 29h (DB schema ahead of the deployment snapshot) → forward release 1.0.4, migrations 20260829003 → 20260912006, service recovered; cadenza disk reclaimed 20G + deploy.sh 30-day retention policy (#1912/#1913)",

    "- 2026-09-14: 批量处理完成，包含任务: Resolved Issues: #1915（订单详情支持微信订单中心 out_trade_no 跳转，审核订单 path 端到端可用）":
        "- 2026-09-14: Batch run completed; tasks included: Resolved Issues: #1915 (order detail supports WeChat order-center out_trade_no jump; the order-review path works end to end)",

    "- 2026-09-14: 批量处理完成，包含任务: Resolved Issues: #1919（PC乐器详情页白屏：补全Detail.jsx中formatBeijingDateTimeShort的import）":
        "- 2026-09-14: Batch run completed; tasks included: Resolved Issues: #1919 (PC instrument detail blank page: restored the missing import of formatBeijingDateTimeShort in Detail.jsx)",

    "- 2026-09-14: 批量处理完成，包含任务: Resolved Issues: #1926（待发货/已发货订单详情实际租赁天数与实际租金归零，收货后才计算）":
        "- 2026-09-14: Batch run completed; tasks included: Resolved Issues: #1926 (for pending-shipment/shipped orders, actual rental days and rent stay zero until receipt)",

    "- 2026-09-14: 批量处理完成，包含任务: Resolved Issues: #1923（注册勾选协议强校验 H5/小程序 + 登录页未登录协议入口）":
        "- 2026-09-14: Batch run completed; tasks included: Resolved Issues: #1923 (strict terms-agreement validation at registration, H5 + mini-program + terms entry on the logged-out login page)",

    "- 2026-09-15: 批量处理完成，包含任务: Resolved Issues: #1902, #1918, #1920, #1921, #1922, #1925; Rejected: #1924 (退回修复)":
        "- 2026-09-15: Batch run completed; tasks included: Resolved Issues: #1902, #1918, #1920, #1921, #1922, #1925; Rejected: #1924 (returned for fix)",

    "- 2026-09-17: 审计批次完成：#1949 PASS(done)；#1950/#1953 REJECTED 退回重做（详见各 Issue 审计报告）。Resolved Issues: #1949":
        "- 2026-09-17: Audit batch completed: #1949 PASS (done); #1950/#1953 REJECTED and returned for rework (see each Issue's audit report). Resolved Issues: #1949",

    "- 2026-09-17: 审计批次（Q3）— PASS/done: #1949 #1950 #1952 #1953 #1954 #1956 #1957 #1946 #1940；REJECTED: #1955（item 8 补缴支付入口缺后端数据源）；HOLD: #1951（父任务待 #1955 重做 + 预生产走查）。Resolved Issues: #1949, #1950, #1952, #1953, #1954, #1956, #1957, #1946, #1940":
        "- 2026-09-17: Audit batch (Q3) — PASS/done: #1949 #1950 #1952 #1953 #1954 #1956 #1957 #1946 #1940; REJECTED: #1955 (item 8 back-payment entry lacks a backend data source); HOLD: #1951 (parent waiting for #1955 rework + prerelease walkthrough). Resolved Issues: #1949, #1950, #1952, #1953, #1954, #1956, #1957, #1946, #1940",

    "- 2026-09-18: 审计批次（Q4）— PASS/done: #1955 #1958 #1959 #1960 #1961 #1962；父任务验收关闭: #1951 #1948。Resolved Issues: #1951, #1948, #1955, #1958, #1959, #1960, #1961, #1962":
        "- 2026-09-18: Audit batch (Q4) — PASS/done: #1955 #1958 #1959 #1960 #1961 #1962; parent acceptance closed: #1951 #1948. Resolved Issues: #1951, #1948, #1955, #1958, #1959, #1960, #1961, #1962",

    "- 2026-09-18: 审计批次（Q5）— PASS/done: #1965 #1966 #1967 #1968 #1969 #1971 #1972 #1973 #1975 #1976 #1977 #1978 #1979 #1944。Resolved Issues: #1965, #1966, #1967, #1968, #1969, #1971, #1972, #1973, #1975, #1976, #1977, #1978, #1979, #1944":
        "- 2026-09-18: Audit batch (Q5) — PASS/done: #1965 #1966 #1967 #1968 #1969 #1971 #1972 #1973 #1975 #1976 #1977 #1978 #1979 #1944. Resolved Issues: #1965, #1966, #1967, #1968, #1969, #1971, #1972, #1973, #1975, #1976, #1977, #1978, #1979, #1944",

    "- 2026-09-19: 审计批次（Q6）— PASS/done: #1941 #1947 #1980 #1981 #1983 #1987；REJECTED 退回重做: #1945（迁移 011 预设 name 匹配未命中+缺迁移测试）、#1982（加赠 sourceRef 字节截断中文致 500）；HOLD: #1974（父任务待 #1984 预生产走查）。Resolved Issues: #1941, #1947, #1980, #1981, #1983, #1987":
        "- 2026-09-19: Audit batch (Q6) — PASS/done: #1941 #1947 #1980 #1981 #1983 #1987; REJECTED and returned for rework: #1945 (migration 011 preset name matching missed + no migration test), #1982 (gift-add sourceRef truncation cut multi-byte characters → 500); HOLD: #1974 (parent waiting for #1984 prerelease walkthrough). Resolved Issues: #1941, #1947, #1980, #1981, #1983, #1987",

    "- 2026-09-19: 审计批次（Q6 补遗）— PASS/done: #1988（main 分支，文档治理新布局）。Resolved Issues: #1988":
        "- 2026-09-19: Audit batch (Q6 addendum) — PASS/done: #1988 (main branch, new documentation governance layout). Resolved Issues: #1988",

    "- 2026-09-19: 审计批次（Q7）— 二次审计 PASS/done: #1982（rune 截断+文案+回归测试 e9542ff0）、#1945（id 基预设迁移 20260918003+迁移测试 a92d9f28）。Resolved Issues: #1982, #1945":
        "- 2026-09-19: Audit batch (Q7) — second-round audit PASS/done: #1982 (rune truncation + wording + regression test e9542ff0), #1945 (id-based preset migration 20260918003 + migration test a92d9f28). Resolved Issues: #1982, #1945",

    "- 2026-09-19: 审计批次（Q8）— PASS/done: #1964 #1939 #1931（父任务集成核验）；REJECTED 退回重做: #1970（docs/topics/iam/iam.md 未按 A5 重建 symlink，为普通文件含路径字符串）。Resolved Issues: #1964, #1939, #1931":
        "- 2026-09-19: Audit batch (Q8) — PASS/done: #1964 #1939 #1931 (parent integration verification); REJECTED and returned for rework: #1970 (docs/topics/iam/iam.md not rebuilt as a symlink per A5; it was a regular file containing a path string). Resolved Issues: #1964, #1939, #1931",

    "- 2026-09-20: 审计批次（Q9）— OSS 链路二次审计 PASS/done: #1991 #1992 #1993 #1994 #1995（P4 回填/P3 双写/P5 切读/P6 收尾/S5 绕抽象改造）；#1989（7 个 H5 薄壳，build:h5 恢复）；#1986（迁移工件启动断言+快照恢复 SOP）；#1970（文档治理方案 A 合并落地，A5 symlink 修复）。Resolved Issues: #1970, #1986, #1989, #1991, #1992, #1993, #1994, #1995":
        "- 2026-09-20: Audit batch (Q9) — OSS pipeline second-round audit PASS/done: #1991 #1992 #1993 #1994 #1995 (P4 backfill / P3 dual-write / P5 switch reads / P6 wrap-up / S5 bypass-abstraction refactor); #1989 (7 H5 thin shells, build:h5 restored); #1986 (migration-artifact startup assertion + snapshot recovery SOP); #1970 (doc governance Option A merged, A5 symlink fixed). Resolved Issues: #1970, #1986, #1989, #1991, #1992, #1993, #1994, #1995",

    "- 2026-09-20: QA 批量审计完成，通过并关闭: #2003, #2002, #2001, #2000, #1997, #1980, #1964；REJECT 退回: #2004(2 项返工), #1999(集成阻塞)":
        "- 2026-09-20: QA batch audit completed; passed and closed: #2003, #2002, #2001, #2000, #1997, #1980, #1964; REJECTED and returned: #2004 (2 rework items), #1999 (integration blocked)",

    "- 2026-09-20: QA 二轮审计：#2004 返工复审 PASS（d1a3d2ae，2 项审计意见闭环）；父任务 #1999 集成验收 PASS 手动闭合（脚本 Parent Auto-Completion 因 comments 自匹配未触发）":
        "- 2026-09-20: QA second-round audit: #2004 rework re-audit PASS (d1a3d2ae, both audit findings closed); parent #1999 integration acceptance PASS closed manually (the Parent Auto-Completion script did not fire because of self-matching in comments)",

    "- 2026-09-21: QA 轮③：仪表盘分层与数据模型收敛全链路闭环——PASS 关闭 #2015 #2014 #2013 #2012 #2011（worktree 基线对照 0 新增失败）；返工复审 PASS #2006（聚合 fail-fast）/#2009（租约口径统一 orders）；父任务 #2005/#2010 集成验收手动闭合（脚本 Parent 搜索自匹配缺陷，已两度记录）。DB 红线：迁移未由 AI 执行，部署 bootstrap 校验项已列。":
        "- 2026-09-21: QA round 3: dashboard layering and data-model convergence closed end to end — PASS closed #2015 #2014 #2013 #2012 #2011 (worktree baseline comparison: 0 new failures); rework re-audit PASS for #2006 (aggregation fail-fast) / #2009 (lease basis unified to orders); parents #2005/#2010 integration acceptance closed manually (Parent search self-match defect in the script, recorded twice). DB red line: migrations not executed by AI; deployment bootstrap checks listed.",

    "- 2026-09-21: QA 轮④：users.wx_openid 废弃链路全绿——#2017（008b 接线+5 处回退移除+内部解析端点+客户端 helper）、#2018（用户管理/支付回退切换）、#2019（openidOfUser 切换+删本地列）全部 PASS；生产/预生产绑定覆盖实证 0 未覆盖（删列不丢数据）；发现并记录预存缺陷：解绑从未删 wx_user_bindings 行（#483 起）。":
        "- 2026-09-21: QA round 4: users.wx_openid deprecation pipeline fully green — #2017 (008b wiring + 5 fallbacks removed + internal resolve endpoint + client helper), #2018 (user management / payment fallback switch), #2019 (openidOfUser switch + local column dropped) all PASS; production/prerelease binding coverage evidence: 0 uncovered (dropping the column loses no data); pre-existing defect found and recorded: unbind never deleted wx_user_bindings rows (since #483).",

    "- 2026-09-21: #2016 关闭审核通过（引用清零实证 + 子任务全 done）；另立解绑失效缺陷 Issue（#483 起解绑未删绑定行，升级时随 beaconiam 修复发布）":
        "- 2026-09-21: #2016 closed after audit passed (evidence of zero remaining references + all sub-tasks done); a separate Issue filed for the broken-unbind defect (unbind has not deleted binding rows since #483; to ship with the beaconiam fix during upgrade)",

    "- 2026-09-30: 批量处理完成，包含任务: Resolved Issues: #2110, #2111, #2113（v1.0.32 预生产发布）":
        "- 2026-09-30: Batch run completed; tasks included: Resolved Issues: #2110, #2111, #2113 (v1.0.32 prerelease deploy)",

    "- 2026-09-30: 批量处理完成，包含任务: Resolved Issues: #2112（v1.0.33 预生产发布）":
        "- 2026-09-30: Batch run completed; tasks included: Resolved Issues: #2112 (v1.0.33 prerelease deploy)",

    "- 2026-10-15: 生产发布 v1.0.61（9bd0e0e1：main 合并 develop，含 #2202…#2216「加入网点」定向邀请制等 13 commit）；小程序 v1.0.61 已上传待微信审核":
        "- 2026-10-15: Production release v1.0.61 (9bd0e0e1: main merged develop, including #2202…#2216 \"join site\" targeted-invitation model and 13 commits total); mini-program v1.0.61 uploaded, awaiting WeChat review",

    "- 2026-10-15: 生产 IAM（beaconiam）升级 6a1a7bf：修复小程序登录（生产 IAM 缺 wx-login-select，根因/预防见 #2221）":
        "- 2026-10-15: Production IAM (beaconiam) upgraded to 6a1a7bf: fixes mini-program login (production IAM lacked wx-login-select; root cause/prevention in #2221)",

    "- 2026-10-15: 生产补发 v1.0.62（30f4f8e7：补入 #2217/#2218/#2219/#2220——首轮 9bd0e0e1 因本地 develop 过期漏合 4 提交）；小程序 v1.0.62 已上传（含全部 weapp 修复）待微信审核":
        "- 2026-10-15: Production re-release v1.0.62 (30f4f8e7: added #2217/#2218/#2219/#2220 — the first release 9bd0e0e1 missed 4 commits because the local develop branch was stale); mini-program v1.0.62 uploaded (includes all weapp fixes), awaiting WeChat review",

    "- 2026-10-16: 预生产刷新：服务端 tuneloop-pre 0fb8e5d4(v1.0.62) + 小程序开发版 APP_VERSION=1.0.62-pre 已上传（待后台设体验版）":
        "- 2026-10-16: Prerelease refresh: server tuneloop-pre 0fb8e5d4 (v1.0.62) + mini-program dev build APP_VERSION=1.0.62-pre uploaded (pending manual promotion to trial build in the WeChat console)",

    "- 2026-10-16: 【事故】全站 HTTPS 中断 ~7.7h（LE 证书过期）：certbot standalone 与 nginx 容器（host 网络）争 80 端口致续期静默失败；手动续期恢复（新证书至 2027-01-14）+ 安装 pre/post 续期钩子（根因/预防见 #2226）":
        "- 2026-10-16: [INCIDENT] Site-wide HTTPS outage ~7.7h (Let's Encrypt certificate expiry): certbot standalone collided with the nginx container (host network) over port 80, so renewals failed silently; recovered via manual renewal (new cert valid until 2027-01-14) + installed pre/post renewal hooks (root cause/prevention in #2226)",
}

CJK = re.compile(r"[\u3400-\u4dbf\u4e00-\u9fff\u3000-\u303f\uff00-\uffef]")


def build(src_lines: list[str]) -> tuple[list[str], list[tuple[int, str]]]:
    """Return (english_lines, untranslated). Never emits untranslated CJK."""
    out: list[str] = []
    untranslated: list[tuple[int, str]] = []

    for idx, line in enumerate(src_lines, 1):
        key = line.rstrip()
        if key in TRANSLATIONS:
            out.append(TRANSLATIONS[key])
            continue
        line = line.replace(PLACEHOLDER, PLACEHOLDER_EN)
        if CJK.search(line):
            untranslated.append((idx, line))
        out.append(line)

    return out, untranslated


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--src", type=Path, default=DEFAULT_SRC, help="source blog (default: docs/blog.md)")
    parser.add_argument("--dst", type=Path, default=DEFAULT_DST, help="english mirror (default: docs/en/blog.md)")
    parser.add_argument("--check", action="store_true", help="verify parity only; write nothing")
    args = parser.parse_args()

    raw = args.src.read_text(encoding="utf-8")
    src_lines = raw.split("\n")
    out, untranslated = build(src_lines)

    if untranslated:
        print(f"FAIL: {len(untranslated)} line(s) contain CJK with no TRANSLATIONS entry:", file=sys.stderr)
        for idx, line in untranslated:
            print(f"  {args.src}:{idx}: {line}", file=sys.stderr)
        print("\nAdd an English entry to TRANSLATIONS in this script, then re-run.", file=sys.stderr)
        return 1

    rendered = "\n".join(out)
    entry_count = sum(1 for line in src_lines if line.startswith("- "))
    generic_count = sum(1 for line in src_lines if PLACEHOLDER in line)
    explicit_count = len(TRANSLATIONS)

    if args.check:
        if not args.dst.exists():
            print(f"FAIL: {args.dst} does not exist", file=sys.stderr)
            return 1
        if args.dst.read_text(encoding="utf-8") != rendered:
            print(f"FAIL: {args.dst} is out of sync with {args.src}", file=sys.stderr)
            print("Run: python3 scripts/blog-en-sync.py", file=sys.stderr)
            return 1
        print(f"PASS: {args.dst} matches {args.src} ({entry_count} entries, {generic_count} templated, {explicit_count} explicit translations)")
        return 0

    args.dst.write_text(rendered, encoding="utf-8")
    print(f"OK: wrote {args.dst} ({entry_count} entries, {generic_count} templated, {explicit_count} explicit translations)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
