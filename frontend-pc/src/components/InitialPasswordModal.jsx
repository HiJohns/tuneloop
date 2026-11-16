import React, { useRef } from 'react'
import { Modal, Typography, Button, message } from 'antd'
import { copyText } from '../utils/clipboard'

const { Paragraph, Text } = Typography

// #2370 F1：一次性初始密码展示（1..N 条）。
// D2：不落盘、不写 localStorage；关闭后不可再查看（父组件清理 state）。
// #2387 D1：一次性指「关闭后不可再看」，**不是**「只能复制一次」——关闭前每次点击都复制。
export default function InitialPasswordModal({ open, passwords = [], onClose, title = '初始密码（关闭后不可再看）' }) {
  const passwordRefs = useRef({})

  const copy = async (text, index) => {
    const ok = await copyText(text, { fallbackEl: passwordRefs.current[index] })
    if (ok) {
      message.success('已复制')
    } else {
      message.warning('复制失败，已选中密码文本，请按 Ctrl+C 手动复制')
    }
  }

  return (
    <Modal open={open} title={title} onCancel={onClose} footer={<Button type="primary" onClick={onClose}>我已保存，关闭</Button>} width={560}>
      <Paragraph type="danger">密码仅在本窗口展示：关闭后无法再次查看；关闭前可重复复制。</Paragraph>
      {(passwords || []).map((p, i) => (
        <Paragraph key={`${p.identifier || ''}-${i}`} style={{ marginBottom: 8 }}>
          <Text strong>{p.identifier || p.email || p.username || `用户${i + 1}`}</Text>
          <Text>：</Text>
          <span
            data-copy-fallback={i}
            ref={(el) => { passwordRefs.current[i] = el }}
          >
            <Text code>{p.password}</Text>
          </span>
          <Button type="link" size="small" onClick={() => copy(p.password, i)}>复制</Button>
        </Paragraph>
      ))}
    </Modal>
  )
}
