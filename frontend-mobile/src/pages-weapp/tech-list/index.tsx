import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TechList from '../../pages/TechList'
export default function TechListWithGuard() {
  useIdentityGuard()
  return <TechList />
}
