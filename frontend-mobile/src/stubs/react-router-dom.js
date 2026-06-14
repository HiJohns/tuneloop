import Taro from '@tarojs/taro'

const STATE_KEY = '__nav_state__'

export function useNavigate() {
  return (to, options) => {
    if (to === -1) {
      Taro.navigateBack()
      return
    }
    if (options?.state) {
      Taro.setStorageSync(STATE_KEY, JSON.stringify(options.state))
    }
    const taroUrl = '/pages' + to + '/index'
    if (options?.replace) {
      Taro.redirectTo({ url: taroUrl })
    } else {
      Taro.navigateTo({ url: taroUrl })
    }
  }
}

export function useParams() {
  return Taro.getCurrentInstance().router?.params || {}
}

export function useSearchParams() {
  const params = Taro.getCurrentInstance().router?.params || {}
  const searchStr = Object.entries(params)
    .map(([k, v]) => `${k}=${encodeURIComponent(String(v))}`)
    .join('&')
  return [new URLSearchParams(searchStr)]
}

export function useLocation() {
  let state = {}
  try {
    const raw = Taro.getStorageSync(STATE_KEY)
    if (raw) {
      state = JSON.parse(raw)
      Taro.removeStorageSync(STATE_KEY)
    }
  } catch {}
  const router = Taro.getCurrentInstance().router
  return {
    pathname: '/' + (router?.path || ''),
    search: '',
    state,
  }
}

export function BrowserRouter({ children }) {
  return children
}

export function Routes({ children }) {
  return children
}

export function Route() {
  return null
}


