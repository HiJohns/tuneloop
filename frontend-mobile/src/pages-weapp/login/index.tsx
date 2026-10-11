import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Login from './Login'
export default function LoginWithGuard() {
  useIdentityGuard()
  return <Login />
}
