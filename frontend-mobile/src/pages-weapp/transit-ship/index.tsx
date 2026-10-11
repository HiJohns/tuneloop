import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TransitPage from '../../pages/TransitShip'
export default function TransitPageWithGuard() {
  useIdentityGuard()
  return <TransitPage />
}
