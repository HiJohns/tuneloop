import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Checkout from '../Checkout'
import PageShell from '../../components/PageShell'
export default function CheckoutWithGuard() {
  useIdentityGuard()
  return <PageShell><Checkout /></PageShell>
}
