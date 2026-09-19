# API 响应格式规范

## 标准响应格式

所有 API 端点应返回统一的标准格式：

```json
{
  "code": 20000,
  "data": {
    "list": [...],
    "total": 100,
    "page": 1,
    "pageSize": 20
  },
  "message": "success"
}
```

### 字段说明

| 字段 | 类型 | 必填 | 说明 |
|-----|------|------|------|
| code | int | 是 | 状态码，20000 表示成功 |
| data | object | 是 | 响应数据主体 |
| data.list | array | 否 | 数据列表（用于列表类接口） |
| data.total | int | 否 | 总记录数（用于分页） |
| data.page | int | 否 | 当前页码（用于分页） |
| data.pageSize | int | 否 | 每页大小（用于分页） |
| message | string | 否 | 成功/失败消息 |

## 错误响应格式

```json
{
  "code": 40002,
  "message": "invalid parameters: xxx",
  "data": null
}
```

## 需要修改的 API 端点

| 端点 | 当前格式 | 目标格式 |
|------|----------|----------|
| GET /api/common/sites | `{ data: { list: [...], total: ... } }` | `{ data: { list: [...], total: ... } }` ✅ |
| GET /api/merchant/inventory | `{ data: { instruments: [...], total: ... } }` | `{ data: { list: [...], total: ... } }` |
| GET /api/sites/tree | `{ data: { sites: [...] } }` | `{ data: { list: [...] } }` |
| GET /api/categories | `{ data: [...] }` | `{ data: { list: [...] } }` |
| GET /api/instruments | `{ data: [...], pagination: {...} }` | `{ data: { list: [...], total, page, pageSize } }` |

## 示例

### 列表接口

```json
{
  "code": 20000,
  "data": {
    "list": [
      { "id": "1", "name": "Item 1" },
      { "id": "2", "name": "Item 2" }
    ],
    "total": 100,
    "page": 1,
    "pageSize": 20
  }
}
```

### 详情接口

```json
{
  "code": 20000,
  "data": {
    "id": "1",
    "name": "Item 1"
  }
}
```

### 创建/更新接口

```json
{
  "code": 20000,
  "data": {
    "id": "1",
    "name": "New Item"
  },
  "message": "创建成功"
}
```