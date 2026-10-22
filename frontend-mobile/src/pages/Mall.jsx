import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image } from '@tarojs/components'
import { env, previewImage, useDidShow } from '../platform'
import { apiFetch } from '../services/api'
import { t } from '../i18n'
import BottomNav from '../components/BottomNav'
import BottomNavWeapp from '../components-weapp/BottomNav'

// #2175: 商城页（底条 tab）。
// #2210: 读取平台级商城图片（/public/mall-images）竖排无缝展示；
//        link_url 非空点击跳转，为空则不跳转（仅图片预览）；列表为空回退占位文案。
export default function Mall() {
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl
  const imageBaseUrl = baseUrl.replace(/\/api$/, '')
  const [images, setImages] = useState([])

  const fetchImages = async () => {
    try {
      const res = await apiFetch(`${baseUrl}/public/mall-images`)
      const result = await res.json()
      if (result.code === 20000 && result.data?.list) {
        setImages(result.data.list.map(it => ({
          ...it,
          image_url: it.image_url && it.image_url.startsWith('http') ? it.image_url : `${imageBaseUrl}${it.image_url}`,
        })))
      }
    } catch (e) {
      // 拉取失败：保持占位，不阻断页面
    }
  }

  useEffect(() => { fetchImages() }, [])
  // tabBar 页返回/切换后刷新（#2210 R6）
  useDidShow(() => { fetchImages() })

  const handleTap = (item) => {
    if (item.link_url) {
      navigate(item.link_url)
      return
    }
    // link_url 为空：不跳转，仅预览大图
    previewImage({ urls: images.map(i => i.image_url), current: item.image_url })
  }

  return (
    <View style={{ display: 'flex', flexDirection: 'column', height: '100vh', backgroundColor: '#FDFBF7' }}>
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', textAlign: 'center' }}>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>{t('商城')}</Text>
        </View>
      )}
      {images.length > 0 ? (
        <View style={{ flex: 1, overflow: 'auto' }}>
          {images.map((item, index) => (
            <Image
              key={item.id || index}
              src={item.image_url}
              mode="widthFix"
              onClick={() => handleTap(item)}
              style={{ width: '100%', display: 'block' }}
            />
          ))}
        </View>
      ) : (
        <View style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingBottom: 60 }}>
          <Text style={{ fontSize: 15, color: '#A1A1AA' }}>{t('建设中，敬请期待')}</Text>
        </View>
      )}
      {env.isMiniProgram
        ? <BottomNavWeapp active="mall" />
        : <BottomNav active="mall" navigate={navigate} />}
    </View>
  )
}
