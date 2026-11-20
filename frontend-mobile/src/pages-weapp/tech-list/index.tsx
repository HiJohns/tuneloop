import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TechList from '../../pages/TechList'
import PageShell from '../../components/PageShell'
export default function TechListWithGuard() {
  useIdentityGuard()
  return <PageShell><TechList /></PageShell>
}
