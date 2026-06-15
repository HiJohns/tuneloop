// frontend-mobile/src/pages/detail/index.tsx
import React, { useState } from 'react';
import { View, Text, Image, ScrollView, Swiper, SwiperItem } from '@tarojs/components';

const InstrumentDetailPage = () => {
  // 模拟数据源，对接你的 7 态及多租户架构
  const [instrument] = useState({
    name: '吉普森（Gibson）Les Paul 大师典藏款复古电吉他',
    category_name: '电吉他',
    level: 'master', // entry | professional | master
    rent_price: 299,
    deposit: 5000,
    site_name: '北京朝阳网点',
    site_address: '北京市朝阳区艺术园区A座',
    site_phone: '010-88888888',
    description: '殿堂级摇滚利器，手工定制桃花心木琴身，搭载经典双整流拾音器。',
    media_list: [
      'https://example.com/guitar1.jpg',
      'https://example.com/guitar2.jpg',
      'https://example.com/guitar3.jpg'
    ]
  });

  return (
    // 全局滚动容器：暖棕底色 bg-[#915F38]，底部留出 120px 空间给常驻结算栏
    <View className="container min-h-screen bg-[#915F38] pb-[120px] flex flex-col relative antialiased">
      
      {/* 顶部常驻自定义简易导航条 */}
      <View className="w-full pt-3 pb-2 px-4 flex justify-between items-center bg-[#FDFBF7] border-b border-zinc-100">
        <Text className="text-xl font-bold text-black" onClick={() => wx.navigateBack()}>❮</Text>
        <Text className="text-lg font-black text-black">乐器详情</Text>
        <Text className="text-sm font-bold text-zinc-700">★ 收藏</Text>
      </View>

      <ScrollView className="w-full flex-1" scrollY scrollWithAnimation showScrollbar={false}>
        
        {/* 1. 顶部轮播图：像素级还原设计稿的“左右露边”高级感 */}
        <View className="w-full py-4 bg-[#FDFBF7]">
          <Swiper
            className="w-full h-[200px]"
            circular
            indicatorDots
            indicatorActiveColor="#915F38"
            indicatorColor="rgba(0,0,0,0.15)"
            previousMargin="36px" // 核心：向左露出上一张图的边
            nextMargin="36px"     // 核心：向右露出下一张图的边
          >
            {instrument.media_list.map((src, index) => (
              <SwiperItem key={index} className="px-2 box-border">
                <View className="w-full h-full bg-zinc-100 rounded-xl overflow-hidden shadow-sm">
                  <Image src={src} className="w-full h-full object-cover" />
                </View>
              </SwiperItem>
            ))}
          </Swiper>
        </View>

        {/* 页面主卡片内容区 */}
        <View className="px-4 mt-4 space-y-3">
          
          {/* 卡片 A：乐器核心主价格信息卡片 */}
          <View className="bg-white rounded-2xl p-4 shadow-sm flex flex-col space-y-3">
            <View className="flex justify-between items-start w-full">
              {/* 左侧：名称（带超长单行截断防御） */}
              <View className="flex-1 min-w-0 pr-4">
                <Text className="block text-2xl font-black text-black tracking-wide truncate">
                  {instrument.name}
                </Text>
              </View>
              {/* 右侧：押金明细（高优先级不缩小换行） */}
              <View className="flex-shrink-0 whitespace-nowrap text-right">
                <Text className="text-[#C21838] font-bold text-base tracking-tight active:opacity-70">
                  押金 ¥{instrument.deposit} <Text className="text-zinc-400 font-normal">❯</Text>
                </Text>
              </View>
            </View>

            {/* 标签与月租金联动行 */}
            <View className="flex items-center space-x-3">
              {/* 级别标签条件矩阵 */}
              {instrument.level === 'entry' && (
                <View className="bg-[#FF6B00] text-white text-[10px] font-black px-2.5 py-0.5 rounded-full shadow-sm">入门级</View>
              )}
              {instrument.level === 'professional' && (
                <View className="bg-[#0084FF] text-white text-[10px] font-black px-2.5 py-0.5 rounded-full shadow-sm">专业级</View>
              )}
              {instrument.level === 'master' && (
                <View className="bg-[#8A2BE2] text-white text-[10px] font-black px-2.5 py-0.5 rounded-full shadow-sm">大师级</View>
              )}
              {/* 核心暗红高对比度价格字号 */}
              <Text className="text-[#C21838] font-black text-xl tracking-tight">
                月租 ¥{instrument.rent_price}/月
              </Text>
            </View>

            {/* 小图标辅助信息网格栏 */}
            <View className="border-t border-zinc-100 pt-3 flex justify-between items-center text-xs text-zinc-500 font-bold">
              <View className="flex items-center space-x-1"><Text>🏠</Text><Text>简介 地址</Text></View>
              <View className="flex items-center space-x-1"><Text>📍</Text><Text>地址 电话</Text></View>
              <View className="flex items-center space-x-1 text-zinc-800"><Text>💬</Text><Text>网点详情页链接</Text></View>
            </View>
          </View>

          {/* 卡片 B：网点描述与媒体流卡片 */}
          <View className="bg-white rounded-2xl p-4 shadow-sm space-y-3">
            <View className="flex justify-between items-center">
              <Text className="text-lg font-black text-black">网点描述</Text>
              <Text className="text-xs text-zinc-400 font-medium">更多网点实境 ❯</Text>
            </View>
            <Text className="block text-sm text-zinc-500 font-medium leading-relaxed">
              {instrument.description}
            </Text>
            
            {/* 横向平铺的网点高清实景图片与视频快照流 */}
            <ScrollView className="w-full whitespace-nowrap pt-1" scrollX showScrollbar={false}>
              <View className="inline-flex space-x-3 pr-4 items-center">
                <Image src="https://example.com/site1.jpg" className="w-20 h-20 rounded-xl bg-zinc-50 flex-shrink-0 object-cover" />
                <Image src="https://example.com/site2.jpg" className="w-20 h-20 rounded-xl bg-zinc-50 flex-shrink-0 object-cover" />
                <Image src="https://example.com/site3.jpg" className="w-20 h-20 rounded-xl bg-zinc-50 flex-shrink-0 object-cover" />
                {/* 独立有设计感的视频播放控制入口按钮 */}
                <View className="w-20 h-20 rounded-xl bg-zinc-100 flex-shrink-0 flex flex-col items-center justify-center border border-zinc-200 active:bg-zinc-200">
                  <Text className="text-lg">▶</Text>
                  <Text className="text-[10px] font-black text-zinc-500 mt-0.5">视频 ❯</Text>
                </View>
              </View>
            </ScrollView>
          </View>

          {/* 卡片 C：规格参数与表格表单清单 */}
          <View className="bg-white rounded-2xl p-4 shadow-sm divide-y divide-zinc-100">
            <View className="flex justify-between items-center py-2.5">
              <Text className="text-base font-bold text-black">属性名</Text>
              <Text className="text-sm text-zinc-400">❯</Text>
            </View>
            <View className="flex justify-between items-center py-2.5">
              <Text className="text-base font-bold text-black">属性值</Text>
              <Text className="text-sm text-zinc-400">❯</Text>
            </View>
            <View className="flex justify-between items-center py-2.5">
              <Text className="text-base font-bold text-black">公共信息</Text>
              <Text className="text-sm text-zinc-400">❯</Text>
            </View>
            <View className="flex justify-between items-center py-2.5">
              <Text className="text-base font-bold text-black">租赁须知</Text>
              <Text className="text-sm text-zinc-400">❯</Text>
            </View>
          </View>

        </View>
      </ScrollView>

      {/* 3. 底部常驻双操纵杆结算面板（高度固定，绝对不参与页面滚动） */}
      <View className="absolute bottom-0 left-0 right-0 bg-[#FDFBF7] border-t border-zinc-100 p-4 pb-2 flex flex-col space-y-2 z-50 shadow-2xl">
        <View className="flex w-full space-x-3">
          {/* 加入购物车按钮：柔和金黄落日渐变色 */}
          <Button className="flex-1 h-12 bg-gradient-to-r from-[#E2B07E] to-[#C98E54] text-white font-black text-base rounded-full shadow-sm active:opacity-90 flex items-center justify-center">
            加入购物车
          </Button>
          {/* 立即租赁按钮：大气华丽的复古橘红渐变色 */}
          <Button className="flex-1 h-12 bg-gradient-to-r from-[#FA5E3C] to-[#E63917] text-white font-black text-base rounded-full shadow-sm active:opacity-90 flex items-center justify-center">
            立即租赁
          </Button>
        </View>
        {/* 底部正中央的合计金额说明文字 */}
        <Text className="block text-center text-xs font-bold text-zinc-400 tracking-wide">
          合计金额：预付全款租金 + 固定押金 + 往返运费
        </Text>
      </View>

    </View>
  );
};

export default InstrumentDetailPage;
