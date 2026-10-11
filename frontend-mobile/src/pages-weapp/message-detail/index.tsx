import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MessageDetail from '../../pages/MessageDetail'
export default function MessageDetailWithGuard() {
  useIdentityGuard()
  return <MessageDetail />
}
