// frontend-mobile/src/pages/Checkout.jsx
import React from 'react';
import { View, Text, Button, Image } from '@tarojs/components';

const CheckoutPage = () => {
  return (
    // 全局灰底
    <View className="container h-screen w-screen bg-zinc-50 overflow-hidden flex flex-col relative antialiased">
      
      {/* 1. 简易导航顶栏 */}
      <View className="w-full pt-3 pb-2 px-4 flex justify-between items-center bg-white border-b border-zinc-100">
        <Text className="text-xl font-bold text-black" onClick={() => wx.navigateBack()}>❮</Text>
        <Text className="text-lg font-black text-black">确认支付</Text>
        <View className="w-6"></View>
      </View>

      {/* 2. 核心：高智感“电子账单收据”卡片区块 */}
      <View className="p-6 m-4 bg-white rounded-2xl shadow-sm border border-zinc-100 space-y-6 flex flex-col items-center">
        
        {/* 顶部状态 */}
        <View className="text-center space-y-1">
          <Text className="text-xs text-zinc-400 font-bold tracking-widest block uppercase">TOTAL PAYABLE</Text>
          {/* 修正设计稿的乌龙，纯正暗红大字重强调，不带错误括号 */}
          <Text className="text-[#C21838] text-4xl font-black tracking-tight block">
            ¥ 7620.00
          </Text>
        </View>

        {/* 财务拆解对账明细线 */}
        <View className="w-full border-t border-dashed border-zinc-200 pt-4 space-y-2 text-sm text-zinc-500 font-medium">
          <View className="flex justify-between">
            <Text>预付租金全款 (1天)</Text>
            <Text className="text-black font-bold">¥ 2500.00</Text>
          </View>
          <View className="flex justify-between">
            <Text>资产固定押金 (可退)</Text>
            <Text className="text-black font-bold">¥ 5000.00</Text>
          </View>
          <View className="flex justify-between">
            <Text>往返物流配送费</Text>
            <Text className="text-black font-bold">¥ 120.00</Text>
          </View>
        </View>

        {/* 底部合规安全提示 */}
        <View className="w-full bg-zinc-50 p-3 rounded-xl text-[11px] text-zinc-400 leading-normal">
          🔒 暖心提示：资产固定押金将在乐器归还、网点网管质检合格后，按原支付渠道原路退回至您的微信零钱。
        </View>
      </View>

      {/* 3. 支付方式选择区：轻量化平铺，不再搞巨大 Logo */}
      <View className="mx-4 p-4 bg-white rounded-2xl shadow-sm flex items-center justify-between border border-zinc-100">
        <View className="flex items-center space-x-3">
          <Text className="text-2xl">🟢</Text> {/* 微信绿色代币图标 */}
          <View>
            <Text className="block text-base font-black text-black">微信支付</Text>
            <Text className="block text-[11px] text-zinc-400">亿万用户的安全选择</Text>
          </View>
        </View>
        {/* 默认选中的右侧勾选反馈 */}
        <Text className="text-sm font-black text-orange-500">✓</Text>
      </View>

      {/* 4. 底部常驻极简确认支付大按钮 */}
      <View className="absolute bottom-0 left-0 right-0 bg-white p-4 pb-6 border-t border-zinc-100 z-50 flex flex-col items-center">
        <Button className="w-full m-0 bg-[#B98E5F] active:bg-[#A87D50] text-white font-extrabold text-base h-12 rounded-full shadow-md flex items-center justify-center tracking-wider">
          确认支付 ¥ 7620.00
        </Button>
      </View>

    </View>
  );
};

export default CheckoutPage;
