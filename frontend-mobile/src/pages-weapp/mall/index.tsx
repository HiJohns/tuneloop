import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Mall from '../../pages/Mall'
import PageShell from '../../components/PageShell'
export default function MallWithGuard() {
  useIdentityGuard()
  return <PageShell><Mall /></PageShell>
}
