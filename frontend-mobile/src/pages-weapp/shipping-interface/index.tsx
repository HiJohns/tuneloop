import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ShippingInterface from '../../pages/ShippingInterface'
import PageShell from '../../components/PageShell'
export default function ShippingInterfaceWithGuard() {
  useIdentityGuard()
  return <PageShell><ShippingInterface /></PageShell>
}
