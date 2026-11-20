import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReceiveConfirm from '../../pages/ReceiveConfirm'
import PageShell from '../../components/PageShell'
export default function ReceiveConfirmWithGuard() {
  useIdentityGuard()
  return <PageShell><ReceiveConfirm /></PageShell>
}
