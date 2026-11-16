import React from 'react'
import { Form, Radio, Input, Checkbox } from 'antd'
import { validatePasswordStrength } from '../utils/password'

// #2370 F1：共享「密码设置」字段（自动生成 / 手动 / 强制改密）。
// 受控字段：autoGenerate / password / forcePasswordChange。
// 密码不在前端生成（D1）：自动生成时只提交 auto_generate=true，由后端生成并一次性返回。
export default function PasswordSettingFields({
  autoGenerate = true,
  forcePasswordChange = true,
  onAutoGenerateChange,
  onForcePasswordChangeChange,
  passwordLabel = '初始密码',
}) {
  return (
    <>
      <Form.Item label="密码设置" required>
        <Radio.Group
          value={autoGenerate}
          onChange={(e) => onAutoGenerateChange && onAutoGenerateChange(e.target.value)}
        >
          <Radio value={true}>自动生成</Radio>
          <Radio value={false}>手动设置</Radio>
        </Radio.Group>
      </Form.Item>
      {autoGenerate ? (
        <div style={{ marginTop: -8, marginBottom: 16, fontSize: 12, color: '#999' }}>
          12 位随机密码，创建后仅展示一次
        </div>
      ) : (
        <Form.Item
          name="password"
          label={passwordLabel}
          rules={[
            { required: true, message: '请输入初始密码' },
            {
              validator: (_, value) => {
                if (!value) return Promise.resolve()
                const r = validatePasswordStrength(value)
                return r.ok ? Promise.resolve() : Promise.reject(new Error(r.message))
              },
            },
          ]}
        >
          <Input.Password placeholder="8位+大写+小写+数字" autoComplete="new-password" />
        </Form.Item>
      )}
      <Form.Item>
        <Checkbox
          checked={forcePasswordChange}
          onChange={(e) => onForcePasswordChangeChange && onForcePasswordChangeChange(e.target.checked)}
        >
          首次登录时强制修改密码
        </Checkbox>
      </Form.Item>
    </>
  )
}
