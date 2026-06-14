import { Component } from 'react'
import { initializeApp, setInitDeps } from './platform/init'
import { initPermissionMapping, publicRoutes } from './services/api'

import './app.css'

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
