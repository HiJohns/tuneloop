// frontend-mobile/src/pages/Profile.jsx
import React from 'react';
import { View, Text, Image, ScrollView } from '@tarojs/components';

const ProfilePage = () => {
  return (
    // 全局视口容器
    <View className="container h-screen w-screen bg-zinc-50 overflow-hidden flex flex-col relative antialiased">
      
      <ScrollView className="w-full flex-1" scrollY showScrollbar={false}>
        
        {/* 1. 头部渐变身份区 */}
        <View className="w-full bg-gradient-to-b from-[#FDF4E7] to-white px-6 pt-8 pb-4 flex items-center justify-between relative">
          <View className="flex items-center space-x-4">
            {/* 优雅大圆圈头像 */}
            <View className="w-20 h-20 rounded-full overflow-hidden border-2 border-white shadow-sm flex-shrink-0">
              <Image src="https://example.com/avatar.jpg" className="w-full h-full object-cover" />
            </View>
            {/* 昵称与多状态标签 */}
            <View className="space-y-1.5">
              <Text className="block text-2xl font-black text-black tracking-wide">大昵称</Text>
              <View className="flex flex-wrap gap-1">
                <Text className="bg-[#0084FF] text-white text-[9px] font-black px-1.5 py-0.5 rounded">插画</Text>
                <Text className="bg-[#FF6B00] text-white text-[9px] font-black px-1.5 py-0.5 rounded">标马</Text>
                <Text className="bg-[#00B981] text-white text-[9px] font-black px-1.5 py-0.5 rounded">标符</Text>
                <Text className="bg-[#8A2BE2] text-white text-[9px] font-black px-1.5 py-0.5 rounded">隐藏角色</Text>
              </View>
            </View>
          </View>

          {/* 退出登录：飘在右侧的小胶囊白卡片 */}
          <Button className="m-0 bg-white/80 backdrop-blur-sm border border-zinc-100 text-amber-800 text-xs font-bold px-4 h-8 rounded-full shadow-sm flex items-center justify-center">
            退出登录
          </Button>
        </View>

        {/* 2. 双轨制大 Tab 分流卡片 */}
        <View className="mx-4 bg-white rounded-2xl shadow-sm p-4 flex divide-x divide-zinc-100">
          <View className="flex-1 flex items-center justify-center space-x-2 py-1 active:opacity-70" onClick={navigateToLeases}>
            <Text className="text-xl">📋</Text>
            <Text className="text-base font-bold text-zinc-800">租赁订单</Text>
            <Text className="text-zinc-300 text-xs">❯</Text>
          </View>
          <View className="flex-1 flex items-center justify-center space-x-2 py-1 active:opacity-70" onClick={navigateToRepairs}>
            <Text className="text-xl">🔸</Text>
            <Text className="text-base font-bold text-zinc-800">报修订单</Text>
            <Text className="text-zinc-300 text-xs">❯</Text>
          </View>
        </View>

        {/* 3. 过滤器状态金刚区 */}
        <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 grid grid-cols-4 gap-2 text-center">
          {/* 带红色消息未读通知气泡的全部项 */}
          <View className="flex flex-col items-center justify-center relative py-1 active:bg-zinc-50 rounded-xl">
            <View className="text-2xl mb-1 relative">
              ☑️
              <View className="absolute -top-1 -right-2 bg-[#FF2A55] text-white text-[10px] font-black w-4 h-4 rounded-full flex items-center justify-center border border-white">
                1
              </View>
            </View>
            <Text className="text-xs font-bold text-zinc-700">全部</Text>
          </View>
          <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl">
            <View className="text-2xl mb-1">📥</View>
            <Text className="text-xs font-bold text-zinc-700">待付款</Text>
          </View>
          <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl">
            <View className="text-2xl mb-1">💬</View>
            <Text className="text-xs font-bold text-zinc-700">服务中</Text>
          </View>
          <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl">
            <View className="text-2xl mb-1">✖️</View>
            <Text className="text-xs font-bold text-zinc-700">已完成</Text>
          </View>
        </View>

        {/* 4. 下方通用抽屉式列表树 */}
        <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 divide-y divide-zinc-100">
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center space-x-2">
              <Text className="text-lg">✉️</Text>
              <Text className="text-base font-bold text-zinc-800">系统信息</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center space-x-2">
              <Text className="text-lg">🎁</Text>
              <Text className="text-base font-bold text-zinc-800">收藏</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center space-x-2">
              <Text className="text-lg">💬</Text>
              <Text className="text-base font-bold text-zinc-800">售后、优惠券</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center space-x-2">
              <Text className="text-lg">⚙️</Text>
              <Text className="text-base font-bold text-zinc-800">设置</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center space-x-2">
              <Text className="text-lg">💼</Text>
              <Text className="text-base font-bold text-zinc-800">商务合作</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center space-x-2">
              <Text className="text-lg">📞</Text>
              <Text className="text-base font-bold text-zinc-800">联系我们</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
        </View>

      </ScrollView>

      {/* 5. 底部金刚固定导航栏保持全站不动 */}
      <View className="absolute bottom-0 left-0 right-0 bg-[#5A3B24] border-t border-[#4E321E] py-2 flex justify-around items-center z-50 shadow-2xl flex-shrink-0">
        <View className="flex flex-col items-center justify-center text-white/40">
          <View className="text-xl mb-0.5">🏪</View>
          <Text className="text-[10px] font-medium text-white/50">首页</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40">
          <View className="text-xl mb-0.5">🪕</View>
          <Text className="text-[10px] font-medium text-white/50">租赁</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40">
          <View className="text-xl mb-0.5">🛠️</View>
          <Text className="text-[10px] font-medium text-white/50">维修</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white">
          <View className="text-xl mb-0.5">👤</View>
          <Text className="text-[10px] font-bold text-white">我的</Text>
        </View>
      </View>

    </View>
  );
};

export default ProfilePage;
