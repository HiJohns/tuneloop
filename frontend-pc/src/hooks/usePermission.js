export function usePermission() {
  const sysPerm = parseInt(localStorage.getItem('user_sys_perm') || '0')
  const cusPerm = parseInt(localStorage.getItem('user_cus_perm') || '0')
  const mapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')

  function hasCusPerm(code) {
    const bit = mapping[code]
    return bit !== undefined && bit >= 0 && (cusPerm & (1 << bit)) !== 0
  }

  function hasSysPerm(bit) {
    return (sysPerm & (1 << bit)) !== 0
  }

  return { hasCusPerm, hasSysPerm }
}
