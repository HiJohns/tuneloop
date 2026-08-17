import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, RichText } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function ContentPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate?.()
  // 跨端统一 query 约定（#1674）：跳转一律 ?key=
  const key = searchParams.get('key') || ''
  const [content, setContent] = useState('')
  const [loading, setLoading] = useState(true)

  const titles = {
    rental_notice: '租赁须知',
    contact_us: '联系我们',
    cooperation: '商务合作',
    rental_agreement: '租用服务协议',
    user_agreement: '用户协议',
    privacy_policy: '隐私协议',
    digital_certificate: '数字证书授权使用协议',
    damage_standard: '《乐器损耗与赔偿标准》细则',
  }

  // #1686: default content when the backend has no record yet — the entry
  // must never feel like a dead page.
  const DEFAULT_CONTENT = {
    cooperation: '商务合作联系方式：\n邮箱：business@cadenzayueqi.com\n电话：400-xxx-xxxx（工作日 9:00-18:00）\n\n欢迎乐器品牌、教育机构、渠道伙伴洽谈合作。',
    contact_us: '联系我们：\n客服电话：400-xxx-xxxx\n客服邮箱：service@cadenzayueqi.com\n服务时间：每日 9:00-21:00\n\n如遇问题请前往「我的-设置」查看协议条款，或联系门店工作人员。',
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
          setContent(result.data?.value || DEFAULT_CONTENT[key] || '暂无内容')
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
