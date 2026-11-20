import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Bind from './Bind'
import PageShell from '../../components/PageShell'
export default function BindWithGuard() {
  useIdentityGuard()
  return <PageShell><Bind /></PageShell>
}
