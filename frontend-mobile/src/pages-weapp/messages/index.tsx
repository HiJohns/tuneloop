import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Messages from '../../pages/Messages'
export default function MessagesWithGuard() {
  useIdentityGuard()
  return <Messages />
}
