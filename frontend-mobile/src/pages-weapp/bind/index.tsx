import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Bind from './Bind'
export default function BindWithGuard() {
  useIdentityGuard()
  return <Bind />
}
