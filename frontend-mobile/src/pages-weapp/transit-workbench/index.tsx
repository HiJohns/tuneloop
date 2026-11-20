import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TransitPage from '../../pages/TransitWorkbench'
import PageShell from '../../components/PageShell'
export default function TransitPageWithGuard() {
  useIdentityGuard()
  return <PageShell><TransitPage /></PageShell>
}
