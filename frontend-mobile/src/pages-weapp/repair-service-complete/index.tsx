import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairServiceComplete from '../../pages/RepairServiceComplete'
import PageShell from '../../components/PageShell'
export default function RepairServiceCompleteWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairServiceComplete /></PageShell>
}
