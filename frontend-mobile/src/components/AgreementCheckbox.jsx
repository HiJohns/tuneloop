// #2244：协议勾选（显性阅读同意流，方案 1：复用 #2258 合并阅读页）
// 行为：未选中点击 → 进入合并阅读页的「同意流」；已选中点击 → 直接取消。
// 结果回写：storage 标记 + eventBus（H5 SPA/整页返回）+ useDidShow（weapp 返回栈）。

import Taro from '@tarojs/taro'
import { useEffect, useCallback } from 'react'
import { Text, View } from '@tarojs/components'
import { env, eventBus, navigation, storage } from '../platform'

export const AGREEMENT_CONSENT_RESULT_KEY = 'agreement_consent_result'
export const AGREEMENT_CONSENT_EVENT = 'agreementConsentChanged'

const CONSENT_URL_H5 = '/content?key=related_agreements&consent=1'
const CONSENT_URL_WEAPP = '/pages-weapp/content/index?key=related_agreements&consent=1'

export default function AgreementCheckbox({ agreed, onChange, style }) {
  const applyResult = useCallback(() => {
    const result = storage.getItem(AGREEMENT_CONSENT_RESULT_KEY)
    if (!result) return
    storage.removeItem(AGREEMENT_CONSENT_RESULT_KEY)
    onChange(result === 'agree')
  }, [onChange])

  // H5：整页返回（挂载时读取）/ SPA 返回（事件）
  useEffect(() => { applyResult() }, [applyResult])
  useEffect(() => {
    eventBus.on(AGREEMENT_CONSENT_EVENT, applyResult)
    return () => eventBus.off(AGREEMENT_CONSENT_EVENT, applyResult)
  }, [applyResult])
  // weapp：navigateBack 不重新挂载页面 → 页面显示时读取
  Taro.useDidShow(() => {
    if (env.isMiniProgram) applyResult()
  })

  const openConsent = () => {
    if (env.isMiniProgram) {
      Taro.navigateTo({ url: CONSENT_URL_WEAPP })
    } else {
      navigation.redirect(CONSENT_URL_H5)
    }
  }
  const openPlainRead = () => {
    if (env.isMiniProgram) {
      Taro.navigateTo({ url: '/pages-weapp/content/index?key=related_agreements' })
    } else {
      navigation.redirect('/content?key=related_agreements')
    }
  }
  const toggle = () => {
    if (agreed) {
      onChange(false)
      return
    }
    openConsent()
  }

  return (
    <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', flexWrap: 'wrap', ...style }}>
      <View
        onClick={toggle}
        style={{ width: 18, height: 18, borderRadius: 4, border: agreed ? 'none' : '1px solid #d4d4d8', backgroundColor: agreed ? '#915F38' : '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0 }}
      >
        {agreed ? <Text style={{ color: '#fff', fontSize: 12, lineHeight: '18px' }}>✓</Text> : null}
      </View>
      <Text style={{ fontSize: 12, color: '#a1a1aa', marginLeft: 8 }}>我已阅读并同意所有「</Text>
      <Text style={{ fontSize: 12, color: '#915F38' }} onClick={openPlainRead}>相关协议</Text>
      <Text style={{ fontSize: 12, color: '#a1a1aa' }}>」</Text>
    </View>
  )
}
