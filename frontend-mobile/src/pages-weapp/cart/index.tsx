import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Cart from '../../pages/Cart'
import PageShell from '../../components/PageShell'
export default function CartWithGuard() {
  useIdentityGuard()
  return <PageShell><Cart /></PageShell>
}
