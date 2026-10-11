import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairScan from '../../pages/RepairScan'
export default function RepairScanWithGuard() {
  useIdentityGuard()
  return <RepairScan />
}
