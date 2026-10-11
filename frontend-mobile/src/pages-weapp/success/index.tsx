import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Success from '../Success'
export default function SuccessWithGuard() {
  useIdentityGuard()
  return <Success />
}
