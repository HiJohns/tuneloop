import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, RichText } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function ContentPage() {
  const params = useParams?.() || Taro.getCurrentInstance().router?.params || {}
  const navigate = useNavigate?.()
  const key = params.key || ''
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)

  const titles = {
    rental_notice: '租赁须知',
    contact_us: '联系我们',
  }

  const goBack = () => {
    if (env.isMiniProgram) {
      Taro.navigateBack()
    } else if (navigate) {
      navigate(-1)
    }
  }

  useEffect(() => {
    const fetchContent = async () => {
      try {
        const res = await apiFetch(`${env.apiBaseUrl}/public/settings/${key}`)
        const result = await res.json()
        if (result.code === 20000) {
          setContent(result.data?.value || '暂无内容')
        }
      } catch {
        setContent('加载失败')
      }
      setLoading(false)
    }
    if (key) fetchContent()
  }, [key])

  return (
    <View className="min-h-screen bg-[#FDFBF7]">
      {/* Navigation bar — H5 only, weapp uses native nav (#1511) */}
      {!env.isMiniProgram && (
        <View className="flex items-center px-4 pt-3 pb-2 bg-[#FDF4E7]">
          <Text className="text-xl font-bold text-black mr-4" onClick={goBack}>❮</Text>
          <Text className="text-lg font-black text-black">{titles[key] || '内容'}</Text>
        </View>
      )}
      <View className="px-4 py-4">
        {loading ? (
          <Text className="text-zinc-400">加载中...</Text>
        ) : /<[a-z][\s\S]*>/i.test(content) ? (
          <RichText nodes={content} />
        ) : (
          <Text className="text-sm text-zinc-700 leading-6 whitespace-pre-wrap">{content}</Text>
        )}
      </View>
    </View>
  )
}
