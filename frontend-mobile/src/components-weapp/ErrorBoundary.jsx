import { Component } from 'react'
import { View, Text } from '@tarojs/components'

export default class ErrorBoundary extends Component {
  state = { error: null }
  componentDidCatch(error) {
    this.setState({ error: '' + error })
  }
  render() {
    if (this.state.error) {
      return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}>
        <Text style={{ color: '#a1a1aa' }}>渲染错误: {this.state.error}</Text>
      </View>
    }
    return this.props.children
  }
}
