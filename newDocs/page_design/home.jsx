// frontend-mobile/src/pages/Home.jsx
import React from 'react';
import { View, Text, Image, ScrollView, Input } from '@tarojs/components';

const HomePage = () => {
  return (
    // 全局视口容器
    <View className="container h-screen w-screen bg-[#915F38] overflow-hidden flex flex-col relative antialiased">
      
      {/* 主动态滚动容器 */}
      <ScrollView className="w-full flex-1" scrollY scrollWithAnimation enhanced showScrollbar={false}>
        
        {/* A. Banner 轮换区 */}
        <View className="relative w-full h-[240px] bg-[#784A2B] overflow-hidden">
          <Image src="https://pic-placeholder.com/instruments-banner.jpg" className="w-full h-full object-cover" />
          <View className="absolute bottom-3 flex items-center space-x-1.5 justify-center w-full z-10">
            <View className="w-1.5 h-1.5 rounded-full bg-white/40"></View>
            <View className="w-1.5 h-1.5 rounded-full bg-white/40"></View>
            <View className="w-3 h-1.5 rounded-full bg-white"></View>
          </View>

          {/* 1. 极致打磨：完全透明的极简搜索框 */}
          {/* bg-transparent 彻底消灭白雾，border-white/25 提供纤细边界感 */}
          <View className="absolute top-4 left-0 right-0 px-6 flex justify-center z-20">
            <View className="w-full max-w-[480px] h-10 bg-transparent rounded-full flex items-center px-4 border border-white/25 backdrop-blur-sm">
              <Text className="text-white/80 text-base mr-2">🔍</Text>
              <Input placeholder="搜索乐器..." placeholderStyle="color: rgba(255,255,255,0.5)" className="text-white text-sm flex-1" disabled />
            </View>
          </View>
        </View>

        {/* B. 品类菜单条 */}
        <View className="sticky top-0 z-40 bg-[#FDFBF7] py-2 shadow-sm border-b border-zinc-100">
          <ScrollView className="w-full whitespace-nowrap pl-7" scrollX showScrollbar={false}>
            <View className="inline-flex items-center space-x-8 pr-4">
              <Text className="text-lg font-black text-black border-b-2 border-black pb-0.5">钢琴</Text>
              <Text className="text-lg font-bold text-zinc-500/90">立式钢琴</Text>
              <Text className="text-lg font-bold text-zinc-500/90">三角钢琴</Text>
              <Text className="text-lg font-bold text-zinc-500/90">打击乐器</Text>
            </View>
          </ScrollView>
        </View>

        {/* C. 乐器列表区 */}
        <View className="pl-7 pr-0 py-4 space-y-4 bg-[#915F38]">
          
          {/* 卡片：根据数据渲染不同的活泼级别色彩标签 */}
          {instruments.map((item) => (
            <View key={item.id} className="bg-white rounded-l-2xl p-3 flex items-center shadow-md w-full">
              <View className="w-28 h-28 bg-zinc-50 rounded-xl overflow-hidden flex-shrink-0 flex items-center justify-center">
                <Image src={item.cover} className="w-24 h-24 object-contain" />
              </View>

              <View className="flex-1 ml-3 h-28 flex justify-between items-start pr-4 overflow-hidden">
                <View className="flex flex-col space-y-1.5 h-full justify-between py-0.5 min-w-0 flex-1">
                  <View className="w-full min-w-0">
                    <Text className="block text-2xl font-black text-black tracking-wide truncate">{item.name}</Text>
                    <Text className="block text-xs text-zinc-500 font-bold">{item.category_name}</Text>
                  </View>
                  
                  {/* 2. 活泼高亮的大圆角矩形级别标签系统 */}
                  {item.level === 'entry' && (
                    <View className="inline-block bg-[#FF6B00] text-white text-xs font-black px-3.5 py-0.5 rounded-full shadow-sm tracking-wider">
                      入门级
                    </View>
                  )}
                  {item.level === 'professional' && (
                    <View className="inline-block bg-[#0084FF] text-white text-xs font-black px-3.5 py-0.5 rounded-full shadow-sm tracking-wider">
                      专业级
                    </View>
                  )}
                  {item.level === 'master' && (
                    <View className="inline-block bg-[#8A2BE2] text-white text-xs font-black px-3.5 py-0.5 rounded-full shadow-sm tracking-wider">
                      大师级
                    </View>
                  )}
                </View>

                <View className="h-full flex flex-col justify-end text-right self-end ml-2 flex-shrink-0 whitespace-nowrap">
                  <Text className="text-[#C21838] font-black text-3xl tracking-tight">
                    ¥{item.rent}<Text className="text-xs font-bold text-[#C21838]/70"> / 月</Text>
                  </Text>
                </View>
              </View>
            </View>
          ))}

        </View>
      </ScrollView>

      {/* 底部导航栏保持不动 */}
      <View className="absolute bottom-0 left-0 right-0 bg-[#5A3B24] border-t border-[#4E321E] py-2 flex justify-around items-center z-50 shadow-2xl">
        <View className="flex flex-col items-center justify-center text-white">
          <View className="text-xl mb-0.5">🏪</View>
          <Text className="text-[10px] font-bold text-white">首页</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40">
          <View className="text-xl mb-0.5">🪕</View>
          <Text className="text-[10px] font-medium text-white/50">租赁</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40">
          <View className="text-xl mb-0.5">🛠️</View>
          <Text className="text-[10px] font-medium text-white/50">维修</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40">
          <View className="text-xl mb-0.5">👤</View>
          <Text className="text-[10px] font-medium text-white/50">我的</Text>
        </View>
      </View>

    </View>
  );
};

export default HomePage;
