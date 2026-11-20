import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Messages from '../../pages/Messages'
import PageShell from '../../components/PageShell'
export default function MessagesWithGuard() {
  useIdentityGuard()
  return <PageShell><Messages /></PageShell>
}
