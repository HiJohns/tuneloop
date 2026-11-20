import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import MembershipCenter from '../../pages/MembershipCenter'
import PageShell from '../../components/PageShell'
export default function MembershipCenterWithGuard() {
  useIdentityGuard()
  return <PageShell><MembershipCenter /></PageShell>
}
