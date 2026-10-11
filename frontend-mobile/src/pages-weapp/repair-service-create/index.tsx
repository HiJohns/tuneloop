import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairServiceCreate from '../../pages/RepairServiceCreate'
export default function RepairServiceCreateWithGuard() {
  useIdentityGuard()
  return <RepairServiceCreate />
}
