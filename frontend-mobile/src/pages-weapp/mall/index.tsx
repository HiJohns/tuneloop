import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Mall from '../../pages/Mall'
export default function MallWithGuard() {
  useIdentityGuard()
  return <Mall />
}
