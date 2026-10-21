import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import RichContent from '../components/RichContent'

export default function ContentPage() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate?.()
  // 跨端统一 query 约定（#1674）：跳转一律 ?key=
  const key = searchParams.get('key') || ''
  const [content, setContent] = useState('')
  // #2258: 合并模式的分段内容 [{ key, html }]
  const [sections, setSections] = useState([])
  const [loading, setLoading] = useState(true)

  // #1840: titles follow the canonical 10-item sequence; rental_notice
  // removed (legacy key no longer reachable from any UI entry).
  const titles = {
    contact_us: '联系我们',
    cooperation: '商务合作',
    rental_agreement: '租用服务协议',
    user_agreement: '个人信息查询授权书',
    privacy_policy: '个人信息保护政策',
    digital_certificate: '数字证书授权使用协议',
    damage_standard: '《乐器损耗与赔偿标准》细则',
    platform_rules: '平台规则文档',
    merchant_audit_requirements: '平台入驻审核要求与规范',
    merchant_agreement: '商家入驻协议',
    related_agreements: '相关协议内容',
  }

  // #2258: 合并阅读页 — 一个入口连续显示两项协议正文
  const RELATED_AGREEMENT_KEYS = ['rental_agreement', 'privacy_policy']

  // #1686: default content when the backend has no record yet — the entry
  // must never feel like a dead page.
  const DEFAULT_CONTENT = {
    cooperation: '商务合作联系方式：\n邮箱：business@cadenzayueqi.com\n电话：400-xxx-xxxx（工作日 9:00-18:00）\n\n欢迎乐器品牌、教育机构、渠道伙伴洽谈合作。',
    contact_us: '联系我们：\n客服电话：400-xxx-xxxx\n客服邮箱：service@cadenzayueqi.com\n服务时间：每日 9:00-21:00\n\n如遇问题请前往「我的-设置」查看协议条款，或联系门店工作人员。',
    platform_rules: '暂无内容',
    merchant_audit_requirements: '暂无内容',
    merchant_agreement: '暂无内容',
  }

  const goBack = () => {
    if (env.isMiniProgram) {
      Taro.navigateBack()
    } else if (navigate) {
      navigate(-1)
    }
  }

  useEffect(() => {
    // #2258: 单段请求，失败时回退默认内容（不整页失败）
    const fetchOne = async (k) => {
      const res = await apiFetch(`${env.apiBaseUrl}/public/settings/${k}`)
      const result = await res.json()
      if (result.code === 20000) {
        return result.data?.value || DEFAULT_CONTENT[k] || '暂无内容'
      }
      return '暂无内容'
    }
    const fetchContent = async () => {
      try {
        if (key === 'related_agreements') {
          const htmls = await Promise.all(RELATED_AGREEMENT_KEYS.map(fetchOne))
          setSections(RELATED_AGREEMENT_KEYS.map((k, i) => ({ key: k, html: htmls[i] })))
        } else {
          const res = await apiFetch(`${env.apiBaseUrl}/public/settings/${key}`)
          const result = await res.json()
          if (result.code === 20000) {
            setContent(result.data?.value || DEFAULT_CONTENT[key] || '暂无内容')
          }
        }
      } catch {
        setContent('加载失败')
      }
      // #2258: weapp 走原生导航栏，动态设置标题（兼容单协议/合并两种）
      if (env.isMiniProgram) {
        Taro.setNavigationBarTitle({ title: titles[key] || '相关协议内容' })
      }
      setLoading(false)
    }
    if (key) fetchContent()
  }, [key])

  return (
    <View style={{ backgroundColor: "#FDFBF7" }} className="min-h-screen">
      {/* Navigation bar — H5 only, weapp uses native nav (#1511) */}
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: "#FDF4E7" }} className="flex items-center px-4 pt-3 pb-2">
          <Text className="text-xl font-bold text-black mr-4" onClick={goBack}>❮</Text>
          <Text className="text-lg font-black text-black">{titles[key] || '内容'}</Text>
        </View>
      )}
      <View className="px-4 py-4">
        {loading ? (
          <Text className="text-zinc-400">加载中...</Text>
        ) : key === 'related_agreements' ? (
          sections.map((seg, i) => (
            <View key={seg.key}>
              {i > 0 && (
                <View style={{ height: 1, backgroundColor: '#e4e4e7', marginTop: 24, marginBottom: 24 }} />
              )}
              <RichContent html={seg.html} />
            </View>
          ))
        ) : (
          <RichContent html={content} />
        )}
      </View>
    </View>
  )
}
