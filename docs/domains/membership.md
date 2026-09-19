# 会员与促销系统设计（域索引）

> 来源: Issue #880 各轮评论汇总
> 版本: v4
> 最后更新: 2026-09-19
>
> **本文档为会员域索引**：表结构、商业逻辑、用例、权限已逆迁移至横切文档（单一权威），正文不再重复。

---

## 一、数据模型 → `docs/spec/database/database.md`

会员域表结构与字段已迁入 `docs/spec/database/database.md`：

| 主题 | 权威位置 |
|------|---------|
| 会员级别 membership_levels | `database.md` §2.40 |
| 促销方案 promo_plans / promo_plan_details | `database.md` §2.41-2.42 |
| 返点配置 rebate_config（⚠️ 已废弃 #1899 方案 A） | `database.md` §2.43 |
| 点数政策 points_policies（旧体系，并入 gift_policies） | `database.md` §2.44 |
| 赠点策略 gift_policies（v2 权威） | `database.md` §2.45 |
| 会员赠点比例 membership_gift_ratios（退役） | `database.md` §2.46 |
| 会员权益 membership_level_benefits | `database.md` §2.47 |
| 结算 settlements / 退款 order_refund_records | `database.md` §2.48-2.49 |
| 乐器促销覆盖 instrument_promo_overrides | `database.md` §2.50 |
| 会员域字段补充（users/instruments/orders/merchants） | `database.md` §2.51 |

---

## 二、商业逻辑 → `docs/cases/membership.md`（M-00~M-08）

会员/乐币商业规则权威口径为 `cases/membership.md`（手册 v2，#1939，凡与旧文档冲突以该文件为准）：

| 规则 | 权威位置 |
|------|---------|
| 会员等级与晋升门槛（乐手/首席/演奏家） | `cases/membership.md` M-01 |
| 乐币获取（入会/邀请/裂变/活动/购买） | `cases/membership.md` M-02 |
| 乐币使用与抵扣上限 | `cases/membership.md` M-03 |
| 乐币效期与过期清扫 | `cases/membership.md` M-05 |
| 裂变奖励（推荐人返佣比例） | `cases/membership.md` M-04 |
| 退款差额结算（取消退款返点） | `cases/membership.md` M-06 |
| 赠点策略配置 | `cases/membership.md` M-07 |
| 违约判定与晋升阻塞 | `cases/membership.md` M-08 |

---

## 三、用例 → `docs/cases/membership.md`

会员域用例权威为 `cases/membership.md`（M-00~M-08，结构化用例）。本域不再重复底录，各角色操作见该文件对应用例章节。

---

## 四、权限控制 → `docs/spec/permissions.md`

会员域 cus_perm 已收录于 `permissions.md` §3.1（bit21-25 去重，不作重复补录）：

| 权限码 | bit | 名称 | 授予角色 |
|--------|-----|------|---------|
| `rebate:manage` | 21 | 返点管理（⚠️ #1899 方案 A 已废弃） | sys_admin |
| `promo:manage` | 22 | 折扣政策管理 | sys_admin, merchant_admin |
| `promo:override` | 23 | 乐器促销覆盖 | site_admin |
| `points:manage` | 24 | 点数政策管理 | sys_admin, merchant_admin, site_admin |
| `membership:manage` | 25 | 会员级别管理 | sys_admin |

**错误码**：`40310`（membership_level_insufficient）/ `40311`（promo_not_applicable）。

---

## 五、相关文档

- `docs/spec/database/database.md` §2.40-2.51 — 会员域表结构与字段
- `docs/spec/permissions.md` §3.1 — 会员域 cus_perm（bit21-25）
- `docs/spec/api/api.md` §8.11（点数钱包）/ §12.5-12.7（会员级别/返点/折扣管理）— 会员域端点
- `docs/spec/ui/ui.md` §3.15/§3.33-3.34/§3.36 — 会员级别管理/返点/折扣/会员中心页面
- `docs/cases/membership.md` — M-00~M-08 会员与乐币权威用例（手册口径 v2，#1939）
- `AGENTS.md` §「业务数值必须配置化（#1939）」 — 手册数值可调整准则