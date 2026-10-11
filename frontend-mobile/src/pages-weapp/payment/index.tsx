import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Payment from './Payment'
export default function PaymentWithGuard() {
  useIdentityGuard()
  return <Payment />
}
