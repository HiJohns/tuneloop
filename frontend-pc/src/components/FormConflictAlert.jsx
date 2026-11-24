import { Alert } from 'antd'

// #2480 v2（D7）：业务冲突的**持久**提示共享件（唯一 owner）。
//
// 两页（网点 / 员工）只消费本组件，禁止各自写 Alert markup 或各自维护 code 白名单。
// text 为空时不渲染（避免空 Alert 占位）；清空时机由调用方负责（D8 四处）。
export default function FormConflictAlert({ text, testId = 'form-conflict-alert', style }) {
  if (!text) return null
  return (
    <Alert
      type="error"
      showIcon
      closable={false}
      data-testid={testId}
      style={{ marginBottom: 8, ...style }}
      message={text}
    />
  )
}
