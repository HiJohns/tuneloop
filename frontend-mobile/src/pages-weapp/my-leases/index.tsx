import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MyLeases from '../MyLeases'
export default function MyLeasesWithGuard() {
  useIdentityGuard()
  return <MyLeases />
}
