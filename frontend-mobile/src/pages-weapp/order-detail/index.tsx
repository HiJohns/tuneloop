import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import OrderDetail from './OrderDetail'
import PageShell from '../../components/PageShell'
export default function OrderDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><OrderDetail /></PageShell>
}
