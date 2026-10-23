# tenant payload 盘点（#2262 B5）

> 生成时间：2026-10-23；范围：`backend/services/**`（非测试）、`frontend-mobile/src/**`、`frontend-pc/src/**`。
> 分类规则见 #2262 计划 §2.2。**仅 `webhook` 替换、`InvoiceConfirm` 移除 payload 两处改代码；其余保留并注明理由。**

## services

| file:line | 片段 | 判定 |
|---|---|---|
| `backend/services/quote_scanner.go:72` | `if err := db.Where("tenant_id = ?", req.TenantID).First(&merchant).Error; err == nil {` | 调度器/数据归属（保留） |
| `backend/services/email.go:27` | `// WarningConfigTenantID is the platform tenant that owns warning settings.` | 平台级/元数据（保留） |
| `backend/services/email.go:28` | `const WarningConfigTenantID = "00000000-0000-0000-0000-000000000000"` | 平台级/元数据（保留） |
| `backend/services/email.go:88` | `err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, SMTPConfigKey)` | 平台级/元数据（保留） |
| `backend/services/email.go:129` | `if err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, SMTPConfigK` | 平台级/元数据（保留） |
| `backend/services/email.go:147` | `err = db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, SMTPConfigKey).` | 平台级/元数据（保留） |
| `backend/services/email.go:153` | `TenantID:     WarningConfigTenantID,` | 平台级/元数据（保留） |
| `backend/services/email.go:207` | `err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, WarningConfigK` | 平台级/元数据（保留） |
| `backend/services/email.go:267` | `if err := db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, WarningConf` | 平台级/元数据（保留） |
| `backend/services/email.go:280` | `err = db.Where("tenant_id = ? AND setting_key = ?", WarningConfigTenantID, WarningConfigKe` | 平台级/元数据（保留） |
| `backend/services/email.go:286` | `TenantID:     WarningConfigTenantID,` | 平台级/元数据（保留） |
| `backend/services/warning_notification.go:84` | `TenantID:  warning.MerchantID,` | 调度器/数据归属（保留） |
| `backend/services/deposit_refund_scheduler.go:94` | `TenantID:        order.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/deposit_refund_scheduler.go:148` | `TenantID:   order.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/deposit_refund_scheduler.go:167` | `TenantID:   order.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/audit_query.go:26` | `TenantID     string` | 平台级/元数据（保留） |
| `backend/services/audit_query.go:103` | `q := &AuditLogQuery{BusinessRole: businessRole, ActorID: userID, TenantID: tenantID}` | 平台级/元数据（保留） |
| `backend/services/audit_query.go:171` | `if q.TenantID != "" {` | 平台级/元数据（保留） |
| `backend/services/audit_query.go:172` | `return query.Where("tenant_id = ?", q.TenantID)` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:102` | `result := db.Where("tenant_id = ? AND setting_key = ?", nilUUID, pair[0]).First(&existing)` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:105` | `TenantID:     nilUUID,` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:165` | `TenantID: tenantID,` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:293` | `TenantID: namespaceOrgID,` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:306` | `siteTenantID := namespaceOrgID` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:308` | `siteTenantID = *org.ParentID` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:312` | `TenantID: siteTenantID,` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:348` | `TenantID: namespaceOrgID,` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:369` | `if err := db.Where("tenant_id = ? AND setting_key = 'iam_client_id'", nilUUID).First(&sett` | 平台级/元数据（保留） |
| `backend/services/iam_bootstrap.go:377` | `if err := db.Where("tenant_id = ? AND setting_key = 'iam_client_secret'", nilUUID).First(&` | 平台级/元数据（保留） |
| `backend/services/bulk_import_accounts.go:84` | `db.Where("tenant_id = ?", tenantID).Find(&existingUsers)` | 平台导入（保留） |
| `backend/services/bulk_import_accounts.go:94` | `db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&sites)` | 平台导入（保留） |
| `backend/services/bulk_import_accounts.go:226` | `TenantID: tenantID,` | 平台导入（保留） |
| `backend/services/return_reminder_scheduler.go:65` | `TenantID:  order.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/iam.go:102` | `TenantID   string      `json:"tenant_id"`` | 平台级/元数据（保留） |
| `backend/services/iam.go:121` | `TenantID    string   `json:"tid"`` | 平台级/元数据（保留） |
| `backend/services/notification.go:25` | `TenantID:   tenantID,` | 调度器/数据归属（保留） |
| `backend/services/notification.go:94` | `TenantID:   tenantID,` | 调度器/数据归属（保留） |
| `backend/services/notification.go:127` | `Where("tenant_id = ? AND role IN ?", tenantID, []string{"OWNER", "ADMIN"}).` | 调度器/数据归属（保留） |
| `backend/services/notification.go:151` | `TenantID:   tenantID,` | 调度器/数据归属（保留） |
| `backend/services/overdue_deduction_scheduler.go:95` | `TenantID:   order.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/point_batches.go:62` | `if err := db.Where("tenant_id = ? AND setting_key = ?", "00000000-0000-0000-0000-000000000` | 调度器/数据归属（保留） |
| `backend/services/point_batches.go:269` | `TenantID:    u.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/point_batches.go:291` | `tenantByUser[b.UserID] = u.TenantID` | 调度器/数据归属（保留） |
| `backend/services/point_batches.go:302` | `TenantID:   tenantByUser[userID],` | 调度器/数据归属（保留） |
| `backend/services/point_batches.go:349` | `TenantID:   u.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/collection_reminder_scheduler.go:143` | `NotifyUsersBySiteWithAction(s.db, order.TenantID, siteID,` | 调度器/数据归属（保留） |
| `backend/services/collection_reminder_scheduler.go:148` | `NotifyMerchantAdmins(s.db, order.TenantID,` | 调度器/数据归属（保留） |
| `backend/services/audit_writer.go:11` | `TenantID     string` | 平台级/元数据（保留） |
| `backend/services/audit_writer.go:91` | `TenantID:     rec.TenantID,` | 平台级/元数据（保留） |
| `backend/services/rent_calculator.go:58` | `TenantID          string` | 调度器/数据归属（保留） |
| `backend/services/rent_calculator.go:189` | `promoRates, promoPlans, err := getPromoDiscounts(db, input.TenantID, input.OrgID)` | 调度器/数据归属（保留） |
| `backend/services/webhook.go:102` | `err := db.Where("tenant_id = ? AND setting_key = ?", merchantOrgID, MerchantWebhookKey).Fi` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:104` | `// 旧键回退：商户 org → merchants.tenant_id` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:106` | `if e := db.Where("org_id = ?", merchantOrgID).First(&m).Error; e == nil && m.TenantID != "` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:107` | `err = db.Where("tenant_id = ? AND setting_key = ?", m.TenantID, MerchantWebhookKey).First(` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:144` | `if err := db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).Firs` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:156` | `err = db.Where("tenant_id = ? AND setting_key = ?", tenantID, MerchantWebhookKey).First(&s` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:162` | `TenantID:     tenantID,` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:170` | `// merchantTenantID resolves a warning's merchant reference (merchant id or` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:172` | `func merchantTenantID(db *gorm.DB, merchantRef string) string {` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/webhook.go:187` | `if tenantID := merchantTenantID(db, warning.MerchantID); tenantID != "" {` | 替换（已改为商户 org，`d96d385e` 后本批） |
| `backend/services/bulk_import_orgs.go:86` | `db.Where("tenant_id = ? AND deleted_at IS NULL", tenantID).Find(&existingSites)` | 平台导入（保留） |
| `backend/services/bulk_import_orgs.go:231` | `TenantID: tenantID,` | 平台导入（保留） |

## frontend-mobile

| file:line | 片段 | 判定 |
|---|---|---|
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:55` | `if (!map[o.tenant_id]) map[o.tenant_id] = { tenant_id: o.tenant_id, merchant_name: o.merch` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:56` | `map[o.tenant_id].orders.push(o)` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:57` | `map[o.tenant_id].total += o.total_cents` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:76` | `const f = getForm(g.tenant_id)` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:123` | `const f = getForm(g.tenant_id)` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:125` | `<View key={g.tenant_id} style={{ background: '#fff', borderRadius: 12, padding: 16, margin` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:142` | `<View key={tt} onClick={() => setFormField(g.tenant_id, 'invoice_type', tt)} style={{ padd` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:148` | `<Input value={f.title} onInput={e => setFormField(g.tenant_id, 'title', getInputValue(e))}` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/InvoiceConfirm.jsx:150` | `<Input value={f.tax_number} onInput={e => setFormField(g.tenant_id, 'tax_number', getInput` | 展示/分组（保留）；payload 已移除 |
| `frontend-mobile/src/pages/Detail.jsx:98` | `tenant_id: instrument?.tenant_id \|\| '',` | 展示/元数据（保留） |
| `frontend-mobile/src/pages/Checkout.jsx:917` | `const key = `${item.tenant_id \|\| 'unknown'}-${item.site_id \|\| 'unknown'}`` | 展示/元数据（保留） |
| `frontend-mobile/src/pages/Checkout.jsx:920` | `tenant_id: item.tenant_id,` | 展示/元数据（保留） |
| `frontend-mobile/src/pages/Checkout.jsx:1135` | `<View key={group.tenant_id \|\| 'unknown'} className="rounded-xl p-3" style={{ backgroundC` | 展示/元数据（保留） |
| `frontend-mobile/src/pages/Cart.jsx:246` | `const key = `${item.tenant_id \|\| 'unknown'}-${item.site_id \|\| 'unknown'}`` | 展示/元数据（保留） |
| `frontend-mobile/src/pages/Cart.jsx:249` | `tenant_id: item.tenant_id,` | 展示/元数据（保留） |
| `frontend-mobile/src/pages/Cart.jsx:361` | `<View key={group.tenant_id \|\| 'unknown'} className="bg-white rounded-2xl shadow-sm overf` | 展示/元数据（保留） |
| `frontend-mobile/src/services/errorMessages.js:48` | `'tenant_id not found': '未找到租户信息',` | 展示/元数据（保留） |
| `frontend-mobile/src/pages-weapp/Detail.jsx:103` | `tenant_id: instrument?.tenant_id \|\| '',` | 展示/元数据（保留） |
| `frontend-mobile/src/pages-weapp/Checkout.jsx:874` | `const key = `${item.tenant_id \|\| 'unknown'}-${item.site_id \|\| 'unknown'}`` | 展示/元数据（保留） |
| `frontend-mobile/src/pages-weapp/Checkout.jsx:878` | `tenant_id: item.tenant_id,` | 展示/元数据（保留） |
| `frontend-mobile/src/pages-weapp/Checkout.jsx:1088` | `<View key={group.tenant_id \|\| 'unknown'} style={{ backgroundColor: 'rgba(250,250,250,0.4` | 展示/元数据（保留） |

## frontend-pc

| file:line | 片段 | 判定 |
|---|---|---|
| `frontend-pc/src/pages/TenantManagement.jsx:62` | `dataIndex: 'tenant_id',` | 平台展示（保留） |
| `frontend-pc/src/pages/TenantManagement.jsx:63` | `key: 'tenant_id',` | 平台展示（保留） |
| `frontend-pc/src/pages/Dashboard.jsx:87` | `{ title: '租户 ID', dataIndex: 'tenant_id', key: 'tenant_id', render: v => <span style={{ fo` | 平台展示（保留） |
| `frontend-pc/src/services/errorMessages.js:45` | `'tenant_id not found': '未找到租户信息',` | 展示/元数据（保留） |

