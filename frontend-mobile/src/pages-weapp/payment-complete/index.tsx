import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import PaymentComplete from '../../pages/PaymentComplete'
import PageShell from '../../components/PageShell'
export default function PaymentCompleteWithGuard() {
  useIdentityGuard()
  return <PageShell><PaymentComplete /></PageShell>
}
