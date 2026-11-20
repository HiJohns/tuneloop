import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TransitPage from '../../pages/TransitReceive'
import PageShell from '../../components/PageShell'
export default function TransitPageWithGuard() {
  useIdentityGuard()
  return <PageShell><TransitPage /></PageShell>
}
