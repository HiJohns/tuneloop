import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ShippingInterface from '../../pages/ShippingInterface'
export default function ShippingInterfaceWithGuard() {
  useIdentityGuard()
  return <ShippingInterface />
}
