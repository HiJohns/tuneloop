import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairServiceComplete from '../../pages/RepairServiceComplete'
export default function RepairServiceCompleteWithGuard() {
  useIdentityGuard()
  return <RepairServiceComplete />
}
