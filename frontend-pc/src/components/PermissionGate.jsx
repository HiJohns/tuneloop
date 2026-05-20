import { usePermission } from '../hooks/usePermission'

export default function PermissionGate({ code, sysPermBit, children, fallback = null }) {
  const { hasCusPerm, hasSysPerm } = usePermission()
  const ok = (code && hasCusPerm(code)) || (sysPermBit != null && hasSysPerm(sysPermBit))
  return ok ? children : fallback
}
