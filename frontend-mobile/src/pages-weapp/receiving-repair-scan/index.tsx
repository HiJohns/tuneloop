import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReceivingRepairScan from '../../pages/ReceivingRepairScan'
import PageShell from '../../components/PageShell'
export default function ReceivingRepairScanWithGuard() {
  useIdentityGuard()
  return <PageShell><ReceivingRepairScan /></PageShell>
}
