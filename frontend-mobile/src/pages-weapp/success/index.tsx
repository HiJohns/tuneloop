import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Success from '../Success'
import PageShell from '../../components/PageShell'
export default function SuccessWithGuard() {
  useIdentityGuard()
  return <PageShell><Success /></PageShell>
}
