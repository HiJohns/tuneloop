import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairPaymentComplete from '../../pages/RepairPaymentComplete'
export default function RepairPaymentCompleteWithGuard() {
  useIdentityGuard()
  return <RepairPaymentComplete />
}
