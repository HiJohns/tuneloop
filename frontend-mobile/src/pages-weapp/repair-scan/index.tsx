import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairScan from '../../pages/RepairScan'
import PageShell from '../../components/PageShell'
export default function RepairScanWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairScan /></PageShell>
}
