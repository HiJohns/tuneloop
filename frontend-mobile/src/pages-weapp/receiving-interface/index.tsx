import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReceivingInterface from '../../pages/ReceivingInterface'
export default function ReceivingInterfaceWithGuard() {
  useIdentityGuard()
  return <ReceivingInterface />
}
