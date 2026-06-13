import { Component } from 'react'
import { View, Text, Image } from '@tarojs/components'
import './index.scss'

export default class Detail extends Component {
  state = {
    instrument: null,
  }

  render() {
    return (
      <View className="bg-gray-50 min-h-screen">
        <View className="bg-white px-4 py-3 flex items-center sticky top-0 z-10">
          <Text className="text-lg">返回</Text>
          <Text className="flex-1 text-center font-medium">乐器详情</Text>
        </View>

        <View className="p-4">
          <View className="bg-white rounded-xl p-4 mb-3">
            <View className="w-full h-48 bg-gray-100 rounded-lg flex items-center justify-center mb-3">
              <Text className="text-gray-400">暂无图片</Text>
            </View>
            <Text className="text-lg font-semibold">加载中...</Text>
          </View>
        </View>
      </View>
    )
  }
}
