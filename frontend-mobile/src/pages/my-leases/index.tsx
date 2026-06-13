// @ts-nocheck
import { Component } from 'react'
import Taro from '@tarojs/taro'
import { View, Text } from '@tarojs/components'

export default class Page extends Component {
  state = {
    data: null,
    loading: true,
  }

  componentDidMount() {
    this.loadData()
  }

  async loadData() {
    try {
      this.setState({ loading: true })
      const res = await Taro.request({ url: '/api/placeholder' })
      if (res.data.code === 20000) this.setState({ data: res.data.data })
    } catch (err) {
      console.error(err)
    } finally {
      this.setState({ loading: false })
    }
  }

  render() {
    const { data, loading } = this.state
    return (
      <View className='bg-gray-50 min-h-screen p-4'>
        {loading ? (
          <Text className='text-gray-400'>加载中...</Text>
        ) : (
          <View className='bg-white rounded-xl p-4'>
            <Text className='text-lg font-bold'>页面</Text>
          </View>
        )}
      </View>
    )
  }
}
