import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import InvoiceConfirm from '../../pages/InvoiceConfirm'
import PageShell from '../../components/PageShell'
export default function InvoiceConfirmWithGuard() {
  useIdentityGuard()
  return <PageShell><InvoiceConfirm /></PageShell>
}
