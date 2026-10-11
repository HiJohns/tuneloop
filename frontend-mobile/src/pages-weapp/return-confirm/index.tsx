import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReturnConfirm from '../../pages/ReturnConfirm'
export default function ReturnConfirmWithGuard() {
  useIdentityGuard()
  return <ReturnConfirm />
}
