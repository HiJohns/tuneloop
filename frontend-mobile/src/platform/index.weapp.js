import Taro from '@tarojs/taro'

export const storage = {
  getItem: (key) => {
    try { return Taro.getStorageSync(key) } catch { return null }
  },
  setItem: (key, value) => {
    try { Taro.setStorageSync(key, value) } catch {}
  },
  removeItem: (key) => {
    try { Taro.removeStorageSync(key) } catch {}
  },
  getJSON: (key, defaultValue = null) => {
    try {
      const val = Taro.getStorageSync(key)
      return val ? JSON.parse(val) : defaultValue
    } catch { return defaultValue }
  },
  setJSON: (key, value) => {
    try { Taro.setStorageSync(key, JSON.stringify(value)) } catch {}
  },
}

export const session = {
  getItem: (key) => storage.getItem(key),
  setItem: (key, value) => storage.setItem(key, value),
  removeItem: (key) => storage.removeItem(key),
}

export const cookie = {
  get: () => null,
  set: () => {},
  remove: () => {},
}

export const request = (url, options = {}) => {
  return new Promise((resolve, reject) => {
    Taro.request({
      url,
      method: (options.method || 'GET').toUpperCase(),
      data: options.body,
      header: {
        'Content-Type': 'application/json',
        ...options.headers,
      },
      success: (res) => {
        resolve({
          ok: res.statusCode >= 200 && res.statusCode < 300,
          status: res.statusCode,
          statusText: '',
          headers: res.header,
          json: () => Promise.resolve(res.data),
          text: () => Promise.resolve(typeof res.data === 'string' ? res.data : JSON.stringify(res.data)),
          clone: () => ({
            json: () => Promise.resolve(res.data),
            text: () => Promise.resolve(typeof res.data === 'string' ? res.data : JSON.stringify(res.data)),
          }),
        })
      },
      fail: (err) => reject(err),
    })
  })
}

export const uploadFile = (url, filePath, options = {}) => {
  return new Promise((resolve, reject) => {
    Taro.uploadFile({
      url,
      filePath,
      name: options.name || 'file',
      formData: options.formData || {},
      header: options.headers || {},
      success: (res) => {
        resolve({
          ok: res.statusCode >= 200 && res.statusCode < 300,
          status: res.statusCode,
          data: res.data,
        })
      },
      fail: (err) => reject(err),
    })
  })
}

export const previewImage = (args) => Taro.previewImage(args)

export const dialog = {
  alert: (msg) => {
    Taro.showModal({ title: '提示', content: msg, showCancel: false })
  },
  confirm: (msg) => {
    return new Promise((resolve) => {
      Taro.showModal({
        title: '提示',
        content: msg,
        success: (res) => resolve(res.confirm),
      })
    })
  },
  toast: (msg) => {
    Taro.showToast({ title: msg, icon: 'none', duration: 2000 })
  },
}

export const navigation = {
  redirect: (url) => {
    Taro.redirectTo({ url })
  },
  navigateTo: (url) => {
    Taro.navigateTo({ url })
  },
  switchTab: (url) => {
    Taro.switchTab({ url })
  },
  getCurrentPath: () => {
    const pages = Taro.getCurrentPages()
    if (pages.length > 0) {
      const current = pages[pages.length - 1]
      return `/${current.route}`
    }
    return '/'
  },
  getOrigin: () => '',
  getQueryParams: () => {
    const pages = Taro.getCurrentPages()
    if (pages.length > 0) {
      return pages[pages.length - 1].options || {}
    }
    return {}
  },
}

export const eventBus = {
  on: (event, handler) => Taro.eventCenter.on(event, handler),
  off: (event, handler) => Taro.eventCenter.off(event, handler),
  emit: (event) => Taro.eventCenter.trigger(event),
}

export const phone = {
  call: (number) => { Taro.makePhoneCall({ phoneNumber: number }) },
}

export const openLink = (url) => { Taro.setClipboardData({ data: url }) }

export const scanQRCode = () => new Promise((resolve, reject) => {
  Taro.scanCode({
    onlyFromCamera: true,
    scanType: ['qrCode', 'barCode'],
    success: (res) => resolve(res.result),
    fail: (err) => reject(err),
  })
})

export const getLocation = () => new Promise((resolve, reject) => {
  Taro.getLocation({
    type: 'gcj02',
    success: (res) => resolve({ latitude: res.latitude, longitude: res.longitude }),
    fail: (err) => reject(err),
  })
})

export const onPageScroll = (handler) => {
  Taro.onPageScroll(handler)
  return () => Taro.offPageScroll(handler)
}

export const env = {
  apiBaseUrl: 'https://wx.cadenzayueqi.com/api',
  iamExternalUrl: '',
  isDev: process.env.NODE_ENV === 'development',
  isWechatBrowser: false,
  isMiniProgram: true,
  isWechat: true,
}

export const getWindowSize = () => {
  try {
    const info = Taro.getSystemInfoSync()
    return { width: info.windowWidth || 375, height: info.windowHeight || 667 }
  } catch {
    return { width: 375, height: 667 }
  }
}

export const wxLogin = () => new Promise((resolve, reject) => {
  const timer = setTimeout(() => reject(new Error('wxLogin timeout')), 5000)
  Taro.login({
    success: (res) => { clearTimeout(timer); resolve(res.code) },
    fail: (err) => { clearTimeout(timer); reject(err) },
  })
})

export const getPhoneNumber = (e) => {
  const { encryptedData, iv } = e.detail || {}
  return { encryptedData, iv, errMsg: e.detail?.errMsg }
}
