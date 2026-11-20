import { useEffect, useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, Input, ScrollView } from '@tarojs/components'
import { t } from '../i18n'
import { pickHostRoute } from '../utils/dialogHostQueue.mjs'

// #2415 S1：跨端自绘对话框（白底卡片 + 主色按钮 + 队列）。
// 订阅式 API：showDialog(...) → Promise；DialogHost 在 app 入口挂载一次。
// 未挂载时 platform 回退原生（Taro.showModal / window.alert）。

const queue = []
const listeners = new Map() // route -> sync（#2437 页面级宿主）
let seq = 0

function topRoute() {
  try {
    const pages = Taro.getCurrentPages ? Taro.getCurrentPages() : []
    return pages && pages.length ? (pages[pages.length - 1].route || '') : ''
  } catch {
    return ''
  }
}

function notify() {
  for (const fn of listeners.values()) {
    try { fn() } catch { /* host sync 不阻断队列 */ }
  }
}

export function notifyDialogHosts() { notify() }

export function __resetDialogQueueForTest() {
  for (const item of queue) if (item.timer) clearTimeout(item.timer)
  queue.length = 0
  listeners.clear()
}

export function isDialogHostMounted() {
  if (listeners.size === 0) return false
  const top = topRoute()
  if (!top) return true
  return listeners.has(top)
}

export function showDialog(options = {}) {
  return new Promise((resolve) => {
    const item = { id: ++seq, ...options, resolve }
    const timeoutMs = Number.isFinite(options.timeoutMs)
      ? options.timeoutMs
      : 1500
    item.timer = setTimeout(() => {
      const idx = queue.indexOf(item)
      if (idx < 0) return // 已被宿主消费
      queue.splice(idx, 1)
      const fallback = typeof item.onTimeout === 'function' ? item.onTimeout(item) : undefined
      Promise.resolve(fallback).then(item.resolve)
      notify()
    }, timeoutMs)
    queue.push(item)
    notify()
  })
}

export function DialogHost() {
  const [current, setCurrent] = useState(null)
  const [value, setValue] = useState('')
  useEffect(() => {
    const route = Taro.getCurrentInstance?.().router?.route || ''
    const sync = () => {
      const top = topRoute()
      const mine = pickHostRoute([...listeners.keys()], top)
      const next = (!top || top === route || mine === route) ? (queue[0] || null) : null
      setCurrent(next)
      setValue(next?.defaultValue || '')
    }
    listeners.set(route, sync)
    sync()
    return () => {
      if (listeners.get(route) === sync) listeners.delete(route)
    }
  }, [])

  useEffect(() => {
    if (!current || current.type !== 'toast') return undefined
    const timer = setTimeout(() => {
      const item = queue.shift()
      if (item) {
        if (item.timer) clearTimeout(item.timer)
        item.resolve(undefined)
      }
      notify()
    }, current.duration || 2000)
    return () => clearTimeout(timer)
  }, [current])

  if (!current) return null

  const close = (result) => {
    const item = queue.shift()
    if (item) {
      if (item.timer) clearTimeout(item.timer)
      item.resolve(result)
    }
    notify()
  }

  const isToast = current.type === 'toast'
  const isConfirm = current.type === 'confirm'
  const isEditable = current.type === 'editable' || current.editable === true

  return (
    <View
      data-dialog-host="true"
      style={{
        position: 'fixed',
        top: 0,
        left: 0,
        right: 0,
        bottom: 0,
        zIndex: 9999,
        backgroundColor: isToast ? 'transparent' : 'rgba(0,0,0,0.45)',
        display: 'flex',
        alignItems: 'center',
        justifyContent: 'center',
        padding: 24,
        boxSizing: 'border-box',
        pointerEvents: isToast ? 'none' : 'auto',
      }}
    >
      <View
        data-dialog-toast={isToast ? 'true' : undefined}
        style={
          isToast
            ? {
                maxWidth: 320,
                backgroundColor: 'rgba(0,0,0,0.78)',
                borderRadius: 12,
                paddingTop: 12,
                paddingBottom: 12,
                paddingLeft: 20,
                paddingRight: 20,
                boxSizing: 'border-box',
              }
            : {
                width: '100%',
                maxWidth: 320,
                backgroundColor: '#ffffff',
                borderRadius: 16,
                padding: 24,
                boxSizing: 'border-box',
                borderWidth: 1,
                borderStyle: 'solid',
                borderColor: '#f4f4f5',
              }
        }
      >
        {current.title && !isToast ? (
          <Text
            style={{
              display: 'block',
              fontSize: 18,
              fontWeight: '700',
              color: '#18181b',
              marginBottom: 12,
              lineHeight: '24px',
            }}
          >
            {current.title}
          </Text>
        ) : null}

        <ScrollView
          scrollY
          data-dialog-scroll="true"
          style={{ maxHeight: 260 }}
        >
          <Text
            style={{
              display: 'block',
              fontSize: 15,
              color: isToast ? '#ffffff' : '#3f3f46',
              lineHeight: '22px',
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
            }}
          >
            {current.message || ''}
          </Text>
        </ScrollView>

        {isEditable && !isToast ? (
          <Input
            value={value}
            placeholder={current.placeholder || ''}
            onInput={(e) => setValue(e?.detail?.value ?? e?.target?.value ?? '')}
            style={{
              marginTop: 12,
              height: 44,
              paddingLeft: 12,
              paddingRight: 12,
              borderWidth: 1,
              borderStyle: 'solid',
              borderColor: '#e4e4e7',
              borderRadius: 10,
              fontSize: 15,
              color: '#18181b',
              backgroundColor: '#ffffff',
              boxSizing: 'border-box',
            }}
          />
        ) : null}

        {!isToast ? (
          <View
            style={{
              display: 'flex',
              flexDirection: isConfirm || isEditable ? 'row' : 'column',
              justifyContent: 'flex-end',
              gap: 10,
              marginTop: 20,
            }}
          >
            {isConfirm || isEditable ? (
              <Button
                onClick={() => close(isEditable ? null : false)}
                style={{
                  flex: 1,
                  margin: 0,
                  height: 48,
                  lineHeight: '48px',
                  borderRadius: 24,
                  borderWidth: 1,
                  borderStyle: 'solid',
                  borderColor: '#e4e4e7',
                  backgroundColor: '#ffffff',
                  color: '#3f3f46',
                  fontSize: 15,
                  fontWeight: '600',
                }}
              >
                {current.cancelText || t('取消')}
              </Button>
            ) : null}
            <Button
              onClick={() => close(isEditable ? value : isConfirm ? true : undefined)}
              style={{
                flex: 1,
                margin: 0,
                height: 48,
                lineHeight: '48px',
                borderRadius: 24,
                borderWidth: 0,
                backgroundColor: '#915F38',
                color: '#ffffff',
                fontSize: 15,
                fontWeight: '700',
              }}
            >
              {current.confirmText || t('确定')}
            </Button>
          </View>
        ) : null}
      </View>
    </View>
  )
}

export default DialogHost
