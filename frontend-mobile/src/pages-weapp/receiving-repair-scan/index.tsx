import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReceivingRepairScan from '../../pages/ReceivingRepairScan'
export default function ReceivingRepairScanWithGuard() {
  useIdentityGuard()
  return <ReceivingRepairScan />
}
