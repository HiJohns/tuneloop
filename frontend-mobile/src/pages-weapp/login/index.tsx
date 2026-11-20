import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Login from './Login'
import PageShell from '../../components/PageShell'
export default function LoginWithGuard() {
  useIdentityGuard()
  return <PageShell><Login /></PageShell>
}
