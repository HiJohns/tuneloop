import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MessageDetail from '../../pages/MessageDetail'
import PageShell from '../../components/PageShell'
export default function MessageDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><MessageDetail /></PageShell>
}
