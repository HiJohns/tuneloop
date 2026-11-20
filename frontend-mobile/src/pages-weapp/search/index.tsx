import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Search from './Search'
import PageShell from '../../components/PageShell'
export default function SearchWithGuard() {
  useIdentityGuard()
  return <PageShell><Search /></PageShell>
}
