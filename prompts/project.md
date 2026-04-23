添加新页面时必须验证：
- [ ] 支持列表页 URL（`/:page`）
- [ ] 支持详情页 URL（`/:page/:id`）
- [ ] 支持编辑页 URL（`/:page/:id/edit`）
- [ ] 支持创建页 URL（`/:page/new`）
- [ ] 操作后 URL 正确更新
- [ ] 浏览器前进/后退正常工作
- [ ] 左侧菜单选中状态同步

---

## 📦 Properties 关联业务逻辑

### 数据模型关系

```
properties (属性定义)     property_options (属性选项)    instrument_properties (乐器属性)
├── id                   ├── id                        ├── id
├── name                 ├── property_id (FK)          ├── instrument_id (FK)
├── property_type        ├── value                     ├── property_id (FK)
├── is_required          ├── status                    ├── value
├── unit                 └── alias                     └── ...
└── ...
```

### 创建乐器时 Properties 处理流程

当 POST /api/instruments 请求包含 `properties` 字段时：

1. **属性定义查找**: 根据 `key` 在 `properties` 表查找 `name = key` 的记录
2. **选项匹配**: 根据 `property_id` 和 `value` 在 `property_options` 表查找匹配记录
3. **自动创建**: 若选项不存在，自动创建 `property_options` 记录，`status = 'pending'`
4. **关联建立**: 在 `instrument_properties` 表创建乐器与属性的关联

### 状态说明

| Status | 说明 |
|--------|------|
| `pending` | 新创建的选项，待管理员审核 |
| `approved` | 已审核通过的选项 |
| `rejected` | 已拒绝的选项 |

### 示例

请求体:
```json
{
  "properties": {
    "型号": ["U1"],
    "品牌": ["雅马哈"]
  }
}
```

处理结果:
- 查找 `properties.name = '型号'` → 获取 property.id
- 查找 `property_options.property_id = ? AND value = 'U1'`
  - 找到 → 使用现有记录
  - 未找到 → 创建新记录 (status='pending')
- 创建 `instrument_properties` 关联记录

### 错误处理

- 若 `key` 在 `properties` 表中不存在 → 返回错误 "属性 '{key}' 未定义"

---

## 🔐 Instrument Level 关联

### 数据模型

```sql
instrument_levels
├── id (UUID, PK)
├── caption (varchar) - 显示名称：入门/专业/大师
├── code (varchar) - 代码：entry/professional/master
└── sort_order (int) - 排序
```

### 使用方式

1. **优先使用 level_id**: `POST /api/instruments {"level_id": "uuid-here", ...}`
2. **向后兼容**: `POST /api/instruments {"level": "专业", ...}` (自动查找匹配)
3. **降级处理**: 若 level 未在 instrument_levels 表中定义，使用旧版字符串映射

### 关联关系

`instruments.level_id` → 外键 → `instrument_levels.id`

查询时自动加载：`gorm:"foreignKey:LevelID"`

---

### 核心文档 / Core Documents
docs/cases.md
docs/api_design.md
docs/ui_design.md
docs/database_design.md
