import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairServiceCreate from '../../pages/RepairServiceCreate'
import PageShell from '../../components/PageShell'
export default function RepairServiceCreateWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairServiceCreate /></PageShell>
}
