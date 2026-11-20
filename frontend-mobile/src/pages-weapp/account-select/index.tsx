import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import AccountSelect from './AccountSelect'
import PageShell from '../../components/PageShell'
export default function AccountSelectWithGuard() {
  useIdentityGuard()
  return <PageShell><AccountSelect /></PageShell>
}
