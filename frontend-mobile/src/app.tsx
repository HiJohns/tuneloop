import { Component } from 'react'
// First import: inject atob/btoa polyfill before any JWT parsing runs
// (weapp JSCore has no global atob — #1653).
import './platform/polyfill'
import { initializeApp, setInitDeps } from './platform/init'
import { initPermissionMapping, publicRoutes } from './services/api'

// #ifdef H5
import './app.css'
// #endif

setInitDeps(initPermissionMapping, publicRoutes)

class App extends Component {
  componentDidMount() {
    initializeApp()
  }

  render() {
    return this.props.children
  }
}

export default App
