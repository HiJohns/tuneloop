# TuneLoop UI 设计文档

> 版本: v2.3 (状态模型重构: Instrument 5态 + Order 11态; 订单详情页 + 顾客/员工按钮逻辑)
> 最后更新: 2026-06-09
> 覆盖度: 100% features.md

---

## 一、设计原则

### 1.1 白标化适配 (White-labeling)
- **BrandProvider**: 根据 `client_id` 动态加载品牌配置
- **动态变量**: `--brand-primary`, `--brand-logo-url`
- **覆盖范围**: 所有页面（小程序/PC）支持主题切换

### 1.2 状态色规范

#### Instrument 状态
```css
:root {
  --status-available: #10B981;     /* Green - 可租 */
  --status-rented: #3B82F6;        /* Blue/Indigo - 租赁中 */
  --status-maintenance: #F59E0B;   /* Orange - 维修中 */
  --status-archived: #9CA3AF;      /* Gray - 已下架 */
  --status-lost: #6B7280;          /* Dark Gray - 已丢失 */
}
```

#### Order 状态
```css
:root {
  --order-reserved: #3B82F6;       /* Blue - 已预约 */
  --order-paid: #F97316;           /* Orange - 待发货 */
  --order-pending-shipment: #F97316; /* Orange - 待发货 */
  --order-in-transit: #06B6D4;     /* Cyan - 运输中 */
  --order-shipped: #10B981;        /* Green - 已发货 */
  --order-in-lease: #6366F1;       /* Indigo - 租赁中 */
  --order-returning: #EAB308;      /* Yellow - 归还中 */
  --order-returned: #9CA3AF;       /* Gray - 已归还 */
  --order-completed: #9CA3AF;      /* Gray - 已完成 */
  --order-cancelled: #EF4444;      /* Red - 已取消 */
  --order-expired: #EF4444;        /* Red - 超期 */
  --order-transferred: #8B5CF6;    /* Purple - 已过户 */
}
```

---

## 二、用户端（微信小程序）

### 2.1 页面架构

```
/pages
├── index/                          # 首页
│   ├── index.wxml
│   ├── index.wxss
│   └── index.js
├── instrument/
│   └── detail/                     # 乐器详情
│       └── detail.wxml
├── order/
│   ├── confirm/                    # 订单确认
│   ├── list/                       # 订单列表
│   └── detail/                     # 订单详情
├── maintenance/
│   ├── apply/                      # 报修申请
│   └── progress/                   # 报修进度
├── site/
│   └── detail/                     # 【新增】网点实境
├── certificate/
│   └── preview/                    # 【新增】证书预览
└── user/
    ├── index/                      # 个人中心
    ├── leases/                     # 租约管理
    ├── favorites/                  # 我的收藏
    └── addresses/                  # 地址管理
```

---

### 2.1a React Mobile App 路由表

**组件**: `frontend-mobile/src/App.jsx`

| 路由 | 组件 | 认证 | 说明 |
|------|------|------|------|
| `/` | `Home` | 可选 | 首页（乐器浏览） |
| `/instrument/:id` | `Detail` | 可选 | 乐器详情 |
| `/order/:id` | `OrderDetail` | 必须 | 顾客订单详情 |
| `/profile` | `Profile` | 必须 | 个人中心 |
| `/cart` | `Cart` | 可选 | 购物车 |
| `/receive/:orderId` | `ReceiveConfirm` | 必须 | 确认收货 |
| `/return/:orderId` | `ReturnConfirm` | 必须 | 归还 |
| `/staff/orders` | `StaffOrders` | 必须 | 员工订单管理（含搜索+扫码） |
| `/staff/orders/:id` | `StaffOrderDetail` | 必须 | 员工订单详情 |
| `/staff/instruments` | `StaffInstruments` | 必须 | 员工乐器管理 |
| `/staff/instrument/:id` | `StaffInstrumentDetail` | 必须 | 员工乐器详情 |
| `/staff/instrument/new` | `StaffInstrumentForm` | 必须 | 新建乐器 |
| `/staff/shipping` | `ShippingInterface` | 必须 | 发货界面 |
| `/staff/receiving` | `ReceivingInterface` | 必须 | 收货界面 |
| `/staff/receiving/:orderId` | `StaffReceiveConfirm` | 必须 | 收货确认 |

---

### 2.2 首页 (`/pages/index`)

**布局结构**:
```html
<view class="container">
  <!-- LBS 网点地图卡片 -->
  <view class="site-map-card" bindtap="navigateToNearbySites">
    <map 
      latitude="{{userLat}}" 
      longitude="{{userLng}}"
      markers="{{nearbySites}}"
      style="width: 100%; height: 120px;"
    />
    <text class="site-card-title">查看附近可租网点 (1.5km内)</text>
  </view>

  <!-- 分类入口 -->
  <scroll-view class="category-nav" scroll-x>
    <view class="category-item" wx:for="{{categories}}">
      <image src="{{item.icon}}" />
      <text>{{item.name}}</text>
    </view>
  </scroll-view>

  <!-- 限时推荐 (高亮大师级) -->
  <view class="recommend-section">
    <text class="section-title">🎯 限时推荐</text>
    <view class="instrument-card" wx:for="{{recommendInstruments}}">
      <image class="card-cover" src="{{item.cover}}" />
      <view class="card-info">
        <text class="card-title">{{item.name}}</text>
        <text class="card-level tag-master">大师级</text>
        <text class="card-price">¥{{item.rent}}/月</text>
      </view>
    </view>
  </view>

  <!-- 快捷功能区 -->
  <view class="quick-actions">
    <view class="action-item" bindtap="navigateToOrders">
      <icon type="order" />
      <text>我的订单</text>
    </view>
    <view class="action-item" bindtap="navigateToMaintenance">
      <icon type="repair" />
      <text>报修服务</text>
    </view>
    <view class="action-item" bindtap="contactCustomerService">
      <icon type="service" />
      <text>联系客服</text>
    </view>
  </view>
</view>
```

**交互说明**:
- 网点地图卡片：点击跳转至 `/pages/site/detail?id=xxx`
- 分类入口：点击筛选对应品类乐器
- 推荐卡片：点击跳转乐器详情页

---

### 2.3 乐器详情页 (`/pages/instrument/detail`)

**布局结构**:
```html
<view class="instrument-detail">
  <!-- 多图轮播 + 视频 (数据来源: GET /api/public/instruments/:id/media，含缩略图封面) -->
  <swiper class="image-swiper">
    <swiper-item wx:for="{{images}}">
      <image src="{{item}}" mode="aspectFill" />
    </swiper-item>
    <swiper-item wx:if="{{video}}">
      <video src="{{video}}" poster="{{thumbUrl}}" />
    </swiper-item>
  </swiper>

  <!-- 基础信息 -->
  <view class="info-section">
    <text class="instrument-name">{{instrument.name}}</text>
    <text class="instrument-brand">{{instrument.brand}}</text>
    <view class="level-badge level-{{instrument.level}}">
      {{instrument.level_name}}
    </view>
  </view>

  <!-- 服务权益对比浮层 (核心) -->
  <view class="service-comparison">
    <text class="section-title">📊 服务权益对比</text>
    <view class="comparison-table">
      <view class="table-header">
        <text class="header-item">权益项</text>
        <text class="header-item level-entry">入门级</text>
        <text class="header-item level-professional">专业级</text>
        <text class="header-item level-master">大师级</text>
      </view>
      <view class="table-row" wx:for="{{serviceItems}}">
        <text class="row-item service-name">{{item.name}}</text>
        <text class="row-item {{item.entry ? 'included' : 'excluded'}}">
          {{item.entry ? '✓' : '✗'}}
        </text>
        <text class="row-item {{item.professional ? 'included' : 'excluded'}}">
          {{item.professional ? '✓' : '✗'}}
        </text>
        <text class="row-item {{item.master ? 'included-highlight'}}">
          {{item.master ? '✓ 免费' : '✗'}}
        </text>
      </view>
    </view>
  </view>

  <!-- 规格选择器 -->
  <view class="spec-selector">
    <text class="selector-label">规格选择</text>
    <view class="spec-options">
      <view 
        class="spec-option {{selectedSpec == item.id ? 'active' : ''}}"
        wx:for="{{specs}}"
        bindtap="selectSpec"
        data-id="{{item.id}}"
      >
        {{item.name}}
      </view>
    </view>
  </view>

  <!-- 租期选择器 -->
  <view class="term-selector">
    <text class="selector-label">租期选择</text>
    <view class="term-options">
      <view 
        class="term-option {{selectedTerm == item.months ? 'active' : ''}}"
        wx:for="{{terms}}"
        bindtap="selectTerm"
        data-months="{{item.months}}"
      >
        <text>{{item.months}}个月</text>
        <text wx:if="{{item.discount < 1}}" class="discount-tag">
          {{item.discount * 10}}折
        </text>
      </view>
    </view>
  </view>

  <!-- 免押开关 (核心) -->
  <view class="deposit-section">
    <view class="deposit-switch">
      <text>信用免押金</text>
      <switch 
        checked="{{depositFreeEnabled}}" 
        bindchange="toggleDepositFree"
        disabled="{{!userDepositEligible}}"
      />
    </view>
    <view class="deposit-amount {{depositFreeEnabled ? 'crossed-out' : ''}}">
      押金 ¥{{instrument.deposit}}
    </view>
    <view wx:if="{{depositFreeEnabled}}" class="deposit-waiver">
      已免除 ¥{{instrument.deposit}}
    </view>
  </view>

  <!-- 计费汇总 -->
  <view class="pricing-summary">
    <text class="summary-label">首月租金</text>
    <text class="summary-value">¥{{calculatedRent}}</text>
    <text class="summary-label">押金</text>
    <text class="summary-value">{{depositFreeEnabled ? '¥0' : '¥' + instrument.deposit}}</text>
    <view class="summary-total">
      <text>合计</text>
      <text class="total-amount">¥{{totalAmount}}</text>
    </view>
  </view>

  <!-- 底部 CTA -->
  <view class="bottom-bar">
    <block wx:if="{{instrument.stock_status === 'in_stock'}}">
      <button class="btn-cart" bindtap="addToCart">🛒 加入购物车</button>
    </block>
    <button class="btn-primary" bindtap="createOrder">立即租用</button>
  </view>
</view>
```

**购物车交互（新增）**:
- **加入购物车**：乐器状态为 `in_stock` 时，底部操作栏并排显示"加入购物车"和"立即租赁"按钮
- **加入成功弹窗**：点击"加入购物车"后弹出确认对话框，显示"加入成功"消息
  - "继续浏览" → 关闭弹窗，留在当前页
  - "提交订单" → 导航至 `/cart` 购物车页面
- **购物车存储**：使用 localStorage 存储，数据结构为 `{items: [{instrument_id, name, tenant_id, ...}]}`

**核心交互**:
1. **服务权益对比**：动态高亮当前选中级别的免费项
2. **免押开关**：信用达标用户可开启，押金实时划线免除
3. **租期联动**：选择12个月自动显示95折标签
4. **实时计价**：所有选择联动更新合计金额

---

### 2.4 网点实境页 (`/pages/site/detail`) 【新增】

**布局结构**:
```html
<view class="site-detail">
  <!-- 门店实拍图 -->
  <swiper class="site-images">
    <swiper-item wx:for="{{site.images}}">
      <image src="{{item}}" mode="aspectFill" />
    </swiper-item>
  </swiper>

  <!-- 基础信息 -->
  <view class="site-info">
    <text class="site-name">{{site.name}}</text>
    <view class="info-row">
      <icon type="location" />
      <text>{{site.address}}</text>
    </view>
    <view class="info-row">
      <icon type="phone" />
      <text bindtap="makePhoneCall">{{site.phone}}</text>
    </view>
    <view class="info-row">
      <icon type="clock" />
      <text>营业时间: {{site.business_hours}}</text>
    </view>
  </view>

  <!-- 在线导航 -->
  <button class="nav-btn" bindtap="openMap">
    导航去这里
  </button>

  <!-- 实时库存 -->
  <view class="stock-status">
    <text class="section-title">本店可租库存</text>
    <view class="stock-items">
      <view class="stock-item" wx:for="{{site.stock_status}}">
        <text class="instrument-name">{{item.name}}</text>
        <view class="stock-counts">
          <text class="available">可租: {{item.available}}</text>
          <text class="renting">在租: {{item.renting}}</text>
          <text class="maintenance">维保: {{item.maintenance}}</text>
        </view>
      </view>
    </view>
  </view>
</view>
```

**交互说明**:
- `makePhoneCall`: 一键拨打门店电话
- `openMap`: 调用微信内置地图导航

---

### 2.5 个人中心页 (`/pages/user/index`)

**布局结构**:
```html
<view class="user-center">
  <!-- 用户信息 -->
  <view class="user-header">
    <image class="avatar" src="{{user.avatar}}" />
    <text class="user-name">{{user.name}}</text>
    <text class="user-phone">{{user.phone}}</text>
    <view wx:if="{{user.is_shadow}}" class="shadow-badge">
      👻 IAM同步用户
    </view>
  </view>

  <!-- 租转售进度组件 (高光) -->
  <view class="ownership-progress-widget">
    <view class="progress-ring" style="--progress: {{progress}}%">
      <text class="progress-text">{{accumulated}}/12 个月</text>
    </view>
    <text class="progress-message">{{progressMessage}}</text>
    <button 
      wx:if="{{transferEligible}}" 
      class="cert-btn"
      bindtap="viewCertificate"
    >
      查看电子证书
    </button>
    <text wx:else class="countdown">预计 {{remaining}} 个月后获得所有权</text>
  </view>

  <!-- 快捷入口 -->
  <view class="quick-menu">
    <view class="menu-item" bindtap="navigateToLeases">
      <icon type="lease" />
      <text>租约管理</text>
    </view>
    <view class="menu-item" bindtap="navigateToFavorites">
      <icon type="star" />
      <text>我的收藏</text>
    </view>
    <view class="menu-item" bindtap="navigateToAddresses">
      <icon type="address" />
      <text>收货地址</text>
    </view>
    <view class="menu-item" bindtap="navigateToHelp">
      <icon type="help" />
      <text>帮助中心</text>
    </view>
  </view>
</view>
```

**核心样式**:
```css
/* 动态进度环 */
.progress-ring {
  width: 120px;
  height: 120px;
  border-radius: 50%;
  background: conic-gradient(
    var(--progress-rent) 0deg, 
    var(--progress-rent) calc(var(--progress) * 3.6deg), 
    #eee calc(var(--progress) * 3.6deg)
  );
  display: flex;
  align-items: center;
  justify-content: center;
}

.progress-ring::before {
  content: '';
  width: 80px;
  height: 80px;
  border-radius: 50%;
  background: white;
}
```

---

### 2.5a 个人中心-React实现 (Profile Page)

**路由**: `/profile`
**组件**: `frontend-mobile/src/pages/Profile.jsx`

#### 顾客视图

当 `businessRole` 不是 `site_admin` 或 `site_member` 时显示：

**当前租赁区**:
- 显示活跃订单列表（状态：`reserved`/`paid`/`pending_shipment`/`in_transit`/`shipped`/`in_lease`/`returning`/`expired`）
- 订单卡片：订单号、状态标签、租期、逾期提醒、月租/押金
- 点击 → 跳转 `/order/:id`（订单详情页）

**租赁历史区**:
- 显示已结束订单（`returned`/`completed`/`cancelled`/`transferred`）
- 显示押金退还状态
- 点击 → 跳转 `/order/:id`

**订单详情页**（顾客，`/order/:id`）:

| 状态 | 按钮 | 行为 |
|------|------|------|
| `reserved` | 支付 | `POST /orders/:id/pay`（二次确认） |
| `paid`/`pending_shipment`/`in_transit` | 取消订单 | `POST /orders/:id/cancel`（二次确认） |
| `shipped` | 确认收货 | 跳转 `/receive/:id` |
| `in_lease`/`expired` | 归还 | 跳转 `/return/:id` |
| `expired` | — | 状态区上方红框显示逾期天数+累计费 |
| `returning` | — | "乐器归还中，等待验收" |
| `cancelled`/`completed`/`returned`/`transferred` | — | 终端描述 |

#### 员工视图

当 `businessRole` 为 `site_admin` 或 `site_member` 时显示：

**员工功能区**（权限门控）：

| 按钮 | 目标 | 所需权限 |
|------|------|---------|
| 乐器管理 | `/staff/instruments` | `instrument:read` |
| 订单管理 | `/staff/orders` | `order:read` |

权限检查方式（客户端 bitmask）:
```js
const mapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')
const cusPerm = parseInt(localStorage.getItem('user_cus_perm') || '0')
const has = (code) => { const b = mapping[code]; return b !== undefined && (cusPerm & (1 << b)) !== 0 }
```

API 来源：
- `GET /api/config/permissions` → `cus_perm_mapping`（权限位映射）
- JWT claims → `sys_perm` / `cus_perm`（用户权限位掩码）

---

**布局结构**:
```html
<view class="certificate-preview">
  <image 
    class="certificate-image" 
    src="{{certificate.preview_url}}" 
    mode="widthFix"
  />
  <view class="preview-actions">
    <button class="btn-download" bindtap="downloadPDF">
      下载PDF证书
    </button>
    <button class="btn-share" bindtap="shareCertificate">
      分享证书
    </button>
  </view>
</view>
```

**交互**:
- `downloadPDF`: 调用 `/api/user/ownership/:id/download` 获取PDF流
- `shareCertificate`: 调用微信分享API

---

### 2.7 购物车页 (`/cart`) 【新增】

**布局结构**:
```html
<view class="cart-page">
  <view class="cart-header">
    <text>购物车</text>
  </view>

  <!-- 按租户分组 -->
  <view class="cart-group" wx:for="{{groups}}">
    <view class="group-header">租户: {{item.tenant_id}}</view>
    <view class="cart-item" wx:for="{{item.items}}">
      <image src="{{subItem.images[0]}}" class="item-image" />
      <view class="item-info">
        <text class="item-name">{{subItem.name}}</text>
        <text class="item-brand">{{subItem.brand}} {{subItem.model}}</text>
        <text class="item-price">¥{{subItem.pricing.monthly_rent}}/月起</text>
      </view>
      <button class="btn-delete" bindtap="removeItem" data-id="{{subItem.instrument_id}}">
        🗑️
      </button>
    </view>
  </view>

  <!-- 空购物车 -->
  <view wx:if="{{items.length === 0}}" class="cart-empty">
    <text>购物车为空</text>
    <button bindtap="goHome">去逛逛</button>
  </view>

  <!-- 底部下单栏 -->
  <view class="bottom-bar" wx:if="{{items.length > 0}}">
    <button class="btn-order" bindtap="handleOrder">
      下单 ({{items.length}} 件)
    </button>
  </view>
</view>
```

**交互**:
- **分组显示**：按 `tenant_id` 将购物车中的乐器分组，每组显示租户标识
- **单项删除**：每个乐器项右侧有删除按钮，点击从购物车移除
- **下单校验**：
  - 未登录 → 跳转登录页，登录后返回购物车
  - 已登录 → 单乐器跳转现有结算流程，多乐器目前提示开发中
- **数据持久化**：购物车数据存储在 localStorage，下单后自动清空

---

## 三、PC 端（商家管理端 & 平台运营端）

### 3.1 技术栈
- **框架**: React 18 + TypeScript
- **UI 库**: Ant Design 5.x
- **样式**: Tailwind CSS
- **图表**: ECharts
- **地图**: AMap（高德地图）

---

### 3.2 登录页 (`/login`)

**组件结构**:
```tsx
// pages/Login/index.tsx
import { BrandProvider } from '@/components/BrandProvider';

const LoginPage: React.FC = () => {
  const { brandConfig, loading } = useBrandConfig();

  if (loading) {
    return (
      <div className="redirect-transition">
        <Spin size="large" />
        <p>正在跳转至安全身份验证中心...</p>
      </div>
    );
  }

  return (
    <BrandProvider config={brandConfig}>
      <div className="login-container" style={{ '--brand-primary': brandConfig.primary_color }}>
        <div className="login-box">
          <img src={brandConfig.logo_url} className="brand-logo" alt="logo" />
          <h1 className="brand-name">{brandConfig.brand_name}</h1>
          <Button 
            type="primary" 
            size="large" 
            onClick={redirectToIAM}
            style={{ backgroundColor: 'var(--brand-primary)' }}
          >
            立即登录
          </Button>
        </div>
      </div>
    </BrandProvider>
  );
};
```

**BrandProvider 实现**:
```tsx
// components/BrandProvider/index.tsx
import { ConfigProvider } from 'antd';

const BrandProvider: React.FC<{ config: BrandConfig; children: React.ReactNode }> = ({ config, children }) => {
  return (
    <ConfigProvider
      theme={{
        token: {
          colorPrimary: config.primary_color,
          borderRadius: 6,
        },
      }}
    >
      {children}
    </ConfigProvider>
  );
};
```

---

### 3.3 Dashboard 首页 (`/dashboard`)

**组件结构**:
```tsx
// pages/Dashboard/index.tsx
import { Row, Col, Card, Statistic } from 'antd';
import { useNavigate } from 'react-router-dom';

const Dashboard: React.FC = () => {
  const navigate = useNavigate();

  // 统计卡片数据（可点击穿透）
  const stats = [
    {
      title: '今日订单',
      value: 12,
      filter: { created_today: 1 },
      route: '/merchant/leases',
    },
    {
      title: '在租资产',
      value: 85,
      filter: { status: 'renting' },
      route: '/merchant/assets',
    },
    {
      title: '逾期预警',
      value: 3,
      filter: { overdue: 1 },
      route: '/merchant/leases/overdue',
      valueStyle: { color: '#EF4444' }, // 状态色
    },
    {
      title: '待处理工单',
      value: 5,
      filter: { status: 'pending' },
      route: '/merchant/maintenance',
      valueStyle: { color: '#F59E0B' }, // 状态色
    },
  ];

  const handleStatClick = (stat: any) => {
    navigate(`${stat.route}?${new URLSearchParams(stat.filter)}`);
  };

  return (
    <div className="dashboard">
      <Row gutter={16}>
        {stats.map((stat, index) => (
          <Col span={6} key={index}>
            <Card 
              className="stat-card clickable" 
              onClick={() => handleStatClick(stat)}
              hoverable
            >
              <Statistic 
                title={stat.title} 
                value={stat.value} 
                valueStyle={stat.valueStyle}
              />
            </Card>
          </Col>
        ))}
      </Row>

      {/* 待办事项 */}
      <Card title="待办事项" style={{ marginTop: 24 }}>
        <List>
          <List.Item>
            <Badge dot><Icon type="warning" /></Badge>
            <span>订单 #L002 已逾期7天，请尽快联系客户</span>
            <Button size="small" onClick={() => navigate('/merchant/leases/overdue')}>
              立即处理
            </Button>
          </List.Item>
        </List>
      </Card>

      {/* 最近订单 */}
      <Card title="最近订单" style={{ marginTop: 24 }}>
        <Table 
          columns={recentOrderColumns} 
          dataSource={recentOrders}
          onRow={(record) => ({
            onClick: () => navigate(`/merchant/leases/${record.id}`),
          })}
        />
      </Card>
    </div>
  );
};
```

---

### 3.4 商家管理端 - 左侧边栏

**菜单结构**:
```tsx
// layouts/MerchantLayout/menu.tsx
const merchantMenu = [
  {
    key: 'asset',
    icon: <Icon component={PackageIcon} />,
    label: '资产管理',
    children: [
      {
        key: '/merchant/assets',
        label: '设备台账',
        route: '/merchant/assets',
      },
      {
        key: '/merchant/inventory',
        label: '库存监控',
        route: '/merchant/inventory',
      },
      {
        key: '/merchant/ownership-monitor',
        label: '所有权监控',
        route: '/merchant/ownership-monitor',
      },
      {
        key: '/merchant/inventory/transfer',
        label: '调拨申请', // 【新增】
        route: '/merchant/inventory/transfer',
      },
    ],
  },
  {
    key: 'lease',
    icon: <Icon component={FileTextIcon} />,
    label: '租赁管理',
    children: [
      {
        key: '/merchant/leases',
        label: '租约台账',
        route: '/merchant/leases',
      },
      {
        key: '/merchant/leases/overdue',
        label: '逾期预警',
        route: '/merchant/leases/overdue',
      },
    ],
  },
  {
    key: 'maintenance',
    icon: <Icon component={ToolIcon} />,
    label: '维保管理',
    children: [
      {
        key: '/merchant/maintenance',
        label: '工单列表',
        route: '/merchant/maintenance',
      },
      {
        key: '/merchant/maintenance/quotes',
        label: '报价中心',
        route: '/merchant/maintenance/quotes',
      },
    ],
  },
  {
    key: 'finance',
    icon: <Icon component={DollarIcon} />,
    label: '财务结算',
    children: [
      {
        key: '/merchant/finance/commissions',
        label: '佣金明细',
        route: '/merchant/finance/commissions',
      },
      {
        key: '/merchant/finance/statement',
        label: '流水报表',
        route: '/merchant/finance/statement',
      },
    ],
  },
];
```

---

### 3.5 平台运营端 - 左侧边栏

**菜单结构**:
```tsx
// layouts/AdminLayout/menu.tsx
const adminMenu = [
  {
    key: 'merchant',
    icon: <Icon component={ShopIcon} />,
    label: '商家管理',
    children: [
      {
        key: '/admin/merchants',
        label: '商家准入审核',
        route: '/admin/merchants',
      },
      {
        key: '/admin/permissions',
        label: '权限管理',
        route: '/system/permissions',
      },
    ],
  },
  {
    key: 'pricing',
    icon: <Icon component={CalculatorIcon} />,
    label: '计费规则',
    children: [
      {
        key: '/admin/pricing-matrix',
        label: '定价矩阵',
        route: '/admin/pricing-matrix',
      },
      {
        key: '/admin/maintenance-packages',
        label: '维保服务包',
        route: '/admin/maintenance-packages',
      },
    ],
  },
  {
    key: 'finance',
    icon: <Icon component={BankIcon} />,
    label: '财务中心',
    children: [
      {
        key: '/admin/settlements',
        label: '全局结算',
        route: '/admin/settlements',
      },
      {
        key: '/admin/deposits',
        label: '押金监管',
        route: '/admin/deposits',
      },
    ],
  },
  {
    key: 'audit',
    icon: <Icon component={AuditIcon} />,
    label: '资产审计',
    children: [
      {
        key: '/admin/assets/trail',
        label: '流转轨迹',
        route: '/admin/assets/trail',
      },
      {
        key: '/admin/assets/map', // 【新增】
        label: '全网资产地图',
        route: '/admin/assets/map',
      },
    ],
  },
];
```

---

### 3.6 定价矩阵页 - Excel 网格编辑 UI (`/admin/pricing-matrix`)

**组件实现**:
```tsx
// pages/PricingMatrix/index.tsx
import { Table } from 'antd';

const PricingMatrix: React.FC = () => {
  const categories = ['钢琴', '小提琴', '吉他', '架子鼓'];
  const levels = ['entry', 'professional', 'master'];
  
  // 模拟数据
  const dataSource = categories.map(category => {
    const row: any = { category };
    levels.forEach(level => {
      row[`${level}_rent`] = pricingData[category][level].monthly_rent;
      row[`${level}_deposit`] = pricingData[category][level].deposit;
    });
    return row;
  });

  const columns = [
    {
      title: '品类',
      dataIndex: 'category',
      fixed: 'left',
      width: 120,
    },
    {
      title: '入门级',
      children: [
        {
          title: '租金',
          dataIndex: 'entry_rent',
          editable: true,
          render: (value: number) => `¥${value}`,
        },
        {
          title: '押金',
          dataIndex: 'entry_deposit',
          editable: true,
          render: (value: number) => `¥${value}`,
        },
      ],
    },
    {
      title: '专业级',
      children: [
        {
          title: '租金',
          dataIndex: 'professional_rent',
          editable: true,
          render: (value: number) => `¥${value}`,
        },
        {
          title: '押金',
          dataIndex: 'professional_deposit',
          editable: true,
          render: (value: number) => `¥${value}`,
        },
      ],
    },
    {
      title: '大师级',
      children: [
        {
          title: '租金',
          dataIndex: 'master_rent',
          editable: true,
          render: (value: number) => `¥${value}`,
        },
        {
          title: '押金',
          dataIndex: 'master_deposit',
          editable: true,
          render: (value: number) => `¥${value}`,
        },
      ],
    },
  ];

  return (
    <div className="pricing-matrix">
      <Card title="定价矩阵 (Excel网格编辑)">
        <Table
          dataSource={dataSource}
          columns={columns}
          bordered
          pagination={false}
          scroll={{ x: 800 }}
          components={{
            body: {
              cell: EditableCell, // 自定义可编辑单元格
            },
          }}
        />
        <div style={{ marginTop: 16 }}>
          <Button type="primary" onClick={saveMatrix}>
            保存修改
          </Button>
        </div>
      </Card>
    </div>
  );
};

// 可编辑单元格组件
const EditableCell: React.FC = ({
  editing,
  dataIndex,
  title,
  record,
  index,
  children,
  ...restProps
}) => {
  const inputNode = <InputNumber />;
  return (
    <td {...restProps}>
      {editing ? (
        <Form.Item
          name={dataIndex}
          style={{ margin: 0 }}
          rules={[{ required: true, message: `请输入${title}` }]}
        >
          {inputNode}
        </Form.Item>
      ) : (
        children
      )}
    </td>
  );
};
```

**交互特性**:
- 双击单元格进入编辑模式
- 支持批量修改
- 实时校验输入
- 保存前预览变更

---

### 3.7 资产流转轨迹 - Timeline 视图 (`/admin/assets/:id/trail`)

**组件实现**:
```tsx
// pages/AssetTrail/index.tsx
import { Timeline } from 'antd';
import { ClockCircleOutlined, CheckCircleOutlined, ToolOutlined } from '@ant-design/icons';

const AssetTrail: React.FC = () => {
  const timelineItems = [
    {
      dot: <ClockCircleOutlined style={{ fontSize: '16px', color: '#10B981' }} />,
      color: '#10B981',
      children: (
        <div>
          <h4>入库</h4>
          <p>北京总仓</p>
          <p>2024-01-15</p>
          <p>采购入库 - 供应商: 雅马哈中国</p>
        </div>
      ),
    },
    {
      dot: <FileTextOutlined style={{ fontSize: '16px', color: '#3B82F6' }} />,
      color: '#3B82F6',
      children: (
        <div>
          <h4>租约 #L001</h4>
          <p>租客: 张三</p>
          <p>2026-03-21 ~ 2027-03-21</p>
          <p>状态: 已完成</p>
        </div>
      ),
    },
    {
      dot: <ToolOutlined style={{ fontSize: '16px', color: '#F59E0B' }} />,
      color: '#F59E0B',
      children: (
        <div>
          <h4>维保记录 #T001</h4>
          <p>师傅: 李师傅</p>
          <p>2026-08-10</p>
          <p>问题: 琴弦松动</p>
          <p>费用: ¥0 (服务包内)</p>
        </div>
      ),
    },
    {
      dot: <FileTextOutlined style={{ fontSize: '16px', color: '#3B82F6' }} />,
      color: '#3B82F6',
      children: (
        <div>
          <h4>租约 #L002 (当前)</h4>
          <p>租客: 王五</p>
          <p>2026-04-01 ~ 2027-04-01</p>
          <p>状态: 在租中</p>
          <p>已累计: 8个月</p>
        </div>
      ),
    },
    {
      dot: <GiftOutlined style={{ fontSize: '16px', color: '#8B5CF6' }} />,
      color: '#8B5CF6',
      children: (
        <div>
          <h4>转售/报废 (预计)</h4>
          <p>预计日期: 2027-04-01</p>
          <p>剩余: 4个月</p>
          <Progress percent={66.7} strokeColor="#8B5CF6" />
        </div>
      ),
    },
  ];

  return (
    <div className="asset-trail">
      <Card title="资产流转轨迹">
        <div className="asset-info">
          <h3>资产编号: INS-2024-00001</h3>
          <p>SN码: SN-2024-0001</p>
          <p>名称: 雅马哈立式钢琴 U1</p>
          <p>当前状态: 在租中</p>
        </div>
        
        <Timeline mode="left">
          {timelineItems.map((item, index) => (
            <Timeline.Item key={index} dot={item.dot} color={item.color}>
              {item.children}
            </Timeline.Item>
          ))}
        </Timeline>
      </Card>
    </div>
  );
};
```

**视觉特性**:
- 不同事件类型使用不同颜色图标
- 左侧垂直时间轴，清晰展示资产全生命周期
- 包含时间、地点、参与方、费用等完整信息

---

### 3.8 全网资产地图 (`/admin/assets/map`) 【新增】

**组件实现**:
```tsx
// pages/AssetsMap/index.tsx
import AMapLoader from '@amap/amap-jsapi-loader';

const AssetsMap: React.FC = () => {
  const mapRef = useRef<HTMLDivElement>(null);
  const [amap, setAmap] = useState<any>(null);

  useEffect(() => {
    AMapLoader.load({
      key: 'your-amap-key',
      version: '2.0',
    }).then((AMap) => {
      const map = new AMap.Map(mapRef.current, {
        zoom: 5,
        center: [116.4074, 39.9042], // 北京
      });

      // 按城市聚合资产数据
      const cityData = [
        { name: '北京', center: [116.4074, 39.9042], total: 150, renting: 120 },
        { name: '上海', center: [121.4737, 31.2304], total: 120, renting: 95 },
        { name: '广州', center: [113.2644, 23.1291], total: 80, renting: 65 },
        { name: '深圳', center: [114.0579, 22.5431], total: 90, renting: 78 },
      ];

      cityData.forEach((city) => {
        const marker = new AMap.Marker({
          position: city.center,
          content: `
            <div class="asset-marker">
              <div class="marker-total">${city.total}</div>
              <div class="marker-renting">${city.renting}</div>
            </div>
          `,
        });
        map.add(marker);
      });

      setAmap(map);
    });
  }, []);

  return (
    <div className="assets-map">
      <Card title="全网资产分布地图">
        <div ref={mapRef} style={{ width: '100%', height: '600px' }} />
        
        <div className="map-legend">
          <div className="legend-item">
            <span className="color-box total"></span>
            <span>资产总数</span>
          </div>
          <div className="legend-item">
            <span className="color-box renting"></span>
            <span>在租数量</span>
          </div>
        </div>

        <div className="map-stats">
          <Row gutter={16}>
            <Col span={6}>
              <Statistic title="全国资产总数" value={440} />
            </Col>
            <Col span={6}>
              <Statistic title="在租总数" value={358} />
            </Col>
            <Col span={6}>
              <Statistic title="在租率" value={81.4} suffix="%" />
            </Col>
            <Col span={6}>
              <Statistic title="覆盖城市" value={4} />
            </Col>
          </Row>
        </div>
      </Card>
    </div>
  );
};

// 自定义标记样式
const markerStyle = `
  .asset-marker {
    width: 40px;
    height: 40px;
    border-radius: 50%;
    background: #6366F1;
    color: white;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    font-size: 12px;
    font-weight: bold;
  }
  .marker-total::after {
    content: ' 总';
    font-size: 10px;
  }
  .marker-renting {
    font-size: 10px;
  }
  .marker-renting::after {
    content: ' 租';
  }
`;
```

**功能特性**:
- 地图展示各城市资产分布
- 聚合标记显示总数和在租数
- 点击查看城市详情
- 右侧统计面板实时更新

---

### 3.9 影子用户状态标识

**组件实现**:
```tsx
// components/ShadowUserBadge/index.tsx
import { Tooltip, Badge } from 'antd';

interface ShadowUserBadgeProps {
  isShadow: boolean;
  userSource?: string; // IAM来源
}

const ShadowUserBadge: React.FC<ShadowUserBadgeProps> = ({ isShadow, userSource }) => {
  if (!isShadow) return null;

  return (
    <Tooltip title={`来自 ${userSource || 'IAM'} 自动同步`}>
      <Badge count="👻" style={{ backgroundColor: '#8B5CF6' }} />
    </Tooltip>
  );
};

// 使用示例：在用户列表中
const UserList: React.FC = () => {
  const columns = [
    {
      title: '用户',
      render: (record: User) => (
        <Space>
          {record.user_name}
          <ShadowUserBadge 
            isShadow={record.is_shadow} 
            userSource={record.identity_source}
          />
        </Space>
      ),
    },
    // 其他列...
  ];

  return <Table columns={columns} dataSource={userData} />;
};
```

---

### 3.10 IAM 同步按钮 (IAM Sync Button)

#### 3.10.1 网点管理 - IAM 组织同步按钮

**位置**: `src/pages/SiteManagement.jsx` 页面头部

**按钮文案**: "从 IAM 同步组织"

**权限控制**:
- 仅对 `role === 'ADMIN'` 或 `role === 'OWNER'` 的用户可见
- 无权限用户不渲染此按钮

**交互流程**:
```javascript
// 点击 handler
const handleSyncFromIAM = async () => {
  setSyncLoading(true);
  try {
    const response = await api.post('/api/iam/organizations/sync');
    if (response.code === 20000) {
      message.success(`同步成功：新增 ${response.data.synced} 个组织`);
      // 重新加载网点树
      fetchSiteTree();
    }
  } catch (error) {
    message.error('同步失败：' + error.message);
  } finally {
    setSyncLoading(false);
  }
};
```

**UI 状态**:
- **默认**: "从 IAM 同步组织" (Button type="primary")
- **加载中**: 显示 `<Spin />` 图标 + "同步中..." (按钮 disabled)
- **成功**: message.success + 自动刷新列表
- **失败**: message.error

**视觉设计**:
```jsx
<Button 
  type="primary" 
  icon={<CloudSyncOutlined />}
  onClick={handleSyncFromIAM}
  loading={syncLoading}
  disabled={!isAdminOrOwner}
>
  从 IAM 同步组织
</Button>
```

---

### 3.10.2 人员管理 - IAM 用户同步按钮

**位置**: `src/pages/StaffManagement.jsx` 页面头部（或用户管理页面）

**按钮文案**: "从 IAM 同步用户"

**权限控制**:
- 仅对 `role === 'ADMIN'` 或 `role === 'OWNER'` 的用户可见

**交互流程**:
```javascript
const handleSyncUsersFromIAM = async () => {
  setSyncLoading(true);
  try {
    const response = await api.post('/api/iam/users/sync');
    if (response.code === 20000) {
      message.success(`同步成功：新增 ${response.data.synced} 个用户`);
      // 重新加载用户列表
      fetchUsers();
    }
  } catch (error) {
    message.error('同步失败：' + error.message);
  } finally {
    setSyncLoading(false);
  }
};
```

**UI 状态**: 同 3.10.1

**视觉设计**:
```jsx
<Button 
  type="primary" 
  icon={<UserAddOutlined />}
  onClick={handleSyncUsersFromIAM}
  loading={syncLoading}
  disabled={!isAdminOrOwner}
>
  从 IAM 同步用户
</Button>
```

---

## 四、原子组件设计 (Atomic Design)

### 4.1 基础组件清单

| 组件名称 | 用途 | 支持平台 |
|----------|------|----------|
| `AssetCard` | 资产卡片展示 | 小程序/PC |
| `OwnershipProgressBar` | 租转售进度条 | 小程序/PC |
| `BrandProvider` | 白标化主题注入 | PC |
| `ShadowUserBadge` | 影子用户标识 | PC |
| `PricingMatrixGrid` | 定价矩阵网格 | PC |
| `AssetTimeline` | 资产流转时间轴 | PC |
| `AssetsMap` | 资产分布地图 | PC |

---

### 4.2 AssetCard 组件 (跨平台)

**小程序实现**:
```html
<!-- components/AssetCard/index.wxml -->
<view class="asset-card {{className}}">
  <image class="card-cover" src="{{coverImage}}" mode="aspectFill" />
  <view class="card-info">
    <text class="card-title">{{title}}</text>
    <view class="card-meta">
      <text class="level-tag level-{{level}}">{{levelName}}</text>
      <text class="price">¥{{monthlyRent}}/月</text>
    </view>
    <view class="card-status">
      <text class="status-dot status-{{status}}"></text>
      <text class="status-text">{{statusText}}</text>
    </view>
  </view>
</view>
```

**PC 端实现**:
```tsx
// components/AssetCard/index.tsx
import { Card } from 'antd';

interface AssetCardProps {
  title: string;
  coverImage: string;
  level: 'entry' | 'professional' | 'master';
  levelName: string;
  monthlyRent: number;
  status: 'available' | 'renting' | 'maintenance';
  statusText: string;
  onClick?: () => void;
}

const AssetCard: React.FC<AssetCardProps> = (props) => {
  const { title, coverImage, level, levelName, monthlyRent, status, statusText, onClick } = props;

  return (
    <Card 
      className="asset-card" 
      hoverable 
      onClick={onClick}
      cover={<img alt={title} src={coverImage} />}
    >
      <Card.Meta
        title={title}
        description={
          <div>
            <Tag className={`level-tag level-${level}`}>{levelName}</Tag>
            <div className="price">¥{monthlyRent}/月</div>
            <div className="status">
              <span className={`status-dot status-${status}`}></span>
              <span>{statusText}</span>
            </div>
          </div>
        }
      />
    </Card>
  );
};
```

**共享样式**:
```css
/* 跨平台共享样式 */
.asset-card {
  border-radius: 8px;
  overflow: hidden;
  transition: all 0.3s;
}

.asset-card:hover {
  transform: translateY(-4px);
  box-shadow: 0 8px 24px rgba(0,0,0,0.15);
}

.level-tag {
  padding: 4px 8px;
  border-radius: 4px;
  font-size: 12px;
  font-weight: 500;
}

.level-entry { background: #E0F2FE; color: #0891B2; }
.level-professional { background: #FEF3C7; color: #D97706; }
.level-master { background: #F3E8FF; color: #7C3AED; }

.status-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  margin-right: 4px;
}

.status-available { background: var(--status-online); }
.status-renting { background: var(--progress-rent); }
.status-maintenance { background: var(--status-maintenance); }
```

---

### 4.3 OwnershipProgressBar 组件

**小程序实现**:
```html
<!-- components/OwnershipProgressBar/index.wxml -->
<view class="ownership-progress">
  <view class="progress-ring" style="--progress: {{progress}}%">
    <text class="progress-text">{{accumulated}}/{{total}} 个月</text>
  </view>
  <view class="progress-info">
    <text class="progress-message">{{message}}</text>
    <text wx:if="{{!transferEligible}}" class="countdown">
      预计 {{remaining}} 个月后获得所有权
    </text>
    <button 
      wx:if="{{transferEligible}}" 
      class="cert-btn"
      bindtap="viewCertificate"
    >
      查看电子证书
    </button>
  </view>
</view>
```

**PC 端实现**:
```tsx
// components/OwnershipProgressBar/index.tsx
import { Progress, Button } from 'antd';

interface OwnershipProgressBarProps {
  accumulated: number;
  total: number;
  remaining: number;
  transferEligible: boolean;
  onViewCertificate?: () => void;
}

const OwnershipProgressBar: React.FC<OwnershipProgressBarProps> = (props) => {
  const { accumulated, total, remaining, transferEligible, onViewCertificate } = props;
  const progress = (accumulated / total) * 100;
  
  const message = transferEligible
    ? '🎉 恭喜！您已获得永久所有权'
    : `🎁 距离永久拥有仅剩 ${remaining} 个月`;

  return (
    <div className="ownership-progress-bar">
      <Progress
        type="circle"
        percent={progress}
        format={(percent) => `${accumulated}/${total} 个月`}
        strokeColor={
          transferEligible ? '#10B981' : '#3B82F6'
        }
        width={120}
      />
      
      <div className="progress-info">
        <div className="progress-message">{message}</div>
        {!transferEligible && (
          <div className="countdown">
            预计 {remaining} 个月后获得所有权
          </div>
        )}
        {transferEligible && (
          <Button type="primary" onClick={onViewCertificate}>
            查看电子证书
          </Button>
        )}
      </div>
    </div>
  );
};
```

---

## 五、Features.md 覆盖率验证

| 功能模块 | 功能点 | 小程序 | PC端 | 覆盖率 |
|----------|--------|--------|------|--------|
| **注册登录** | 微信快捷登录 | ✅ | ✅ | 100% |
| **乐器租赁** | 分类列表/详情/阶梯定价/租期折扣 | ✅ | ✅ | 100% |
| **订单支付** | 免押金/首期汇总/协议签署 | ✅ | ✅ | 100% |
| **维保服务** | 在线报修/工单追踪/服务包查询 | ✅ | ✅ | 100% |
| **个人中心** | 租约管理/收藏/地址 | ✅ | - | 100% |
| **租转售** | 进度条/电子证书 | ✅ | ✅ | 100% |
| **商家管理** | 设备台账/库存监控/所有权监控 | - | ✅ | 100% |
| **租赁管理** | 租约台账/逾期预警 | - | ✅ | 100% |
| **维保调度** | 工单管理/报价中心 | - | ✅ | 100% |
| **财务结算** | 佣金明细/流水报表 | - | ✅ | 100% |
| **平台治理** | 商家准入/RBAC/定价矩阵/押金监管 | - | ✅ | 100% |
| **资产审计** | 流转轨迹/统计大屏 | - | ✅ | 100% |
| **增强功能** | LBS网点地图/服务包对比/免押开关 | ✅ | - | 100% |
| **管理增强** | 调拨申请/全网资产地图/Timeline | - | ✅ | 100% |
| **技术增强** | 白标化BrandProvider/影子用户标识 | ✅ | ✅ | 100% |

**总体覆盖率: 100%**

---

## 六、技术实现要点

### 6.1 Lin-IAM 白标化适配

**前端实现**:
```typescript
// hooks/useBrandConfig.ts
import { useEffect, useState } from 'react';
import { getBrandConfig } from '@/api/common';

interface BrandConfig {
  primary_color: string;
  logo_url: string;
  brand_name: string;
  support_phone: string;
}

export const useBrandConfig = (clientId: string) => {
  const [config, setConfig] = useState<BrandConfig | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const loadConfig = async () => {
      try {
        const { data } = await getBrandConfig(clientId);
        setConfig(data);
        // 注入CSS变量
        document.documentElement.style.setProperty('--brand-primary', data.primary_color);
      } catch (error) {
        console.error('加载品牌配置失败:', error);
      } finally {
        setLoading(false);
      }
    };

    loadConfig();
  }, [clientId]);

  return { brandConfig: config, loading };
};
```

**小程序实现**:
```javascript
// utils/brand.js
export const loadBrandConfig = (clientId) => {
  return new Promise((resolve, reject) => {
    wx.request({
      url: `${API_BASE}/api/common/brand-config`,
      data: { client_id: clientId },
      success: (res) => {
        const config = res.data.data;
        // 存储到全局
        getApp().globalData.brandConfig = config;
        resolve(config);
      },
      fail: reject,
    });
  });
};
```

---

### 6.2 原子组件库搭建

**项目结构**:
```
components/
├── AssetCard/                    # 资产卡片
│   ├── index.tsx                 # PC端
│   ├── index.wxml                # 小程序
│   ├── index.wxss
│   └── index.js
├── OwnershipProgressBar/         # 租转售进度条
│   ├── index.tsx
│   ├── index.wxml
│   └── index.wxss
├── BrandProvider/                # 白标化提供者
│   └── index.tsx
├── ShadowUserBadge/              # 影子用户标识
│   └── index.tsx
├── PricingMatrixGrid/            # 定价矩阵网格
│   └── index.tsx
├── AssetTimeline/                # 资产时间轴
│   └── index.tsx
└── AssetsMap/                    # 资产地图
    └── index.tsx
```

**发布方案**:
- 小程序: 作为项目本地组件
- PC端: 可独立发布为 `@tuneloop/ui` npm包

---

### 6.3 性能优化

| 优化项 | 小程序 | PC端 |
|--------|--------|------|
| 图片懒加载 | ✅ 使用 `lazy-load` | ✅ 使用 `loading="lazy"` |
| 组件按需加载 | ✅ 分包加载 | ✅ React.lazy + Suspense |
| 数据分页 | ✅ `scroll-view` + `onReachBottom` | ✅ Table pagination |
| 缓存策略 | ✅ `wx.setStorage` | ✅ react-query/SWR |
| 骨架屏 | ✅ 页面级骨架屏 | ✅ 组件级 Skeleton |

---

## 七、版本记录

| 版本 | 日期 | 变更内容 |
|------|------|----------|
| v1.0 | 2026-03-20 | 初始版本 |
| v2.0 | 2026-03-21 | 整合客户反馈：LBS网点地图、服务包对比、免押开关、白标化、影子用户标识、资产流转Timeline、全网资产地图 |

---

*文档生成: 2026-03-21*<br>
*覆盖度: 100% features.md (v26.3.16)*<br>
*Model: minimax-m2.5-free*

---

## PC端侧边栏菜单 (Imported from AGENTS.md, 2026-04-17)

### 菜单层级

**1. 仪表盘 (Dashboard)**
- **路由**: `/`
- **权限**: 所有已登录用户
- **说明**: 系统首页

**2. 乐器管理**
- **路由**: 一级菜单
- **权限**: 所有已登录用户
- **子菜单**:
  - 乐器列表 (`/instruments/list`)
  - 分类设置 (`/instruments/categories`)
  - 属性管理 (`/instruments/properties`)

**3. 库存监控** ⭐
- **路由**: 一级菜单
- **权限**: site_manager, admin, owner
- **子菜单**:
  - 库存调拨 (`/inventory/transfer`)
  - 租金设定 (`/inventory/rent-setting`)

**4. 组织管理**
- **路由**: 一级菜单
- **权限**: 所有已登录用户
- **子菜单**:
  - 网点管理 (`/organization/sites`)
  - 人员管理 (`/staff`)

**5. 系统管理**
- **路由**: 一级菜单
- **权限**: 商户管理员（含 sys_perm bit 26 可看权限管理）
- **子菜单**:
  - 商户管理 (`/merchants`) — sys_perm bit 5
    - 创建/编辑表单新增**商户类型**下拉选择（全权商户/受控商户）
    - 选择"受控商户"时条件显示中转地址、中转电话、中转联系人字段
    - 创建表单新增**跳过邮箱验证** Checkbox，勾选后管理员直接激活无需确认邮件
    - 勾选"跳过邮箱验证"且创建成功时，弹窗显示管理员初始密码
  - 操作日志 (`/system/audit-logs`) — sys_perm bit 5
  - 权限管理 (`/system/permissions`) — sys_perm bit 26（商户管理员）
  - 客户端管理 (`/system/clients`) — sys_perm bit 0
  - 租户管理 (`/system/tenants`) — sys_perm bit 6

### 权限管理页面设计（#660）

**路由**: `/system/permissions`  
**权限**: sys_perm bit 26 (`permission:manage`)，商户管理员可见  
**组件**: `frontend-pc/src/pages/admin/PermissionManage/index.jsx`

#### 页面结构

双 Tab 布局：

```
┌──────────────────────────────────────────────────────┐
│  权限管理                                             │
│  ┌──────────────┐ ┌──────────────┐                   │
│  │ 成员权限     │ │ 角色管理     │                   │
│  └──────────────┘ └──────────────┘                   │
│                                                      │
│  [Tab 内容区]                                         │
└──────────────────────────────────────────────────────┘
```

#### Tab 1 — 成员权限

**表格列**: 姓名 | 所属网点 | 角色标签 | 权限摘要 | 操作

**编辑权限 Modal**：
- 角色下拉：从 `GET /admin/roles` 获取角色列表
- 个人权限 Checkbox：按「乐器」和「订单」两个域分组显示
  - 乐器: 创建/查看/编辑/删除/定价/维修管理
  - 订单: 创建/查看/编辑/取消
- 管理员未持有的权限码置灰 + Tooltip 提示
- 保存后提示「权限已更新，该用户下次登录后生效」

#### Tab 2 — 角色管理（商户管理员可见）

**表格列**: 角色名称 | 代码 | 权限数 | 权限详情 | 操作

- 系统角色（owner/admin/staff/worker）：不可删除
- 自定义角色：可编辑/删除
- 删除前检查是否有成员使用，若有则需先重新分配

**新建/编辑角色 Modal**：
- 角色名称 + 代码（新建时填写，编辑时只读）
- 权限 Checkbox：按域分组
- 权限列表自动过滤为当前管理员持有的权限子集

#### 网点管理员角色分配

**组件**: `frontend-pc/src/components/SiteMemberManagement.jsx`

网点管理员在人员管理页面通过行内 `Select` 下拉为成员分配角色。下拉选项实时从 `GET /admin/roles` 获取：
- 3 个标准角色（网点管理员/网点员工/维修工程师）
- 商户管理员创建的自定义角色（全商户可见）

选择角色后调用 `PUT /admin/users/:id/roles` 实时更新。

---

### 权限控制汇总 (v2.2 — 10 cus_perm + sys_perm 25-26)

> 完整权限-人员矩阵和菜单-权限映射参见 [`docs/permissions.md`](./permissions.md)。
> 以下为本 UI 文档特化的菜单可见性规则概览。

**菜单可见性 = sys_perm + cus_perm + businessRole 组合判断**：
- 组合菜单（网点管理/人员管理/角色配置）：需 sys_perm 授权 **且** cus_perm 含有任一业务权限
- 纯业务菜单（乐器/库存/维修/财务）：仅需对应 cus_perm 代码
- 纯管理菜单（商户/客户端）：仅需对应 sys_perm 位码

角色可见菜单详见 [`docs/permissions.md` §四](./permissions.md#四角色-权限分配矩阵)，各菜单项所需权限详见 [`docs/permissions.md` §五](./permissions.md#五菜单-权限映射)。

### 右上角用户信息

**显示格式**: 👤 **{name}** (**{role}**)

**数据来源**（优先级）:
1. JWT token payload（优先）
   - name: name, username, preferred_username, displayName, nickName, nickname
   - email: email, mail
   - role: role, roles, authorities
2. localStorage fallback (`user_info`)

**面包屑导航**:
- TuneLoop: 可点击，返回首页
- 乐器管理: 可点击，返回首页（在相关页面）

### 个人中心页面

**路由**: `/user/profile`  
**组件**: `frontend-pc/src/pages/UserProfile.jsx`  
**权限**: 所有已登录用户可见

**页面布局**:

```
┌──────────────────────────────────────┐
│  个人中心                              │
├──────────────────────────────────────┤
│  基本信息                              │
│  ┌──────────────────────────────────┐│
│  │ 用户名: xxx                       ││
│  │ 姓名: xxx                        ││
│  │ 邮箱: xxx@example.com             ││
│  │ 角色: site_admin                  ││
│  └──────────────────────────────────┘│
├──────────────────────────────────────┤
│  账户安全                              │
│  ┌──────────────────────────────────┐│
│  │ [修改密码]                         ││
│  │ [通过邮件重置密码]（有邮箱时可用）    ││
│  │ 邮箱未配置时重置密码按钮灰显（带提示） ││
│  └──────────────────────────────────┘│
├──────────────────────────────────────┤
│  关联信息                              │
│  ┌──────────────────────────────────┐│
│  │ 关联网点: xxx                      ││
│  │ 网点 ID: xxx                      ││
│  └──────────────────────────────────┘│
└──────────────────────────────────────┘
```

**操作流程**:
1. 用户点击「通过邮件重置密码」按钮
2. 弹出确认框："系统将向您的邮箱 xxx 发送密码重置邮件，邮件中的链接 24 小时内有效"
3. 用户确认后调用 `POST /api/user/reset-password`
4. 后端频率限制：每用户每 30 分钟最多 3 次
5. 后端调用 beaconiam 发送重置邮件
6. 用户在 beaconiam 页面设置新密码
7. 返回成功/失败 Toast 提示

**账户安全按钮**:
- 「修改密码」: 始终显示，跳转 `/user/change-password`
- 「通过邮件重置密码」: 邮箱已配置时可用；未配置时按钮灰显 + 提示"请先配置邮箱后再使用密码重置功能"

---

### 修改密码页面

**路由**: `/user/change-password`  
**组件**: `frontend-pc/src/pages/ChangePassword.jsx`  
**权限**: 所有已登录用户可见

**两种模式**:

1. **普通模式**（从个人中心进入）：
   - 保留侧边栏和顶栏
   - 显示"取消"按钮返回到上一页
   
2. **首次登录强制改密模式**（`?first_login=1`）：
   - 全屏锁定态：无侧边栏、无顶栏、无返回按钮
   - 顶部显示黄色 Alert 提示："首次登录需修改密码"
   - 修改成功后自动跳转首页
   - 前端路由守卫拦截所有导航（防止用户直接改 URL 逃离）

**页面布局**（首次登录模式）:

```
┌──────────────────────────────────────────┐
│  ⚠ 首次登录需修改密码                      │
│  系统要求您在首次登录时设置新密码            │
│                                           │
│  新密码        [················] 👁       │
│  确认新密码    [················] 👁       │
│                                           │
│  密码要求：8 位 + 大小写字母 + 数字          │
│                                           │
│              [确认修改]                    │
└──────────────────────────────────────────┘
```

**密码规则校验**（前后端双重）:
- 长度 ≥ 8
- 至少 1 个大写字母
- 至少 1 个小写字母
- 至少 1 个数字

---

### 用户创建/编辑

> cases.md §0.2 要求："用户搜索与创建功能直接内嵌在表单中，不再使用弹窗对话框"。
> 创建使用面板替换模式：点击按钮 → 列表面板**替换**为创建表单 → 提交成功自动翻转回列表。
> 编辑使用独立路由：点击行「编辑」→ 导航到 `/staff/:id/edit`

**组件**:
- 列表+创建：`frontend-pc/src/pages/StaffManagement.jsx`
- 编辑：`frontend-pc/src/pages/StaffEdit.jsx`
- 密码重置：`frontend-pc/src/pages/StaffResetPassword.jsx`

**位置**: 人员管理页中，使用 `viewMode` 控制（'list' | 'create'）

**交互模式**：

```
┌─────────────────────────────────────────────────┐
│  点击「创建用户」→ viewMode='create'             │
│  ───────────────────────                         │
│  搜索 Tab │ 创建 Tab                             │
│  ───────────────────────                         │
│  输入任意文本 → 300ms debounce → 调 staffApi     │
│  .list({name})                                   │
│                                                  │
│  ↓ 有结果                                       │
│  显示用户列表 [姓名 手机 邮箱] [状态Tag]         │
│                                                  │
│  ↓ 无结果                                       │
│  "未找到匹配用户 → 创建新用户"（可点击切到创建） │
│                                                  │
│  切换到创建 Tab → 显示创建表单：                   │
│   姓名、用户名、邮箱、手机                        │
│   密码设置：自动生成 / 手动设置                   │
│   首次登录强制修改密码 ✓                          │
│   归属网点、角色                                  │
│   [创建用户] [取消] → setViewMode('list')        │
├─────────────────────────────────────────────────┤
│  点击行「编辑」→ navigate('/staff/:id/edit')      │
│  独立页面：/staff/:id/edit                        │
│  通过 navigate state 传递用户数据                 │
│  编辑表单：                                      │
│   姓名、邮箱（改邮箱需确认）、手机                │
│   归属网点、角色                                  │
│   [保存] [取消] → navigate('/staff')             │
└─────────────────────────────────────────────────┘
```

**密码设置区域（创建和密码重置页）**:

```
┌──────────────────────────────────────────┐
│ 密码设置                                  │
│ ○ 自动生成密码                            │
│ ● 手动设置密码  [················] 👁     │
│   要求：8 位 + 大写字母 + 小写字母 + 数字   │
│                                          │
│ ‥ 首次登录时强制修改密码                   │
└──────────────────────────────────────────┘
```

### 3.10.3 密码重置（独立页面）

**路由**: `/staff/:id/reset-password`  
**组件**: `frontend-pc/src/pages/StaffResetPassword.jsx`  
**入口**: 编辑列表中点击用户行「重设密码」按钮

**交互流程**:
1. 进入页面显示用户信息（姓名、邮箱、手机号）
2. 密码设置：自动生成（默认）/ 手动设置（Radio 切换）
3. 手动模式显示密码输入框（8位+大写+小写+数字）
4. Checkbox：首次登录时强制修改密码（默认启用）
5. 点击「确认重置」→ 调 `staffApi.resetPassword(userId, redirectUrl)` → 成功导航回列表

**批量重置**：多选用户后点击顶部「重设密码」→ 保持原有逻辑不变（不跳转路由）

**创建成功后流程**:
- 自动生成密码 → 弹出密码展示 Modal（仅展示一次，含复制按钮）
- 手动设置密码 → 直接创建成功（无 Modal）
- 提交成功后自动 `setViewMode('list')` → 回到成员列表

**编辑成功后流程**:
- 邮箱变更 → 触发 IAM 邮箱变更确认流程
- 提交成功后自动 `setViewMode('list')` → 回到成员列表

### 关键代码位置

- **主布局**: `frontend-pc/src/App.jsx::MainLayout()`
- **用户加载**: lines 66-99 (useEffect)
- **菜单定义**: lines 101-129 (items)
- **面包屑**: lines 146-169 (breadcrumbItems)
- **用户显示**: lines 175-183 (Header)

### 权限判断逻辑 (v2.2 — 10码位图驱动)

> 参见 [`docs/permissions.md` §七](./permissions.md#七权限检查流程)

**核心文件**:
- `frontend-pc/src/config/menuPermissions.js` — 菜单权限规则定义 (含 bits 25-26)
- `frontend-pc/src/App.jsx` — 菜单结构 + 权限过滤
- `frontend-pc/src/hooks/usePermission.js` — hasCusPerm / hasSysPerm 钩子
- `frontend-pc/src/services/api.js` — permissionConfigApi + adminApi
- `backend/services/permission_registry.go` — 10 cus_perm 码定义

### 最后更新记录

- **日期**: 2026-04-17
- **Commit**: $COMMIT_HASH
- **修复内容**:
  - ✅ 中文用户名显示（JWT token 多字段支持）
  - ✅ 面包屑导航点击事件修复
  - ✅ OWNER 角色库存菜单可见性
  - ✅ 添加调试日志

### 构建与部署

```bash
# 拉取最新代码
git pull origin main

# 构建前端
cd frontend-pc && npm install && npm run build

# 验证
cd frontend-pc && npm run build  # 应该成功
```

**注意**: 用户必须重新登录才能看到效果，因为 userInfo 从登录时的 JWT token 加载。

---

## 补充章节 (Consolidated from ui_design.md)

> 以下章节从 `ui_design.md` 合并而来，v2.0 ui.md 中未覆盖。
> 合并日期: 2026-05-01

### 3.0 冷启动向导 (Setup)

**路由**: `/setup`  
**权限**: 无需登录（仅限未初始化系统访问）

**页面流程**:

1. **状态检测**: 页面加载时调用 `GET /api/setup/status`
   - 若 `requires_setup = false` → 自动重定向至登录页 `/`
   - 若 `requires_setup = true` → 显示初始化表单

2. **初始化表单**:
   - 邮箱（输入框，带格式验证）
   - 密码（密码框，强度提示）
   - 确认密码（密码框，一致性验证）
   - 『创建管理员』按钮（提交）

3. **提交处理**:
   - 表单验证通过后调用 `POST /api/setup/init`
   - 显示加载状态
   - 成功后后端返回 OIDC 授权 URL
   - 前端自动跳转至 IAM 完成首次认证

4. **错误处理**:
   - 系统已初始化（403）→ 显示错误并跳转登录页
   - 参数错误（400）→ 高亮显示错误字段

**交互细节**:
- 表单验证实时反馈
- 密码强度可视化（弱/中/强）
- 提交按钮禁用状态管理


## 附录 A: 页面路由清单

| 页面 | 路由 | 权限 |
|------|------|------|
| 登录回调 | `/callback` | 公开 |
| 仪表盘 | `/dashboard` | 需要登录 |
| 乐器列表 | `/instruments/list` | 需要登录 |
| 新增乐器 | `/instruments/new/edit` | OWNER |
| 编辑乐器 | `/instruments/:id/edit` | OWNER |
| 乐器详情 | `/instruments/detail/:id` | 需要登录 |
| 乐器分类 | `/instruments/categories` | 需要登录 |
| 属性管理 | `/instruments/properties` | 需要登录 |
| 订单列表 | `/orders` | 需要登录 |
| 维修工单 | `/maintenance` | 需要登录 |
| 维修师傅管理 | `/maintenance/workers` | MANAGER |
| 维修会话 | `/maintenance/sessions` | MANAGER/技师 |
| 库存调拨 | `/inventory/transfer` | 需要登录 |
| 库存管理&租金设定 | `/inventory/rent-setting` | MANAGER |
| 租赁台账 | `/leases` | 需要登录 |
| 押金流水 | `/deposits` | 需要登录 |
| 财务配置 | `/finance` | ADMIN |
| 网点管理 | `/sites` | 需要登录 |
| 客户管理 | `/clients` | 需要登录 |
| 权限管理 | `/permissions` | ADMIN |
| 租户管理 | `/tenants` | ADMIN |
| 用户租赁列表 | `/user/rentals` | 用户本人 |
| 乐器浏览 | `/instruments` | 需要登录 |
| 乐器详情 | `/instruments/:id` | 需要登录 |
| 订单支付 | `/orders/:id/payment` | 用户本人 |
| 电子合同 | `/user/contracts/:id` | 用户本人 |
| 归还流程 | `/user/rentals/:id/return` | 用户本人 |
| 操作日志 | `/system/audit-logs` | tenant_view (sys_perm bit 5) |
| 库管工作台 | `/warehouse` | MANAGER |
| 申诉处理 | `/appeals` | MANAGER |
| 用户申诉 | `/user/appeals` | 用户本人 |

### 3.17 申诉处理 (AppealManagement)

**路由**: `/appeals`

**权限**: MANAGER

**功能点**:
- 申诉列表
  - 乐器信息（类别、型号）
  - 当前图片
  - 用户、员工信息
  - 员工定损说明
  - 用户申诉理由
  - 租赁过程
- 申诉详情页
  - 完整信息展示
  - 仲裁操作面板
- 仲裁操作
  - 无损坏（取消赔款，直接生成退还事务，乐器在库状态）
  - 调整定损金额
  - 输入仲裁说明
  - 确定（乐器进入维修状态，押金扣除赔款后>0自动生成退还事务）

**状态流转**:
- pending: 待处理（用户申诉提交）
- reviewing: 经理仲裁中
- resolved: 已处理
- canceled: 用户撤销

**交互逻辑**:
- 申诉提交后通知经理
- 仲裁决策需填写完整说明
- 调整金额需合理范围验证
- 处理结果通知双方用户

### 3.18 用户申诉 (UserAppeal)

**路由**: `/user/appeals`

**权限**: 用户本人

**功能点**:
- 申诉列表
- 申诉详情
  - 定损通知（照片、评论、金额）
  - 同意定损按钮
  - 申诉提交表单
- 申诉提交
  - 输入申诉理由
  - 支持上传反驳证据

**状态流转**:
- 收到定损通知后 72 小时内可操作
- 同意：押金扣除，生成退还事务或支付页面
- 申诉：进入经理仲裁流程
- 超时未操作：按申诉处理

### 3.19 库管工作台 / 员工订单管理 (Staff Order Management)

**移动端路由**: `/staff/orders` → `/staff/orders/:id`
**移动端组件**: `frontend-mobile/src/pages/StaffOrders.jsx`, `frontend-mobile/src/pages/StaffOrderDetail.jsx`

**权限**: `businessRole === 'site_admin' || businessRole === 'site_member'`

**入口**: Profile 页「员工功能」→「订单管理」（权限 `order:read`）

#### 订单列表页 (`/staff/orders`)

**功能点**:
- 顶部搜索栏：手动输入订单号 + 扫码按钮（调用 `BarcodeDetector` API 识别二维码）
- 状态筛选 Tab：全部 / 待发货 / 运输中 / 租赁中 / 归还验收
- 订单卡片：订单号、下单人、乐器 SN、到期日、状态标签
- 点击卡片 → 进入订单详情

#### 订单详情页 (`/staff/orders/:id`)

**数据来源**: `GET /api/orders/:id`

**展示项**:
- 订单编号（大字醒目）
- 当前状态标签
- 客户信息：下单人、收货地址
- 租期信息：租期起点、预计天数、预计到期日
- 物流信息（如有）

**状态按钮**:

| 状态 | 按钮 | 目标 | 权限 |
|------|------|------|------|
| `reserved` | 无 | 等待用户支付 | — |
| `paid` / `pending_shipment` | 发货 | /staff/shipping | order:update |
| `in_transit` | 接收并转发 | /staff/shipping | order:update |
| `shipped` | 无 | 乐器已发货，等待用户签收 | — |
| `in_lease` | 无 | 租赁中 | — |
| `expired` | 无 | 租约已超期 ⚠️ | — |
| `returning` | 收货 | /staff/receiving | inventory:manage |
| `returned` / `completed` | 无 | 该订单已完成 | — |
| `cancelled` | 无 | 该订单已取消 | — |
| `transferred` | 无 | 已过户 | — |

#### 收货界面 (`/staff/receiving`)

**功能点**:
- 扫码/手动输入订单号 → 搜索 → 进入订单详情
- 订单详情中 `returning` 状态点击「收货」→ 进入收货验收流程

**状态流转（订单维度）**:
- `reserved` → `paid`（支付完成）
- `paid` → `pending_shipment` → `in_transit` → `shipped`（发货流程）
- `shipped` → `in_lease`（用户签收）
- `in_lease` → `returning`（用户发起归还）
- `returning` → `returned`（验收通过）
- `returning` → `completed`（验收通过或维修完成关闭）
- `in_lease` → `expired`（租约超期）
- `expired` → `returning`（用户归还）
- `reserved` → `cancelled`（10分钟超时或用户取消）
- `paid` / `pending_shipment` / `in_transit` → `cancelled`（用户取消）
- `in_lease` → `transferred`（租转售完成）

**交互逻辑**:
- 扫码使用移动端相机 + BarcodeDetector API
- 订单详情实时渲染，状态变更后自动刷新
- 发货/收货需填写完整物流信息

---

### 3.20 用户订单详情 (Customer Order Detail)

**路由**: `/order/:id`
**组件**: `frontend-mobile/src/pages/OrderDetail.jsx`

**权限**: 需要登录，仅可查看本人订单（后端 `user_id` 过滤）

**入口**: Profile 页「当前租赁」/「租赁历史」点击订单卡片

**数据来源**: `GET /api/orders/:id`

**展示项**:
- 订单编号（大字醒目）
- 当前状态标签
- 超期警告条（红框，显示逾期天数 + 累计逾期费 + 日费率）
- 配送信息：下单人、收货地址
- 租期信息：租期起点、预计天数、预计到期日
- 费用信息：月租金、押金、逾期费（如有）
- 物流信息（如有）

**状态按钮**:

| 状态 | 按钮 | 行为 | 确认弹窗 |
|------|------|------|---------|
| `reserved` | 支付 | `POST /orders/:id/pay` | 二次确认 |
| `paid` | 取消订单 | `POST /orders/:id/cancel` | 二次确认 |
| `pending_shipment` | 取消订单 | `POST /orders/:id/cancel` | 二次确认 |
| `in_transit` | 取消订单 | `POST /orders/:id/cancel` | 二次确认 |
| `shipped` | 确认收货 | 跳转 `/receive/:id` | — |
| `in_lease` | 归还 | 跳转 `/return/:id` | — |
| `expired` | 归还 | 跳转 `/return/:id` | — |
| `returning` | 无 | 显示「乐器归还中，等待验收」 | — |
| `cancelled` | 无 | 显示「该订单已取消」 | — |
| `returned` / `completed` / `transferred` | 无 | 显示「该订单已完成」 | — |

**超期提醒**:
- 当 `status === 'expired'` 或 `in_lease` 且 `end_date < now` 时，在状态区域下方显示醒目红框
- 内容：超期 X 天 · 累计逾期费 ¥XXX（¥XX/天）

**路由**: `/user/rentals`

**权限**: 用户本人

**功能点**:
- 租赁会话列表
  - 乐器类别
  - 到期时间
  - 当前状态
- 租赁详情
  - 乐器信息（图片、品牌、型号）
  - 租赁周期
  - 日/周/月租金
  - 押金说明
- 归还操作
  - 点击期满的租赁会话
  - 输入物流信息
  - 提交归还请求

**状态说明**:
- active: 租赁中
- expiring_soon: 即将到期（3天内）
- overdue: 已逾期

### 3.21 乐器浏览 (InstrumentBrowse)

**路由**: `/instruments`

**权限**: 公开（需登录）

**功能点**:
- 乐器列表展示
  - 最新一批图片
  - 品牌、型号、简介
  - 日租金
  - 所在网点
- 筛选工具栏
  - 类别筛选
  - 网点筛选
  - 级别筛选
  - 可租状态筛选
- 排序选项
  - 价格从低到高
  - 距离最近
  - 评分最高

### 3.22 乐器详情 (InstrumentDetail)

**路由**: `/instruments/:id`

**权限**: 公开（需登录）

**功能点**:
- 多媒体管理
  - 当前展示图片缩略图预览
  - 当前视频播放 + 删除按钮
  - 历史批次列表（按 `batch_id` 分组，含 `batch_type` 标签、创建时间）
  - 每批次支持"设为展示"（调用 `PUT /api/instruments/:id/media/display`）和"删除"操作
  - 无媒体时显示 `<Empty>` 占位
  - 数据来源：`GET /api/instruments/:id/media`（不依赖 `instrument.media` 中的旧 `display`/`video` 字段）
- 图片轮播（最新一批图片）
- 基础信息
  - 品牌、型号
  - 简介
  - 分类、级别
- 租金信息
  - 日租金（instrument.pricing[0].daily_rent）
  - 周租金（instrument.pricing[0].weekly_rent，未定义时使用 daily_rent×7×0.9 回退）
  - 月租金（instrument.pricing[0].monthly_rent，未定义时使用 daily_rent×30×0.8 回退）
  - 押金说明
- 下单按钮
  - 指定租赁时间（起止日期）
  - 确认收货地址
  - 跳转支付界面

### 3.23 订单支付 (OrderPayment)

**路由**: `/orders/:id/payment`

**权限**: 用户本人

**功能点**:
- 订单确认
  - 乐器信息
  - 租赁时间
  - 租金明细（按天/周/月计算）
  - 押金金额
- 收货地址确认
- 支付方式选择
- 支付确认

**说明**: 完成支付后，乐器进入预订状态

### 3.24 电子合同 (LeaseContract)

**路由**: `/user/contracts/:id`

**权限**: 用户本人

**功能点**:
- 合同/收据展示
  - PDF 格式
  - 租赁凭证
- 下载功能
- 存储位置：用户的"我的资料"中

**生成时机**: 支付完成后自动生成

### 3.25 归还流程 (ReturnProcess)

**路由**: `/user/rentals/:id/return`

**权限**: 用户本人

**功能点**:
- 归还确认
  - 确认归还乐器 SN
  - 确认租赁结束时间
- 物流信息录入
  - 物流公司
  - 物流单号
- 提交归还

**状态变更**:
- 提交后租赁会话状态变为 returning
- 系统生成归还通知给库管

---

*Model: glm-5*

---

### 3.99 本地化规范 (Localization Rule)

**生效范围**: PC 管理端 (`frontend-pc`) + 微信小程序端 (`frontend-mobile`) + 后端返回的错误信息

**原则**: 本项目面向国内市场，所有面向用户的可见文本必须使用中文。

**具体要求**:

| 类别 | 必须中文 | 可保留英文 |
|------|---------|-----------|
| 页面标题/章节标题 | ✅ | — |
| 按钮/链接文本 | ✅ | — |
| 表单标签/占位符 | ✅ | — |
| 表格列名 | ✅ | — |
| Toast/Alert 提示 | ✅ | — |
| 空状态/加载状态提示 | ✅ | — |
| 图表标签/图例 | ✅ | — |
| 后端错误信息 message | ✅ | — |
| 代码变量名/函数名/属性名 | — | ✅ |
| URL 路径/路由参数 | — | ✅ |
| 日志输出 (console.log) | — | ✅ |
| 状态枚举值 (如 stock_status) | — | ✅ |

**注意**: 状态枚举值在底层使用英文（如 `"pending"`, `"rented"`），但在渲染时必须映射为中文显示（如 `"待支付"`, `"在租"`）。
