import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffReceiveConfirm from '../../pages/StaffReceiveConfirm'
export default function StaffReceiveConfirmWithGuard() {
  useIdentityGuard()
  return <StaffReceiveConfirm />
}
