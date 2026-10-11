import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReceiveConfirm from '../../pages/ReceiveConfirm'
export default function ReceiveConfirmWithGuard() {
  useIdentityGuard()
  return <ReceiveConfirm />
}
