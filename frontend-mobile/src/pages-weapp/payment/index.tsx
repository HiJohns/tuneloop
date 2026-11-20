import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Payment from './Payment'
import PageShell from '../../components/PageShell'
export default function PaymentWithGuard() {
  useIdentityGuard()
  return <PageShell><Payment /></PageShell>
}
