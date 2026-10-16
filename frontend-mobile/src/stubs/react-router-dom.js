import { useState, useMemo, useCallback } from 'react'
import Taro from '@tarojs/taro'

const STATE_KEY = '__nav_state__'

function readInitialParams() {
  const live = Taro.getCurrentInstance().router?.params
  if (live && Object.keys(live).length > 0) return live
  const pages = Taro.getCurrentPages()
  if (pages.length > 0) {
    return pages[pages.length - 1].options || {}
  }
  return {}
}

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
  const [params] = useState(readInitialParams)
  return params
}

export function useSearchParams() {
  const [params] = useState(readInitialParams)
  // #2225: searchStr 与 URLSearchParams 必须按 params 记忆化——否则每次渲染都
  // 返回新的 URLSearchParams 实例，任何把 searchParams 对象放入 useMemo/useEffect
  // 依赖的页面（如 InvoiceConfirm）都会因身份抖动无限触发 effect。
  const searchStr = useMemo(() =>
    Object.entries(params)
      .map(([k, v]) => `${k}=${encodeURIComponent(String(v ?? ''))}`)
      .join('&')
  , [params])
  const usp = useMemo(() => new URLSearchParams(searchStr), [searchStr])
  // #2225: weapp 无「修改当前页 URL query」的原生等价物；返回稳定 no-op，
  // 使 `const [searchParams, setSearchParams] = useSearchParams()` 在 weapp 不报错。
  const setSearchParams = useCallback(() => {}, [])
  return [usp, setSearchParams]
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


