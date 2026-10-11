import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MyRepairs from '../../pages/MyRepairs'
export default function MyRepairsWithGuard() {
  useIdentityGuard()
  return <MyRepairs />
}
