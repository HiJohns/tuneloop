import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import AccountSelect from './AccountSelect'
export default function AccountSelectWithGuard() {
  useIdentityGuard()
  return <AccountSelect />
}
