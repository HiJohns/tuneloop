import { useEffect } from 'react'

const params = () => {
  try {
    return Object.fromEntries(new URLSearchParams(window.location.search).entries())
  } catch {
    return {}
  }
}

const noop = () => {}
const asyncNoop = async () => ({})

export const useDidShow = (callback) => {
  useEffect(() => { callback?.() }, [])
}
export const useLoad = (callback) => {
  useEffect(() => { callback?.(params()) }, [])
}
export const useRouter = () => ({ params: params() })

const Taro = {
  getCurrentInstance: () => ({ router: { params: params() } }),
  useRouter,
  useDidShow,
  useLoad,
  navigateTo: noop,
  redirectTo: noop,
  navigateBack: noop,
  switchTab: noop,
  reLaunch: noop,
  showToast: noop,
  showModal: noop,
  showLoading: noop,
  hideLoading: noop,
  chooseImage: noop,
  chooseMedia: noop,
  previewImage: noop,
  getSystemInfoSync: () => ({ windowWidth: 375, windowHeight: 667, statusBarHeight: 20, platform: 'devtools' }),
  getMenuButtonBoundingClientRect: () => ({ top: 10, height: 32, right: 365, left: 300, width: 65 }),
  getStorageSync: (key) => {
    try { return window.localStorage.getItem(key) || '' } catch { return '' }
  },
  setStorageSync: (key, value) => {
    try { window.localStorage.setItem(key, typeof value === 'string' ? value : JSON.stringify(value)) } catch {}
  },
  removeStorageSync: (key) => {
    try { window.localStorage.removeItem(key) } catch {}
  },
  login: asyncNoop,
  checkSession: asyncNoop,
  request: asyncNoop,
  uploadFile: asyncNoop,
  downloadFile: asyncNoop,
  getLocation: asyncNoop,
  makePhoneCall: noop,
  nextTick: (fn) => setTimeout(fn, 0),
}

export default Taro
