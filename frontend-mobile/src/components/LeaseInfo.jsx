import { View, Text } from '@tarojs/components'
import { Calendar, Clock } from 'lucide-react'

export default function LeaseInfo({ startDate, endDate, leaseTerm, rentalDays }) {
  return (
    <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
      <Text className="text-base font-black text-black mb-3">租期信息</Text>
      <View className="space-y-3">
        {startDate ? (
        <View className="flex items-start gap-3">
          <Calendar size={18} className="text-zinc-400 mt-0.5" />
          <View className="flex items-start flex-1 min-w-0">
            <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">租期起点</Text>
            <Text className="text-sm font-black text-black">{startDate}</Text>
          </View>
        </View>
        ) : null}
        {leaseTerm !== undefined ? (
        <View className="flex items-start gap-3">
          <Clock size={18} className="text-zinc-400 mt-0.5" />
          <View className="flex items-start flex-1 min-w-0">
            <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">预计租期</Text>
            <Text className="text-sm font-black text-black">{rentalDays} 天（{leaseTerm} 个月）</Text>
          </View>
        </View>
        ) : null}
        {endDate ? (
        <View className="flex items-start gap-3">
          <Calendar size={18} className="text-zinc-400 mt-0.5" />
          <View className="flex items-start flex-1 min-w-0">
            <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">预计到期</Text>
            <Text className="text-sm font-black text-black">{endDate}</Text>
          </View>
        </View>
        ) : null}
      </View>
    </View>
  )
}
