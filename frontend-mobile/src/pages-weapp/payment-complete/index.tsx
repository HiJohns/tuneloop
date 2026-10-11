import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import PaymentComplete from '../../pages/PaymentComplete'
export default function PaymentCompleteWithGuard() {
  useIdentityGuard()
  return <PaymentComplete />
}
