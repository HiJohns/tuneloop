import Taro from '@tarojs/taro'

export const message = {
  success: (msg) => Taro.showToast({ title: msg, icon: 'success', duration: 2000 }),
  error: (msg) => Taro.showToast({ title: msg, icon: 'error', duration: 2000 }),
  warning: (msg) => Taro.showToast({ title: msg, icon: 'none', duration: 2000 }),
  info: (msg) => Taro.showToast({ title: msg, icon: 'none', duration: 2000 }),
}

export const Modal = {
  confirm: ({ title, content, onOk, onCancel }) => {
    Taro.showModal({
      title: title || '确认',
      content,
      success: (res) => res.confirm ? onOk?.() : onCancel?.(),
    })
  },
  info: ({ title, content }) => {
    Taro.showModal({ title, content, showCancel: false })
  },
  warning: ({ title, content }) => {
    Taro.showModal({ title, content, showCancel: false })
  },
  error: ({ title, content }) => {
    Taro.showModal({ title, content, showCancel: false })
  },
}

export const Tag = ({ children, color, className, ...props }) => (
  <span style={{ color }} className={className} {...props}>{children}</span>
)

export const Card = ({ children, className, ...props }) => (
  <div className={className} {...props}>{children}</div>
)

export const Button = ({ children, className, onClick, ...props }) => (
  <button className={className} onClick={onClick} {...props}>{children}</button>
)

export const Switch = ({ checked, onChange, className, ...props }) => (
  <input type="checkbox" checked={checked} onChange={(e) => onChange?.(e.target.checked)} className={className} {...props} />
)

export const Badge = ({ children, count, className, ...props }) => (
  <span className={className} {...props}>
    {count > 0 ? `[${count}]` : ''}{children}
  </span>
)

export const Divider = ({ className, ...props }) => (
  <hr style={{ border: 'none', borderTop: '1px solid #eee' }} className={className} {...props} />
)

export const Image = ({ src, className, ...props }) => (
  <img src={src} className={className} {...props} />
)

export const Steps = ({ children, current, className, ...props }) => (
  <div className={className} {...props}>{children}</div>
)

const Step = ({ title, description, status, ...props }) => (
  <div {...props}>
    <div>{title}</div>
    {description && <div>{description}</div>}
  </div>
)
Steps.Step = Step
Steps.Item = Step
