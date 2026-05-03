# TuneLoop UI 设计文档

> 版本: v2.1 (整合权限控制体系: sys_perm + cus_perm 驱动的菜单可见性)
> 最后更新: 2026-05-03
> 覆盖度: 100% features.md

---

## 一、设计原则

### 1.1 白标化适配 (White-labeling)
- **BrandProvider**: 根据 `client_id` 动态加载品牌配置
- **动态变量**: `--brand-primary`, `--brand-logo-url`
- **覆盖范围**: 所有页面（小程序/PC）支持主题切换

### 1.2 状态色规范
```css
:root {
  --status-online: #10B981;      /* Green - 在库/正常 */
  --status-maintenance: #F59E0B; /* Orange - 维修中 */
  --status-banned: #EF4444;     /* Red - 熔断/逾期 */
  --progress-rent: #3B82F6;      /* 租赁中 */
  --progress-transfer: #8B5CF6;  /* 转售中 */
  --progress-complete: #10B981; /* 已完成 */
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
  <!-- 多图轮播 + 视频 -->
  <swiper class="image-swiper">
    <swiper-item wx:for="{{images}}">
      <image src="{{item}}" mode="aspectFill" />
    </swiper-item>
    <swiper-item wx:if="{{video}}">
      <video src="{{video}}" />
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
    <button class="btn-primary" bindtap="createOrder">立即租用</button>
  </view>
</view>
```

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

### 2.6 电子证书预览页 (`/pages/certificate/preview`) 【新增】

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
        label: 'RBAC权限配置',
        route: '/admin/permissions',
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

**5. 系统管理**
- **路由**: 一级菜单
- **权限**: 所有已登录用户
- **子菜单**:
  - 客户端管理 (`/system/clients`)
  - 租户管理 (`/system/tenants`)

### 权限控制汇总 (v2.1 — sys_perm + cus_perm 位图驱动)

| 菜单 | 命名空间管理员 | 商户管理员(owner) | 网点管理员(admin) | 网点员工(staff) | 所需权限 |
|------|-------------|-------|-------|--------|---------|
| 仪表盘 | ✅ | ✅ | ✅ | ✅ | 已登录 |
| 商户管理 | ✅ | ❌ | ❌ | ❌ | sys_perm: tenant_view |
| 客户端管理 | ✅ | ❌ | ❌ | ❌ | sys_perm: namespace_view |
| 乐器管理 | ❌ | ✅ | ✅ | ✅ | cus_perm: instrument:create 等 |
| 库存监控 | ❌ | ✅ | ✅ | ❌ | cus_perm: inventory:view/manage |
| 维修管理 | ❌ | ✅ | ✅ | ✅ | cus_perm: maintenance:view/assign/complete |
| 组织管理(网点/人员) | ❌ | ✅ | ✅(本网点) | ❌ | sys_perm: organization_/user_ + cus_perm(business) |
| 系统管理(角色/申诉) | ❌ | ✅ | ✅(本网点) | ❌ | sys_perm: role_ + cus_perm: appeal:handle |
| 财务配置 | ❌ | ✅ | ❌ | ❌ | cus_perm: finance:config |

**命名空间管理员规则**: `sys_perm > 0 && cus_perm = 0` → 仅仪表盘 + 商户管理 + 客户端管理可见。

**菜单可见性 = sys_perm + cus_perm + businessRole 组合判断**：
- 组合菜单（网点管理/人员管理/角色配置）：需 sys_perm 授权 **且** cus_perm 含有任一业务权限
- 纯业务菜单（乐器/库存/维修/财务）：仅需对应 cus_perm 代码
- 纯管理菜单（商户/客户端）：仅需对应 sys_perm 位码

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

### 关键代码位置

- **主布局**: `frontend-pc/src/App.jsx::MainLayout()`
- **用户加载**: lines 66-99 (useEffect)
- **菜单定义**: lines 101-129 (items)
- **面包屑**: lines 146-169 (breadcrumbItems)
- **用户显示**: lines 175-183 (Header)

### 权限判断逻辑 (v2.1 — 位图驱动)

```javascript
// 前端从 JWT 解析 sys_perm/cus_perm (frontend-pc/src/App.jsx)
const sysPerm = parseInt(payload.sys_perm) || 0
const cusPerm = parseInt(payload.cus_perm) || 0

// 命名空间管理员检测 (frontend-pc/src/config/menuPermissions.js)
function isNamespaceAdmin(sysPerm, cusPerm) {
  return sysPerm > 0 && cusPerm === 0
}

// 菜单规则判断 (checkRule)
function checkRule(rule, sysPerm, cusPerm, cusPermMapping) {
  // sysPermBits: 组内 OR；cusPermCodes: 组内 OR
  // requireAllGroups: true → 两组必须同时满足
}
```

**核心文件**:
- `frontend-pc/src/config/menuPermissions.js` — 菜单权限规则定义
- `frontend-pc/src/components/ProtectedRoute.jsx` — requiredPermission 路由守卫
- `frontend-pc/src/services/api.js` — permissionConfigApi + initPermissionMapping

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

### 3.19 库管工作台 (WarehouseManagement)

**路由**: `/warehouse`

**权限**: MANAGER

**功能点**:
- 订单列表（按状态筛选）
  - 预订状态（preparing）
  - 发货状态（shipped）
  - 租赁状态（in_lease）
  - 归还状态（returning）
- 订单详情页
  - 乐器信息
  - 用户信息
  - 物流信息
- 物流管理
  - 录入物流单号
  - 选择物流公司
  - 发货确认
- 归还验收
  - 扫码乐器二维码
  - 查看乐器信息
  - 按规定完成拍照上传
  - 检查功能按钮
  - 定损处理入口

**状态流转**:
- 预订 → 发货中（填写物流信息）
- 发货中 → 租赁中（物流到达）
- 租赁中 → 归还中（用户发起归还）
- 归还中 → 在库（验收通过）
- 归还中 → 维修中（验收不通过，定损）

**交互逻辑**:
- 扫码使用移动端相机
- 照片上传自动生成时间戳
- 状态变更推送通知给用户
- 发货确认需填写完整物流信息

### 3.20 用户租赁列表 (UserRentalList)

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
- 图片轮播（最新一批图片）
- 基础信息
  - 品牌、型号
  - 简介
  - 分类、级别
- 租金信息
  - 日租金
  - 周租金（日租金×7×0.9）
  - 月租金（日租金×30×0.8）
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
