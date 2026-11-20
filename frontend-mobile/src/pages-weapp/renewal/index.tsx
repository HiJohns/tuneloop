import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Renewal from './Renewal'
import PageShell from '../../components/PageShell'
export default function RenewalWithGuard() {
  useIdentityGuard()
  return <PageShell><Renewal /></PageShell>
}
