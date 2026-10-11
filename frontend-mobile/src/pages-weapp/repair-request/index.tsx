import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairRequestDetail from '../../pages/RepairRequestDetail'
export default function RepairRequestDetailWithGuard() {
  useIdentityGuard()
  return <RepairRequestDetail />
}
