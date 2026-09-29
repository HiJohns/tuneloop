import { Component } from 'react'
// First import: inject atob/btoa polyfill before any JWT parsing runs
// (weapp JSCore has no global atob — #1653).
import './platform/polyfill'
import { initializeApp, setInitDeps } from './platform/init'
import { initPermissionMapping, publicRoutes } from './services/api'
import { env } from './platform'
// #2109: E证通 SDK（weapp 自动核身）——顶层无 wx 访问，H5 打包安全；仅 weapp 调用
import { initEid } from './mp_ecard_sdk/main'

// #ifdef H5
import './app.css'
// #endif

setInitDeps(initPermissionMapping, publicRoutes)

class App extends Component {
  componentDidMount() {
    initializeApp()
    // #2109: 注册 E证通返回监听（scene=1038 回调派发）；H5 不调用
    if (env.isMiniProgram) {
      try { initEid() } catch (e) { /* SDK 初始化失败不阻断 App 启动 */ }
    }
  }

  render() {
    return this.props.children
  }
}

export default App
