// RepairServiceComplete — 创建维修服务单「成功」独立页（#2219 从 RepairServiceCreate.jsx 拆出）
import { useMemo } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { dialog, env, toWeappRoute } from '../platform'

export default function RepairServiceComplete() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  // #2225: 双源读取（H5 searchParams + weapp Taro params）。依赖用稳定的字符串
  // 原语 idParam/codeParam，而非 searchParams 对象——避免身份抖动导致 useMemo 无谓重算。
  const idParam = searchParams.get('id') || ''
  const codeParam = searchParams.get('code') || ''
  const { id, code } = useMemo(() => {
    let rid = idParam
    let rcode = codeParam
    try {
      const p = Taro.getCurrentInstance()?.router?.params || {}
      if (!rid) rid = p.id || ''
      if (!rcode) rcode = p.code || ''
    } catch {}
    return { id: rid, code: rcode }
  }, [idParam, codeParam])

  const navReplace = (to) => {
    if (!env.isMiniProgram) return navigate(to, { replace: true })
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.redirectTo({ url: route.url })
  }
  const goBack = () => { if (env.isMiniProgram) Taro.navigateBack(); else navigate(-1) }

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      {/* 页首仅 H5（weapp 用原生导航栏，#2219） */}
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={goBack} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>创建成功</Text>
        </View>
      )}
      <View style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 14 }}>
        <View style={{ backgroundColor: '#FFFFFF', borderRadius: 12, padding: 20, display: 'flex', flexDirection: 'column', gap: 10, alignItems: 'center' }}>
          <Text style={{ fontSize: 13, color: '#71717A' }}>您的维修编码</Text>
          <Text style={{ fontSize: 40, fontWeight: 'bold', color: '#18181B', letterSpacing: 6 }}>{code}</Text>
          <Text style={{ fontSize: 12, color: '#D97706', textAlign: 'center' }}>请将该编码写在物流单信息栏，作为唯一标记</Text>
        </View>
        <Button onClick={() => navReplace(`/repair-service-detail?order_id=${id}`)}
          style={{ width: '100%', margin: 0, height: 46, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 15, fontWeight: 'bold' }}>
          查看维修单
        </Button>
        <Button onClick={() => navReplace('/my-repairs')}
          style={{ width: '100%', margin: 0, height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#F4F4F5', color: '#3F3F46', borderRadius: 10, fontSize: 13, fontWeight: 'bold' }}>
          返回维修列表
        </Button>
      </View>
    </View>
  )
}
