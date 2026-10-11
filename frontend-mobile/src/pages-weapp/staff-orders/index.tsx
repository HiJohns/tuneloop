import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffOrders from '../../pages/StaffOrders'
export default function StaffOrdersWithGuard() {
  useIdentityGuard()
  return <StaffOrders />
}
