import { useState, useEffect } from 'react'
import { Card, Button, Modal, message, Tag, QRCode } from 'antd'
import { WechatOutlined, LinkOutlined } from '@ant-design/icons'
import api from '../services/api'

export default function WechatBinding() {
  const [showQR, setShowQR] = useState(false)
  const [qrToken, setQrToken] = useState('')
  const [bound, setBound] = useState(false)
  const [loading, setLoading] = useState(true)
  const [polling, setPolling] = useState(false)

  useEffect(() => {
    api.get('/users/me').then(res => {
      if (res.data?.wx_openid) setBound(true)
    }).finally(() => setLoading(false))
  }, [])

  const handleBind = async () => {
    try {
      const res = await api.post('/users/me/wechat-bind')
      if (res.code === 20000) {
        setQrToken(res.data.token)
        setShowQR(true)
        setPolling(true)
      }
    } catch {
      message.error('生成二维码失败')
    }
  }

  // Poll binding status
  useEffect(() => {
    if (!polling || !qrToken) return
    const timer = setInterval(async () => {
      try {
        const res = await api.get(`/users/me/wechat-bind/${qrToken}`)
        if (res.data?.status === 'bound') {
          setBound(true)
          setShowQR(false)
          setPolling(false)
          clearInterval(timer)
          message.success('微信绑定成功')
        } else if (res.data?.status === 'expired') {
          setPolling(false)
          clearInterval(timer)
        }
      } catch {}
    }, 2000)
    return () => clearInterval(timer)
  }, [polling, qrToken])

  // Auto-close expired QR after 5 minutes
  useEffect(() => {
    if (!showQR) return
    const timer = setTimeout(() => {
      setShowQR(false)
      setPolling(false)
    }, 300000)
    return () => clearTimeout(timer)
  }, [showQR])

  const handleUnbind = () => {
    Modal.confirm({
      title: '确认解绑',
      content: '解绑后，PC 端将无法使用微信快捷登录。',
      onOk: async () => {
        try {
          await api.post('/users/me/wechat-unbind')
          setBound(false)
          message.success('已解绑')
        } catch {
          message.error('解绑失败')
        }
      },
    })
  }

  const qrValue = `https://wx.cadenzayueqi.com/bind?token=${qrToken}`

  return (
    <Card title={<span><WechatOutlined /> 微信绑定</span>} style={{ marginBottom: 16 }}>
      {loading ? null : bound ? (
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <span><Tag color="green">已绑定</Tag>微信号已关联</span>
          <Button danger onClick={handleUnbind}>解绑</Button>
        </div>
      ) : (
        <div>
          <span style={{ color: '#999', marginRight: 16 }}>未绑定微信</span>
          <Button type="primary" icon={<LinkOutlined />} onClick={handleBind}>绑定微信</Button>
        </div>
      )}

      <Modal
        title="微信绑定"
        open={showQR}
        onCancel={() => { setShowQR(false); setPolling(false) }}
        footer={null}
        width={360}
      >
        <div style={{ textAlign: 'center', padding: 16 }}>
          {qrToken && (
            <QRCode value={qrValue} size={256} style={{ margin: '0 auto 16px' }} />
          )}
          <p style={{ color: '#999', fontSize: 13 }}>
            打开微信「扫一扫」扫描二维码完成绑定
          </p>
          {(polling && showQR) && <p style={{ color: '#ff4d4f', fontSize: 12 }}>二维码 5 分钟有效，扫码后自动绑定</p>}
        </div>
      </Modal>
    </Card>
  )
}
