import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView, Button } from '@tarojs/components'
import { addressesApi } from '../services/api'
import AddressForm from '../components/AddressForm'

export default function AddressList() {
  const navigate = useNavigate()
  const [addresses, setAddresses] = useState([])
  const [loading, setLoading] = useState(true)
  const [showForm, setShowForm] = useState(false)
  const [editingAddress, setEditingAddress] = useState(null)

  const fetchAddresses = async () => {
    setLoading(true)
    try {
      const res = await addressesApi.list()
      if (res.code === 20000) {
        setAddresses(res.data?.list || [])
      }
    } catch (err) {
      console.error('Failed to fetch addresses:', err)
    }
    setLoading(false)
  }

  useEffect(() => { fetchAddresses() }, [])

  const handleDelete = (id) => {
    if (!confirm('确认删除此地址？')) return
    addressesApi.delete(id).then(res => {
      if (res.code === 20000) fetchAddresses()
    })
  }

  const handleSetDefault = (id) => {
    addressesApi.setDefault(id).then(res => {
      if (res.code === 20000) fetchAddresses()
    })
  }

  const openForm = (address = null) => {
    setEditingAddress(address)
    setShowForm(true)
  }

  return (
    <View className="h-screen bg-zinc-50 flex flex-col">
      {/* Navigation bar */}
      <View className="flex items-center px-4 py-3 bg-white border-b border-zinc-100">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center mr-4">收货地址</Text>
      </View>

      {/* Address list */}
      <ScrollView scrollY className="flex-1 px-4 pt-4">
        {loading ? (
          <View className="text-center py-16"><Text className="text-zinc-400">加载中...</Text></View>
        ) : addresses.length === 0 ? (
          <View className="text-center py-16"><Text className="text-zinc-400">暂无收货地址</Text></View>
        ) : (
          <View className="space-y-3">
            {addresses.map(addr => (
              <View key={addr.id} className="bg-white rounded-2xl shadow-sm p-4">
                <View className="flex items-start justify-between mb-2">
                  <View className="flex-1">
                    <View className="flex items-center gap-2">
                      <Text className="text-sm font-bold text-black">{addr.recipient_name}</Text>
                      <Text className="text-xs text-zinc-400">{addr.phone}</Text>
                      {addr.is_default && (
                        <Text className="text-xs text-white bg-red-500 px-1.5 py-0.5 rounded">默认</Text>
                      )}
                    </View>
                    <Text className="text-xs text-zinc-500 mt-1">
                      {addr.province} {addr.city} {addr.district} {addr.detail}
                    </Text>
                  </View>
                </View>
                <View className="flex gap-2 mt-2 pt-2 border-t border-zinc-50">
                  <Button
                    onClick={() => openForm(addr)}
                    className="flex-1 py-2 bg-zinc-100 rounded-xl text-xs font-bold text-zinc-600"
                  >编辑</Button>
                  {!addr.is_default && (
                    <Button
                      onClick={() => handleSetDefault(addr.id)}
                      className="flex-1 py-2 bg-zinc-100 rounded-xl text-xs font-bold text-zinc-600"
                    >设为默认</Button>
                  )}
                  <Button
                    onClick={() => handleDelete(addr.id)}
                    className="flex-1 py-2 bg-red-50 rounded-xl text-xs font-bold text-red-500"
                  >删除</Button>
                </View>
              </View>
            ))}
          </View>
        )}
      </ScrollView>

      {/* Add button */}
      <View className="px-4 py-3 bg-white border-t border-zinc-100">
        <Button
          onClick={() => openForm(null)}
          className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center"
        >新增地址</Button>
      </View>

      {/* Address form modal */}
      {showForm && (
        <AddressForm
          address={editingAddress}
          onClose={() => { setShowForm(false); setEditingAddress(null) }}
          onSaved={fetchAddresses}
        />
      )}
    </View>
  )
}
