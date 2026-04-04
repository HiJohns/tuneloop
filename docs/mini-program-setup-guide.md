# Mini-Program Setup Guide for Outbound Confirmation

This document outlines the steps to create the WeChat mini-program for outbound confirmation as specified in Issue #206.

## Required Mini-Program Structure

```
frontend-wx/
├── app.js
├── app.json
├── app.wxss
├── pages/
│   └── outbound/
│       ├── outbound.js
│       ├── outbound.json
│       ├── outbound.wxml
│       └── outbound.wxss
├── utils/
│   └── api.js
└── project.config.json
```

## Implementation Steps

### 1. Initialize Mini-Program Project

```bash
mkdir -p frontend-wx/pages/outbound frontend-wx/utils
```

### 2. App Configuration

**app.json** - Register the outbound page:
```json
{
  "pages": [
    "pages/outbound/outbound"
  ],
  "window": {
    "backgroundTextStyle": "light",
    "navigationBarBackgroundColor": "#fff",
    "navigationBarTitleText": "出库确认",
    "navigationBarTextStyle": "black"
  }
}
```

### 3. Outbound Confirmation Page

**outbound.wxml** - UI template:
```xml
<view class="container">
  <view class="header">
    <text class="title">出库确认</text>
  </view>
  
  <view class="instrument-info" wx:if="{{instrument}}">
    <text class="label">乐器名称: {{instrument.name}}</text>
    <text class="label">品牌: {{instrument.brand}}</text>
    <text class="label">型号: {{instrument.model}}</text>
  </view>
  
  <view class="photo-section">
    <text class="section-title">入库照片</text>
    <view class="photo-grid">
      <image wx:for="{{photos}}" wx:key="*this" src="{{item}}" mode="aspectFill" class="photo"></image>
    </view>
  </view>
  
  <view class="confirm-section">
    <checkbox value="{{confirmed}}" bindtap="toggleConfirm">我已确认以上照片与实物相符</checkbox>
    
    <button type="primary" bindtap="confirmOutbound" disabled="{{!confirmed || loading}}" loading="{{loading}}">
      确认出库
    </button>
  </view>
</view>
```

**outbound.js** - Page logic:
```javascript
const api = require('../../utils/api')

Page({
  data: {
    orderId: '',
    instrument: null,
    photos: [],
    confirmed: false,
    loading: false
  },
  
  onLoad(options) {
    this.setData({
      orderId: options.order_id
    })
    this.fetchOutboundData()
  },
  
  async fetchOutboundData() {
    try {
      const result = await api.get(`/orders/${this.data.orderId}/outbound-photos`)
      if (result.code === 20000) {
        this.setData({
          instrument: result.data,
          photos: result.data.photos || []
        })
      } else {
        wx.showToast({
          title: '获取数据失败',
          icon: 'none'
        })
      }
    } catch (error) {
      wx.showToast({
        title: '网络错误',
        icon: 'none'
      })
    }
  },
  
  toggleConfirm() {
    this.setData({
      confirmed: !this.data.confirmed
    })
  },
  
  async confirmOutbound() {
    if (!this.data.confirmed) {
      wx.showToast({
        title: '请先确认照片',
        icon: 'none'
      })
      return
    }
    
    this.setData({ loading: true })
    
    try {
      const result = await api.post(`/orders/${this.data.orderId}/outbound-confirm`, {
        confirmed: true,
        userId: wx.getStorageSync('userId')
      })
      
      if (result.code === 20000) {
        wx.showToast({
          title: '出库确认成功',
          icon: 'success',
          complete: () => {
            setTimeout(() => {
              wx.navigateBack()
            }, 1500)
          }
        })
      } else {
        wx.showToast({
          title: result.message || '确认失败',
          icon: 'none'
        })
      }
    } catch (error) {
      wx.showToast({
        title: '网络错误',
        icon: 'none'
      })
    } finally {
      this.setData({ loading: false })
    }
  }
})
```

### 4. API Utility

**utils/api.js** - API wrapper:
```javascript
const API_BASE_URL = 'http://opencode.linxdeep.com:5554/api'

function request(url, method = 'GET', data = {}) {
  return new Promise((resolve, reject) => {
    wx.request({
      url: `${API_BASE_URL}${url}`,
      method: method,
      data: method === 'GET' ? {} : data,
      header: {
        'Content-Type': 'application/json',
        'Authorization': `Bearer ${wx.getStorageSync('token')}`
      },
      success(res) {
        if (res.statusCode === 200) {
          resolve(res.data)
        } else {
          reject(new Error(`HTTP ${res.statusCode}`))
        }
      },
      fail(err) {
        reject(err)
      }
    })
  })
}

module.exports = {
  get: (url) => request(url, 'GET'),
  post: (url, data) => request(url, 'POST', data)
}
```

## Backend API Ready

The backend API endpoints have been implemented:
- `GET /api/orders/:order_id/outbound-photos`
- `POST /api/orders/:order_id/outbound-confirm`

## Next Steps

1. Initialize WeChat Developer Tools project in `frontend-wx/`
2. Copy the file structure above
3. Configure `project.config.json` with your appId
4. Test in WeChat Developer Tools

**Note**: This task involves creating 10+ new files for a complete mini-program structure, which exceeds the typical 3-file threshold for splitting tasks.