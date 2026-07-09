import { Component } from 'react'
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
