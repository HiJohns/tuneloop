import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairServiceDetail from '../../pages/RepairServiceDetail'
import PageShell from '../../components/PageShell'
export default function RepairServiceDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairServiceDetail /></PageShell>
}
