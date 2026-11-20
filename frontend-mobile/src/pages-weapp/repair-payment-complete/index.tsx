import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairPaymentComplete from '../../pages/RepairPaymentComplete'
import PageShell from '../../components/PageShell'
export default function RepairPaymentCompleteWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairPaymentComplete /></PageShell>
}
