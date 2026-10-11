import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TechDetail from '../../pages/TechDetail'
export default function TechDetailWithGuard() {
  useIdentityGuard()
  return <TechDetail />
}
