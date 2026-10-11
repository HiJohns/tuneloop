import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TransitPage from '../../pages/TransitWorkbench'
export default function TransitPageWithGuard() {
  useIdentityGuard()
  return <TransitPage />
}
