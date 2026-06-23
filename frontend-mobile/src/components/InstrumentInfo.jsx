import { useState } from 'react'
import { View, Text, Image } from '@tarojs/components'
import { ChevronLeft, ChevronRight, Package } from 'lucide-react'

export default function InstrumentInfo({ instrument, onClick }) {
  const [imageIndex, setImageIndex] = useState(0)

  const thumbnail = instrument?.thumbnail
  const imgs = (() => {
    try {
      return typeof instrument?.images === 'string' ? JSON.parse(instrument.images) : (instrument?.images || [])
    } catch { return [] }
  })()
  const allImgs = thumbnail ? [thumbnail, ...(Array.isArray(imgs) ? imgs : [])] : (Array.isArray(imgs) ? imgs : [])
  const hasImgs = allImgs.length > 0
  const idx = imageIndex % Math.max(allImgs.length, 1)

  return (
    <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4" onClick={onClick}>
      <Text className="text-base font-black text-black mb-3">乐器信息</Text>

      <View className="relative w-full mb-3 flex items-center justify-center">
        {hasImgs ? (
          <>
            <Image src={allImgs[idx]} alt="" className="w-full h-40 object-cover rounded-lg bg-zinc-100" mode="aspectFit" />
            {allImgs.length > 1 && (
              <>
                <View onClick={e => { e.stopPropagation(); setImageIndex(i => (i - 1 + allImgs.length) % allImgs.length) }}
                  className="absolute left-2 top-1/2 -translate-y-1/2 bg-white/80 rounded-full w-8 h-8 flex items-center justify-center shadow-sm">
                  <ChevronLeft size={18} className="text-zinc-600" />
                </View>
                <View onClick={e => { e.stopPropagation(); setImageIndex(i => (i + 1) % allImgs.length) }}
                  className="absolute right-2 top-1/2 -translate-y-1/2 bg-white/80 rounded-full w-8 h-8 flex items-center justify-center shadow-sm">
                  <ChevronRight size={18} className="text-zinc-600" />
                </View>
                <View className="absolute bottom-2 right-2 bg-black/70 rounded-full px-2 py-0.5">
                  <Text className="text-white text-xs font-bold">{idx + 1}/{allImgs.length}</Text>
                </View>
              </>
            )}
          </>
        ) : (
          <View className="w-24 h-24 bg-zinc-100 rounded-lg flex items-center justify-center">
            <Package size={32} className="text-zinc-300" />
          </View>
        )}
      </View>

      <View className="space-y-1.5">
        <View className="flex justify-between text-sm">
          <Text className="text-zinc-500 font-medium">SN</Text>
          <Text className="text-black font-black">{instrument?.sn || '-'}</Text>
        </View>
        <View className="flex justify-between text-sm">
          <Text className="text-zinc-500 font-medium">类型</Text>
          <Text className="text-black font-black">{instrument?.category_name || '-'}</Text>
        </View>
        {instrument?.level_name ? (
        <View className="flex justify-between text-sm">
          <Text className="text-zinc-500 font-medium">级别</Text>
          <Text className="text-black font-black">{instrument.level_name}</Text>
        </View>
        ) : null}
        {instrument?.tenant_name ? (
        <View className="flex justify-between text-sm">
          <Text className="text-zinc-500 font-medium">商户</Text>
          <Text className="text-black font-black">{instrument.tenant_name}</Text>
        </View>
        ) : null}
        {instrument?.site_name ? (
        <View className="flex justify-between text-sm">
          <Text className="text-zinc-500 font-medium">网点</Text>
          <Text className="text-black font-black">{instrument.site_name}</Text>
        </View>
        ) : null}
      </View>
    </View>
  )
}
