import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffOrders from '../../pages/StaffOrders'
import PageShell from '../../components/PageShell'
export default function StaffOrdersWithGuard() {
  useIdentityGuard()
  return <PageShell><StaffOrders /></PageShell>
}
