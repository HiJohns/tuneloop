## 🛡️ Audit Report: REJECTED

### ❌ 审计结论：未通过

### 🎯 计划对齐度检查

| 计划项 | 状态 | 实际代码位置 |
|--------|------|--------------|
| `/merchant/inventory` → `data.list` | ✅ 已完成 | `inventory.go:68-72` |
| `/sites/tree` → `data.list` | ❌ 未完成 | `site.go:558` 仍返回 `"sites"` |
| `/categories` → `data.list` | ❌ 未完成 | `api.go:284` 直接返回数组 |
| `/instruments` → `data.list` + pagination 内嵌 | ❌ 未完成 | `api.go:218-225` 仍是分离结构 |

### 🔍 详细问题

#### 1. `/sites/tree` 端点 (site.go:556-559)
```go
// 当前代码 - ❌ 不符合规范
c.JSON(http.StatusOK, gin.H{
    "code": 20000,
    "data": gin.H{"sites": result},  // 应改为 "list": result
})
```

#### 2. `/categories` 端点 (api.go:282-285)
```go
// 当前代码 - ❌ 不符合规范
c.JSON(http.StatusOK, gin.H{
    "code": 20000,
    "data": result,  // 应改为 "data": gin.H{"list": result}
})
```

#### 3. `/instruments` 端点 (api.go:216-225)
```go
// 当前代码 - ❌ 不符合规范
c.JSON(http.StatusOK, gin.H{
    "code": 20000,
    "data": responseInstruments,
    "pagination": gin.H{...},  // 应内嵌到 data 中
})
```

### ✅ 已完成项

- [x] 规范文档创建 `backend/docs/api_response_format.md`
- [x] `/merchant/inventory` 返回格式已统一为 `data.list`
- [x] `go build` 成功
- [x] `npm run lint` 通过
- [x] `npm run build` 成功

### ❌ 未完成项

- [ ] `/sites/tree` 未改为 `data.list` 格式
- [ ] `/categories` 未改为 `data.list` 格式
- [ ] `/instruments` 未将 pagination 内嵌到 data

### 🚀 修复要求

请 Developer 执行以下修复：

1. **site.go** (line ~558): 将 `"data": gin.H{"sites": result}` 改为 `"data": gin.H{"list": result}`
2. **api.go** (line ~284): 将 `"data": result` 改为 `"data": gin.H{"list": result}`
3. **api.go** (line ~216-225): 将 pagination 结构内嵌到 data 中

---
*Model: moonshotai-cn/kimi-k2-thinking*