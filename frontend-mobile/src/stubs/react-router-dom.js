import { useState } from 'react'
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


