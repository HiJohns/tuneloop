import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Renewal from './Renewal'
export default function RenewalWithGuard() {
  useIdentityGuard()
  return <Renewal />
}
