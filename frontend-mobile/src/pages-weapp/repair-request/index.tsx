import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairRequestDetail from '../../pages/RepairRequestDetail'
import PageShell from '../../components/PageShell'
export default function RepairRequestDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairRequestDetail /></PageShell>
}
