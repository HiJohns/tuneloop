import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Home from '../Home'
export default function HomeWithGuard() {
  useIdentityGuard()
  return <Home />
}
