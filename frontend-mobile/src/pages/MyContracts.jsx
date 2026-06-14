import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { contractsApi } from '../services/api'
import { ArrowLeft, FileText, ChevronRight, ExternalLink, Calendar } from 'lucide-react'
import { openLink } from '../platform'

export default function MyContracts() {
  const navigate = useNavigate()
  const [contracts, setContracts] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchContracts = async () => {
      try {
        const resp = await contractsApi.list()
        if (Array.isArray(resp)) {
          setContracts(resp)
        } else if (resp.code === 20000) {
          setContracts(resp.data?.list || [])
        }
      } catch (err) {
        console.error('Failed to fetch contracts:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchContracts()
  }, [])

  if (loading) {
    return <View className="min-h-screen bg-brand-bg flex items-center justify-center">
      <View className="text-gray-500">加载中...</View>
    </View>
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">我的合同</Text>
      </View>

      <View className="p-4 space-y-3">
        {contracts.length === 0 ? (
          <View className="bg-white rounded-xl p-8 text-center text-gray-400">
            <FileText size={48} className="mx-auto mb-3 opacity-50" />
            <Text>暂无合同</Text>
          </View>
        ) : (
          contracts.map(contract => (
            <View
              key={contract.id}
              className="bg-white rounded-xl p-4 cursor-pointer"
              onClick={() => {
                if (contract.contract_url) {
                  openLink(contract.contract_url)
                }
              }}
            >
              <View className="flex justify-between items-start">
                <Text className="text-sm font-medium">{contract.contract_number || '合同'}</Text>
                <Text className="text-xs px-2 py-1 rounded-full bg-green-100 text-green-700">
                  {contract.status === 'active' ? '有效' : contract.status}
                </Text>
              </View>
              <View className="mt-2 text-xs text-gray-500 space-y-1">
                <Text className="flex items-center gap-1">
                  <Calendar size={12} />
                  {contract.generated_at ? new Date(contract.generated_at).toLocaleDateString() : '-'}
                </Text>
              </View>
              {contract.contract_url ? (
                <View className="mt-2 flex items-center gap-1 text-xs text-brand-primary">
                  <ExternalLink size={12} /> 查看/下载 PDF
                </View>
              ) : (
                <Text className="mt-2 text-xs text-yellow-600">PDF 待生成</Text>
              )}
            </View>
          ))
        )}
      </View>
    </View>
  )
}