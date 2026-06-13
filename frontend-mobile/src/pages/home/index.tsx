import { Component } from 'react'
import { View, Text, Image, ScrollView } from '@tarojs/components'
import './index.scss'

export default class Home extends Component {
  state = {
    instruments: [],
  }

  render() {
    return (
      <View className="bg-gray-50 min-h-screen">
        <View className="bg-white px-4 py-3 flex items-center justify-between sticky top-0 z-10">
          <Text className="text-xl font-bold">TuneLoop</Text>
          <View className="flex items-center gap-3">
            <Text className="text-gray-400">搜索</Text>
            <Text className="text-gray-400">收藏</Text>
          </View>
        </View>

        <ScrollView className="px-4 py-3">
          <View className="bg-white rounded-xl p-4 mb-3">
            <Text className="text-gray-500">欢迎使用 TuneLoop</Text>
            <Text className="text-lg font-semibold mt-1">发现你的乐器</Text>
          </View>

          <View className="text-center py-10 text-gray-400">
            <Text>乐器列表加载中...</Text>
          </View>
        </ScrollView>
      </View>
    )
  }
}
