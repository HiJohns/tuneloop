import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Button, Image, Canvas } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env, dialog, toWeappRoute } from '../platform'
import { useNavigate } from 'react-router-dom'
import QRCode from 'qrcode'
import RichContent from '../components/RichContent'
import '../utils/text-encoder'


// 会员规则与权益手册 — backend-editable rich text (#1830 增量); the static
// copy below stays as the fallback when no admin content exists yet,
// consistent with current system mechanics; policy figures stay
// admin-configurable.
const HANDBOOK_SECTIONS = [
  { title: '会员体系', body: '平台设初级 / 中级 / 高级三级会员，按跨商户累计消费金额自动升级，只升不降。' },
  { title: '升级门槛', body: '累计实付消费达到对应档位门槛即自动升级，当前档位门槛见「会员权益」展示。' },
  { title: '返现乐币', body: '每笔实付租单结算完成后，按当前档位返现比例赠送乐币；乐币按元计，可用于后续订单抵扣，具体抵扣上限以当期政策为准。' },
  { title: '乐币使用', body: '下单支付时可使用乐币抵扣租金（抵用比例以当期系统配置为准）；乐币无现金价值、不可转让。' },
  { title: '其他权益', body: '会员专属活动与权益更新，以平台公告及「会员权益」展示为准。' },
]

export default function MembershipCenter() {
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const [qrDataUrl, setQrDataUrl] = useState('')
  const [qrSrc, setQrSrc] = useState('')
  const [showQR, setShowQR] = useState(false)
  const [refCode, setRefCode] = useState('')
  const [benefits, setBenefits] = useState([])
  // #1830 增量: backend-editable handbook rich text (empty → static fallback)
  const [handbookHtml, setHandbookHtml] = useState('')
  const [showHandbook, setShowHandbook] = useState(false)
  const navigate = useNavigate()
  // Cross-end navigation (issue-1673): weapp has no react-router short paths;
  // central toWeappRoute maps H5 paths → /pages-weapp/... page urls.
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }
  const baseUrl = env.apiBaseUrl

  const handleGetPromo = async () => {
    try {
      const resp = await apiFetch(`${baseUrl}/users/me/promo-qrcode`)
      const result = await resp.json()
      console.log('[QR DEBUG] API status:', resp.status, 'code:', result.code)
      console.log('[QR DEBUG] wxacode_base64:', result.data?.wxacode_base64 ? `LEN=${result.data.wxacode_base64.length}` : 'NIL')
      console.log('[QR DEBUG] h5_url:', result.data?.h5_url)
      if (result.code === 20000) {
        setRefCode(result.data.ref_code)
        const h5Url = result.data.h5_url
        if (result.data.wxacode_base64) {
          console.log('[QR DEBUG] using wxacode image')
          setQrSrc('data:image/png;base64,' + result.data.wxacode_base64)
          setShowQR(true)
        } else {
          if (env.isMiniProgram) {
            console.log('[QR DEBUG] fallback to canvas, URL=', h5Url)
            generateQRCanvas(h5Url)
          } else {
            QRCode.toString(h5Url, { type: 'svg', width: 256 }, (err, svg) => {
              if (err) { dialog.toast('二维码生成失败'); return }
              setQrDataUrl('data:image/svg+xml,' + encodeURIComponent(svg))
              setShowQR(true)
            })
          }
        }
      }
    } catch (err) {
      console.log('[QR DEBUG] API exception:', err.message || err)
      dialog.toast('获取推广二维码失败')
    }
  }

  // Function declaration (hoisted): the fetchUser effect above references
  // this during its callback body — a const arrow here trips
  // no-use-before-define even though runtime order is safe.
  function generateQRCanvas(url) {
    Taro.nextTick(() => {
    console.log('[QR DEBUG] generateQRCanvas called, url=', url)
    const query = Taro.createSelectorQuery()
    query.select('#qrCanvas').fields({ node: true, size: true }).exec((res) => {
      console.log('[QR DEBUG] canvas query result:', JSON.stringify(res))
      if (!res || !res[0] || !res[0].node) {
        console.log('[QR DEBUG] canvas not found, res=', res)
        Taro.showToast({ title: '二维码生成失败', icon: 'none' })
        return
      }
      const canvas = res[0].node
      console.log('[QR DEBUG] canvas found, generating QR...')
      const ctx = canvas.getContext('2d')
      canvas.width = 256
      canvas.height = 256
      QRCode.toCanvas(canvas, url, { width: 256, margin: 1 }, (err) => {
        if (err) { console.log('[QR DEBUG] QRCode.toCanvas error:', err); Taro.showToast({ title: '二维码生成失败', icon: 'none' }); return }
        console.log('[QR DEBUG] QRCode.toCanvas ok, exporting temp file...')
        Taro.canvasToTempFilePath({
          canvas,
          success: (r) => { console.log('[QR DEBUG] canvasToTempFilePath ok, path=', r.tempFilePath); setQrSrc(r.tempFilePath); setShowQR(true) },
          fail: (e) => { console.log('[QR DEBUG] canvasToTempFilePath fail:', JSON.stringify(e)); Taro.showToast({ title: '二维码生成失败', icon: 'none' }) },
        })
      })
    })
    })
  }

  const fetchUser = async () => {
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`)
      const result = await resp.json()
      if (result.code === 20000) {
        setUser(result.data)
      }
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchUser() }, [])

  // #1830 增量: fetch the admin-editable handbook (GET public settings).
  // Empty/failed loads fall back to the static HANDBOOK_SECTIONS copy.
  useEffect(() => {
    let cancelled = false
    const fetchHandbook = async () => {
      try {
        const res = await apiFetch(`${env.apiBaseUrl}/public/settings/membership_handbook`)
        const r = await res.json()
        if (!cancelled && r.code === 20000 && r.data?.value) setHandbookHtml(r.data.value)
      } catch {}
    }
    fetchHandbook()
    return () => { cancelled = true }
  }, [])

  // Benefits follow the current membership level (#1830)
  useEffect(() => {
    if (!user?.membership_level_id) { setBenefits([]); return }
    let cancelled = false
    ;(async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/membership/benefits?level_id=${user.membership_level_id}`)
        const result = await resp.json()
        if (result.code === 20000 && !cancelled) setBenefits(result.data?.list || [])
      } catch { /* benefits are optional UI; keep card hidden on failure */ }
    })()
    return () => { cancelled = true }
  }, [user?.membership_level_id])

  return (
    <>
    <ScrollView scrollY style={{ backgroundColor: "#FDFBF7" }} className="h-screen w-screen">
      {/* Navigation bar — H5 only, weapp uses native nav */}
      {!env.isMiniProgram && (
      <View className="flex items-center px-4 py-3 bg-white border-b border-zinc-100">
        <Text className="text-lg mr-2" onClick={() => window.history.back()}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center mr-4">会员中心</Text>
      </View>
      )}

      {/* Membership level card */}
      <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-6">
        <View className="items-center">
          <Text className="text-2xl font-bold text-amber-700">
            {user?.membership_level_name || '普通会员'}
          </Text>
        </View>
      </View>

      {/* Membership benefits card — follows current level (#1830) */}
      {benefits.length > 0 && (
        <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-4">
          <View className="flex items-center justify-between mb-3">
            <Text className="text-sm font-bold text-zinc-800">会员权益</Text>
            <Text className="text-xs font-bold text-amber-700">{user?.membership_level_name || ''}</Text>
          </View>
          <View style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {benefits.map(b => (
              <View key={b.id}>
                <Text className="text-sm font-bold text-zinc-800 block">{b.title}</Text>
                {b.description && <Text className="text-xs text-zinc-500 block mt-1">{b.description}</Text>}
              </View>
            ))}
          </View>
        </View>
      )}

      {/* Stats cards */}
      <View className="mx-4 mt-4">
        <View className="bg-white rounded-2xl shadow-sm p-4">
          <View className="flex justify-between items-center py-3 border-b border-zinc-50">
            <Text className="text-sm text-zinc-500">消费总额</Text>
            <Text className="text-base font-bold text-zinc-800">
              ¥{user?.total_spending ? (Number(user.total_spending) / 100).toLocaleString() : '0'}
            </Text>
          </View>
          <View className="flex justify-between items-center py-3">
            <Text className="text-sm text-zinc-500">乐币</Text>
            <Text className="text-base font-bold text-zinc-800">
              {user?.promo_points ? Number((Number(user.promo_points) / 100).toFixed(1)).toLocaleString() : '0'}
            </Text>
          </View>
          </View>
        </View>



        {/* Promo QR code */}
      <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-4">
        <View className="items-center">
          <Button onClick={handleGetPromo}
            style={{ backgroundColor: '#000', color: '#fff', borderRadius: 999, padding: '10px 24px', fontSize: 14, fontWeight: '700', border: 'none' }}>
            获取推广二维码
          </Button>
          <Text className="text-xs text-zinc-400 mt-2">邀请好友注册，赚取奖励乐币</Text>
        </View>
      </View>

      {/* Off-screen canvas for QR code rendering — must exist before showQR */}
      {env.isMiniProgram && <Canvas type="2d" id="qrCanvas" style="width:256px;height:256px;position:fixed;left:-999px;top:-999px" />}

      {/* QR Code Modal */}
      {showQR && (
        <View className="fixed inset-0 z-50 flex items-center justify-center" style={{ backgroundColor: 'rgba(0,0,0,0.5)' }} onClick={() => setShowQR(false)}>
          <View className="bg-white rounded-2xl p-6 mx-8 flex-col" style={{ display: 'flex', flexDirection: 'column' }} onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-center mb-4">推广二维码</Text>
            <View style={{ alignItems: 'center' }}>
              {env.isMiniProgram ? (qrSrc && <Image src={qrSrc} className="w-48 h-48" mode="aspectFit" />) : (qrDataUrl && <Image src={qrDataUrl} className="w-48 h-48" mode="aspectFit" />)}
            </View>
            <Text className="text-xs text-zinc-400 text-center mt-2">好友扫码注册，你获得奖励</Text>
            <Button onClick={() => setShowQR(false)} className="mt-4 py-2 bg-zinc-100 rounded-xl font-bold text-sm text-zinc-600 w-full">
              关闭
            </Button>
          </View>
        </View>
      )}

      {/* Membership rules & benefits handbook (#1830 增量: backend-editable) */}
      <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-4 mb-8">
        <View className="flex items-center justify-between" onClick={() => setShowHandbook(!showHandbook)}>
          <Text className="text-sm font-bold text-zinc-800">会员规则与权益手册</Text>
          <Text className="text-xs text-zinc-400">{showHandbook ? '收起 ▲' : '展开 ▼'}</Text>
        </View>
        {showHandbook && (handbookHtml ? (
          <View className="mt-3"><RichContent html={handbookHtml} /></View>
        ) : (
          <View className="mt-3" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {HANDBOOK_SECTIONS.map((s, i) => (
              <View key={i}>
                <Text className="text-xs font-bold text-zinc-700 block">{s.title}</Text>
                <Text className="text-xs text-zinc-500 block mt-1" style={{ lineHeight: '1.6' }}>{s.body}</Text>
              </View>
            ))}
          </View>
        ))}
      </View>

      {loading && (
        <View className="flex-1 items-center justify-center mt-20">
          <Text className="text-zinc-400">加载中...</Text>
        </View>
      )}
    </ScrollView>
    </>
  )
}
