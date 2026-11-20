import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReceivingInterface from '../../pages/ReceivingInterface'
import PageShell from '../../components/PageShell'
export default function ReceivingInterfaceWithGuard() {
  useIdentityGuard()
  return <PageShell><ReceivingInterface /></PageShell>
}
