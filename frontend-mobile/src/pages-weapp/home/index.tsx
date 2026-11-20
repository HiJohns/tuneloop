import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Home from '../Home'
import PageShell from '../../components/PageShell'
export default function HomeWithGuard() {
  useIdentityGuard()
  return <PageShell><Home /></PageShell>
}
