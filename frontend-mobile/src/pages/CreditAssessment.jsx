import { useEffect, useRef, useState } from 'react'
import { View, Text, Button } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'

import { apiFetch, resolveErrorMessage } from '../services/api'
import { env, dialog, toWeappRoute, openCreditAssessment } from '../platform'
import { t } from '../i18n'
import {
  CREDIT_STATE,
  creditGate,
  resolveCreditLaunch,
  pollDelay,
} from '../utils/creditFlow'

// #2327 S4 / D8: 通用微信支付分信用评测页（H5 + weapp 共用）。
// 入参（一律 query）：order_id、tier、risk_amount_cents、return_path。
// 本页只做「展示 + 拉起 + 轮询」，不承载任何业务规则（规则在 services/creditFlow.mjs）。

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
  const tier = params.tier || ''
  const riskAmountCents = Number(params.risk_amount_cents || 0)
  const returnPath = params.return_path || (orderId ? `/payment?type=rent&id=${orderId}` : '')

  const [credit, setCredit] = useState(null)
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
    const route = toWeappRoute(to)
    if (!route) {
      dialog.alert(t('该功能请在微信小程序中完成'))
      return
    }
    return Taro.navigateTo({ url: route.url })
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
      if (!current || current.credit_state === CREDIT_STATE.NONE || current.credit_state === CREDIT_STATE.EXPIRED) {
        await startAssessment()
      }
      if (!cancelled) setLoading(false)
    })()
    return () => { cancelled = true }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [orderId])

  // pending 时轮询（退避 1.5s → 8s），终态停止。
  useEffect(() => {
    if (!credit || credit.credit_state !== CREDIT_STATE.PENDING) {
      if (timerRef.current) clearTimeout(timerRef.current)
      return
    }
    timerRef.current = setTimeout(async () => {
      pollRef.current += 1
      const next = await fetchStatus()
      if (next && next.credit_state === CREDIT_STATE.PENDING) {
        // 触发下一次 effect（state 未变时手动续排）。
        timerRef.current = setTimeout(() => setCredit({ ...next }), pollDelay(pollRef.current))
      }
    }, pollDelay(pollRef.current))
    return () => { if (timerRef.current) clearTimeout(timerRef.current) }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [credit?.credit_state, credit?.service_order_no])

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
      <View className="px-4 py-3 flex items-center" style={{ backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
        <Text className="text-xl font-bold text-black" onClick={() => nav(-1)}>❮</Text>
        <Text className="text-lg font-bold flex-1 text-center">{t('微信信用评测')}</Text>
        <View style={{ width: 24 }} />
      </View>

      <View className="bg-white mx-4 mt-4 rounded-2xl p-4 shadow-sm">
        <View className="flex items-center justify-between">
          <Text className="font-black text-black">{t('押金档位')}</Text>
          <Text className="text-zinc-600">{tier || '—'}</Text>
        </View>
        <View className="flex items-center justify-between mt-2">
          <Text className="font-black text-black">{t('担保金额')}</Text>
          <Text className="text-zinc-600">¥{(riskAmountCents / 100).toFixed(2)}</Text>
        </View>
        <View className="flex items-center justify-between mt-2">
          <Text className="font-black text-black">{t('评测状态')}</Text>
          <Text className="font-medium" style={{ color: state === CREDIT_STATE.APPROVED ? '#16a34a' : state === CREDIT_STATE.REJECTED ? '#dc2626' : '#915F38' }}>
            {t(STATE_TEXT[state] || '尚未评测')}
          </Text>
        </View>
        {credit?.mock && (
          <View className="bg-amber-50 border border-amber-100 rounded-lg px-3 py-2 mt-3">
            <Text className="text-xs text-amber-700">{t('模拟评估（开发/测试环境，不会真实拉起微信）')}</Text>
          </View>
        )}
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
          <Text className="text-sm text-red-500">{t('信用评测未通过：请返回商品页改用全额押金重新下单，或更换乐器。')}</Text>
        </View>
      )}

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4">
        {state === CREDIT_STATE.APPROVED ? (
          <Button
            style={{ width: '100%', margin: 0, height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#16a34a', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16 }}
            onClick={() => nav(returnPath || -1)}
          >
            <Text style={{ color: '#fff', fontWeight: 700 }}>{t('返回继续支付')}</Text>
          </Button>
        ) : gate.action === 'fallback_full' ? (
          <Button
            style={{ width: '100%', margin: 0, height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#B98E5F', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16 }}
            onClick={() => nav(-1)}
          >
            <Text style={{ color: '#fff', fontWeight: 700 }}>{t('返回更换押金档位')}</Text>
          </Button>
        ) : (
          <Button
            style={{ width: '100%', margin: 0, height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: launch.canLaunch ? '#B98E5F' : '#d4d4d8', color: '#fff', fontWeight: 700, fontSize: 16, borderRadius: 16 }}
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
