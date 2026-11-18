import { useEffect, useRef, useState } from 'react'
import { View, Text, Button } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'

import { apiFetch, resolveErrorMessage } from '../services/api'
import { env, dialog, toWeappRoute, openCreditAssessment, useDidShow } from '../platform'
import { t } from '../i18n'
import { pendingOrderHint } from '../utils/pendingOrder.mjs'
import {
  CREDIT_STATE,
  creditGate,
  resolveCreditLaunch,
  pollDelay,
  decodeReturnPath,
} from '../utils/creditFlow'

// #2327 S4 / D8 / #2352 / #2333: 通用微信支付分信用评测页（H5 + weapp 共用）。
// 入参（一律 query）：仅 order_id（可选 return_path）。
// #2333 R8: 评估金额由后端 /credit-{status,assessment} 返回 risk_amount_cents，
// 页面只展示、不自算；不再接收 tier / risk_amount_cents 参数，也不承载业务规则（规则在 services/creditFlow.mjs）。

const STATE_TEXT = {
  [CREDIT_STATE.NONE]: '尚未评测',
  [CREDIT_STATE.PENDING]: '评测进行中',
  [CREDIT_STATE.APPROVED]: '评测通过',
  [CREDIT_STATE.REJECTED]: '评测未通过',
  [CREDIT_STATE.EXPIRED]: '评测已过期',
}

export default function CreditAssessment() {
  const navigate = useNavigate()
  const params = env.isMiniProgram
    ? (Taro.getCurrentInstance().router?.params || {})
    : Object.fromEntries(new URLSearchParams(typeof window !== 'undefined' ? window.location.search : ''))

  const orderId = params.order_id || ''
  // #2411：weapp 侧 return_path 常是 URL-encoded 的 H5 路径（支付页 encodeURIComponent 传入），
  // 不解码会导致 toWeappRoute 匹配失败 → 误报「该功能请在微信小程序中完成」。
  const decodedReturnPath = decodeReturnPath(params.return_path || '')
  const returnPath = decodedReturnPath || (orderId ? `/payment?type=rent&id=${orderId}` : '')

  const [credit, setCredit] = useState(null)
  // 评估金额由后端返回；页面只展示，不自算（#2333 R8）。
  const riskAmountCents = Number(credit?.risk_amount_cents || 0)
  // #2352 F1: 订单支付截止倒计时（30s 刷新，纯文本，不影响布局高度）。
  const [nowTick, setNowTick] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => setNowTick(Date.now()), 30000)
    return () => clearInterval(id)
  }, [])
  // #2352 F1: 建单/截止提示与详情页共用同一份格式化逻辑（utils/pendingOrder.mjs）。
  const deadlineLabel = pendingOrderHint(credit?.payment_deadline, nowTick)

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [launching, setLaunching] = useState(false)
  const pollRef = useRef(0)
  const timerRef = useRef(null)

  const nav = (to) => {
    if (to === -1) {
      if (env.isMiniProgram) return Taro.navigateBack()
      return navigate(-1)
    }
    if (!env.isMiniProgram) return navigate(to)

    // #2411：weapp 的目标可能是 H5 路径（需映射）或已是原生 /pages-weapp/... 路径。
    const target = decodeReturnPath(to)
    const route = (typeof target === 'string' && target.startsWith('/pages'))
      ? { type: 'navigateTo', url: target }
      : toWeappRoute(target)
    if (!route) {
      dialog.alert(t('暂时无法返回，请从首页重新进入'))
      return
    }
    // 目标就是上一个页面（如从支付页进来）→ 直接返回，让其 onShow 重新拉取最新档位。
    try {
      const pages = Taro.getCurrentPages()
      const prev = pages[pages.length - 2]
      const targetPath = route.url.split('?')[0].replace(/^\//, '')
      if (prev && prev.route === targetPath) return Taro.navigateBack()
    } catch { /* getCurrentPages 不可用时走下面兜底 */ }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.redirectTo({ url: route.url })
  }

  const fetchStatus = async () => {
    if (!orderId) return null
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/orders/${orderId}/credit-status`)
      const result = await resp.json()
      if (result.code === 20000) {
        setCredit(result.data)
        setError('')
        return result.data
      }
      setError(resolveErrorMessage(result, t('获取评测状态失败')))
    } catch (e) {
      setError(e.message || t('获取评测状态失败'))
    }
    return null
  }

  // #2407：weapp 定时器在后台会被挂起，返回前台/从其他页面回来时补一次状态刷新，
  // 否则页面可能长时间停在旧状态（首帧由下面的初始化 effect 负责，这里跳过第一次）。
  const didShowRef = useRef(false)
  useDidShow(() => {
    if (!didShowRef.current) { didShowRef.current = true; return }
    if (orderId) fetchStatus()
  })

  const startAssessment = async () => {
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/orders/${orderId}/credit-assessment`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        setCredit(result.data)
        setError('')
        return result.data
      }
      setError(resolveErrorMessage(result, t('发起评测失败')))
    } catch (e) {
      setError(e.message || t('发起评测失败'))
    }
    return null
  }

  // 首次进入：读状态；未开始/过期则发起评测（幂等）。
  useEffect(() => {
    let cancelled = false
    ;(async () => {
      if (!orderId) {
        setError(t('缺少订单参数，请返回重新进入'))
        setLoading(false)
        return
      }
      const current = await fetchStatus()
      if (cancelled) return
      // #2352 F3: credit_state=pending 但服务单未创建（下单即落 pending）时也必须补建，
      // 否则页面停在「评测进行中」、评估金额恒为 0（显示 —）。
      const notStarted = !current || !current.service_order_no
      if (notStarted || current.credit_state === CREDIT_STATE.NONE || current.credit_state === CREDIT_STATE.EXPIRED) {
        await startAssessment()
      }
      if (!cancelled) setLoading(false)
    })()
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orderId])

  // pending 时轮询（退避 1.5s → 8s），终态停止。
  //
  // #2407：原实现用 `setCredit({...next})` 触发本 effect 重跑，但依赖是
  // credit_state/service_order_no —— 值没变时 effect 不会重新执行，于是只轮询
  // 1 次就停住（mock provider 默认 30s 才通过 → 页面永远停在「评测进行中」）。
  // 改为自递归 setTimeout 循环：cancelled 标志负责卸载/终态后停止。
  useEffect(() => {
    if (!orderId || !credit || credit.credit_state !== CREDIT_STATE.PENDING) {
      if (timerRef.current) clearTimeout(timerRef.current)
      return
    }
    let cancelled = false
    let failures = 0
    const tick = async () => {
      const next = await fetchStatus()
      if (cancelled) return
      if (next && next.credit_state === CREDIT_STATE.PENDING) {
        failures = 0
        pollRef.current += 1
        timerRef.current = setTimeout(tick, pollDelay(pollRef.current))
        return
      }
      // 请求失败（next=null）时不要永久停摆：网络抖动/后台恢复都可能失败一次。
      if (!next && failures < 4) {
        failures += 1
        timerRef.current = setTimeout(tick, pollDelay(pollRef.current + failures))
      }
    }
    timerRef.current = setTimeout(tick, pollDelay(pollRef.current))
    return () => {
      cancelled = true
      if (timerRef.current) clearTimeout(timerRef.current)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orderId, credit?.credit_state])

  const gate = creditGate(credit)
  const launch = resolveCreditLaunch({
    mock: !!credit?.mock,
    creditPackage: credit?.credit_package || '',
    isMiniProgram: env.isMiniProgram,
    isWechatBrowser: env.isWechatBrowser,
  })

  const handleLaunch = async () => {
    if (!launch.canLaunch) {
      if (launch.mode === 'unsupported') {
        dialog.alert(t('请在微信小程序中完成信用评测'))
      }
      return
    }
    setLaunching(true)
    try {
      const res = await openCreditAssessment({ package: credit.credit_package })
      if (!res.launched) {
        // H5 真实拉起需 APIv2 密钥 + JS-SDK 签名（D6 灰度）；未接入时明确提示，不伪造成功。
        dialog.alert(t('请在微信小程序中完成信用评测'))
      }
      await fetchStatus()
    } finally {
      setLaunching(false)
    }
  }

  if (loading) {
    return (
      <View className="min-h-screen flex items-center justify-center" style={{ backgroundColor: '#FDFBF7' }}>
        <Text className="text-zinc-400">{t('加载中...')}</Text>
      </View>
    )
  }

  const state = credit?.credit_state || CREDIT_STATE.NONE

  return (
    <View className="min-h-screen" style={{ backgroundColor: '#FDFBF7', paddingBottom: 100 }}>
      {/* ui.md：weapp 页面一律使用原生导航栏，禁止自绘标题栏/返回按钮（H5 无原生栏，保留）。 */}
      {!env.isMiniProgram && (
        <View className="px-4 py-3 flex items-center" style={{ backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
          <Text className="text-xl font-bold text-black" onClick={() => nav(-1)}>❮</Text>
          <Text className="text-lg font-bold flex-1 text-center">{t('微信信用评测')}</Text>
          <View style={{ width: 24 }} />
        </View>
      )}

      {/* #2352 F1: 提交即建单（锁库存 10 分钟），这里必须明确告知截止时间。 */}
      {credit?.payment_deadline && (
        <View className="bg-amber-50 border border-amber-100 mx-4 mt-4 rounded-2xl px-4 py-3">
          <Text className="block text-xs text-amber-700">
            {deadlineLabel || t('订单已创建，请在截止前完成评测与支付，超时订单将自动取消')}
          </Text>
        </View>
      )}

      <View className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
        <View className="flex items-center justify-between">
          <Text className="font-black text-black">{t('评估金额')}</Text>
          <Text className="text-zinc-600">{riskAmountCents > 0 ? `¥${(riskAmountCents / 100).toFixed(2)}` : '—'}</Text>
        </View>
        <Text className="block text-xs text-zinc-400 mt-1">{t('以本次订单信息为准')}</Text>
        <View className="flex items-center justify-between mt-2">
          <Text className="font-black text-black">{t('评测状态')}</Text>
          <Text className="font-medium" style={{ color: state === CREDIT_STATE.APPROVED ? '#16a34a' : state === CREDIT_STATE.REJECTED ? '#dc2626' : '#915F38' }}>
            {t(STATE_TEXT[state] || '尚未评测')}
          </Text>
        </View>
      </View>

      {error ? (
        <View className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <Text className="text-sm text-red-500">{error}</Text>
        </View>
      ) : null}

      {gate.showEntry && (
        <View className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <Text className="text-sm text-zinc-600">{gate.reason}</Text>
          {launch.mode === 'missing-package' && (
            <Text className="block text-xs text-zinc-400 mt-2">{t('微信未返回拉起凭证，请稍后重试')}</Text>
          )}
          {launch.mode === 'unsupported' && (
            <Text className="block text-xs text-zinc-400 mt-2">{t('当前浏览器无法拉起微信支付分，请在小程序中完成')}</Text>
          )}
        </View>
      )}

      {state === CREDIT_STATE.REJECTED && (
        <View className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
          <Text className="text-sm text-red-500">{t('信用评测未通过：本订单暂无法继续支付（全额押金同样需要评测通过）。如需帮助请联系客服。')}</Text>
        </View>
      )}

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4">
        {state === CREDIT_STATE.APPROVED ? (
          <Button
            style={{ width: '100%', margin: 0, padding: 0, height: 48, lineHeight: '48px', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#16a34a', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16 }}
            onClick={() => nav(returnPath || -1)}
          >
            <Text style={{ color: '#fff', fontWeight: 700 }}>{t('返回继续支付')}</Text>
          </Button>
        ) : gate.action === 'blocked' ? (
          <Button
            style={{ width: '100%', margin: 0, padding: 0, height: 48, lineHeight: '48px', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#B98E5F', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16 }}
            onClick={() => nav(-1)}
          >
            <Text style={{ color: '#fff', fontWeight: 700 }}>{t('返回订单')}</Text>
          </Button>
        ) : (
          <Button
            style={{ width: '100%', margin: 0, padding: 0, height: 48, lineHeight: '48px', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: launch.canLaunch ? '#B98E5F' : '#d4d4d8', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16 }}
            disabled={!launch.canLaunch || launching}
            onClick={handleLaunch}
          >
            <Text style={{ color: '#fff', fontWeight: 700 }}>
              {launching ? t('处理中...') : launch.canLaunch ? t('去微信完成评测') : t('评测进行中')}
            </Text>
          </Button>
        )}
      </View>
    </View>
  )
}
