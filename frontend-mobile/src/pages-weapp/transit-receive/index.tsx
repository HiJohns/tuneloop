import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TransitPage from '../../pages/TransitReceive'
export default function TransitPageWithGuard() {
  useIdentityGuard()
  return <TransitPage />
}
