import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MyRepairs from '../../pages/MyRepairs'
import PageShell from '../../components/PageShell'
export default function MyRepairsWithGuard() {
  useIdentityGuard()
  return <PageShell><MyRepairs /></PageShell>
}
