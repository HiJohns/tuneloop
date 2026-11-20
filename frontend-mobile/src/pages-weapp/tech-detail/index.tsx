import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TechDetail from '../../pages/TechDetail'
import PageShell from '../../components/PageShell'
export default function TechDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><TechDetail /></PageShell>
}
