import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairServiceDetail from '../../pages/RepairServiceDetail'
export default function RepairServiceDetailWithGuard() {
  useIdentityGuard()
  return <RepairServiceDetail />
}
