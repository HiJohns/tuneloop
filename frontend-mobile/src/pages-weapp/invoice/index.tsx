import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Invoice from '../../pages/Invoice'
import PageShell from '../../components/PageShell'
export default function InvoiceWithGuard() {
  useIdentityGuard()
  return <PageShell><Invoice /></PageShell>
}
