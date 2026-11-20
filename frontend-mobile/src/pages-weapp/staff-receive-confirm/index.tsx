import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffReceiveConfirm from '../../pages/StaffReceiveConfirm'
import PageShell from '../../components/PageShell'
export default function StaffReceiveConfirmWithGuard() {
  useIdentityGuard()
  return <PageShell><StaffReceiveConfirm /></PageShell>
}
