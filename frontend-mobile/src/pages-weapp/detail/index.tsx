import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Detail from '../Detail'
import PageShell from '../../components/PageShell'
export default function DetailWithGuard() {
  useIdentityGuard()
  return <PageShell><Detail /></PageShell>
}
