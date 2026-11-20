import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReturnSettlement from '../../pages/ReturnSettlement'
import PageShell from '../../components/PageShell'
export default function ReturnSettlementWithGuard() {
  useIdentityGuard()
  return <PageShell><ReturnSettlement /></PageShell>
}
