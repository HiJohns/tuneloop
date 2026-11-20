import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MyLeases from '../MyLeases'
import PageShell from '../../components/PageShell'
export default function MyLeasesWithGuard() {
  useIdentityGuard()
  return <PageShell><MyLeases /></PageShell>
}
