// #2432：weapp 按钮高度必须内联（Taro Button 原生 padding/margin/line-height）。
const BASE = {
  margin: 0,
  display: 'flex',
  alignItems: 'center',
  justifyContent: 'center',
  boxSizing: 'border-box',
  border: 'none',
}

export function secondaryButtonStyle(theme = 'default') {
  const colors = theme === 'danger'
    ? { backgroundColor: '#fef2f2', color: '#ef4444' }
    : { backgroundColor: '#f4f4f5', color: '#52525b' }
  return {
    ...BASE,
    ...colors,
    height: 36,
    lineHeight: '36px',
    padding: '0 12px',
    borderRadius: 8,
    fontSize: 12,
    fontWeight: '700',
  }
}

export function primaryButtonStyle() {
  return {
    ...BASE,
    height: 48,
    lineHeight: '48px',
    padding: '0 16px',
    borderRadius: 12,
    backgroundColor: '#000000',
    color: '#ffffff',
    fontSize: 14,
    fontWeight: '700',
  }
}
