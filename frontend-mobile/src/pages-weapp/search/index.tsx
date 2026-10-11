import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Search from './Search'
export default function SearchWithGuard() {
  useIdentityGuard()
  return <Search />
}
