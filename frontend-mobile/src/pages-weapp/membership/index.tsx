import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MembershipCenter from '../../pages/MembershipCenter'
export default function MembershipCenterWithGuard() {
  useIdentityGuard()
  return <MembershipCenter />
}
