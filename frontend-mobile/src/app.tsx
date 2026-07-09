import { Component } from 'react'
import Taro from '@tarojs/taro'
import { initializeApp, setInitDeps } from './platform/init'
import { initPermissionMapping, publicRoutes } from './services/api'

// #ifdef H5
import './app.css'
// #endif

setInitDeps(initPermissionMapping, publicRoutes)

class App extends Component {
  componentDidMount() {
    initializeApp()
    if (process.env.TARO_ENV === 'weapp') {
      const updateManager = Taro.getUpdateManager()
      updateManager.onUpdateReady(() => {
        Taro.showModal({
          title: '更新提示',
          content: '新版本已就绪，是否重启应用？',
          success: (res) => { if (res.confirm) updateManager.applyUpdate() },
        })
      })
    }
  }

  render() {
    return this.props.children
  }
}

export default App
