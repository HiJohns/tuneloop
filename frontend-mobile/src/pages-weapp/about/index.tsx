import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import About from '../../pages/About'
import PageShell from '../../components/PageShell'
export default function AboutWithGuard() {
  useIdentityGuard()
  return <PageShell><About /></PageShell>
}
