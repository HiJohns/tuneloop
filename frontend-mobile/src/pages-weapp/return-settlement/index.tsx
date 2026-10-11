import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReturnSettlement from '../../pages/ReturnSettlement'
export default function ReturnSettlementWithGuard() {
  useIdentityGuard()
  return <ReturnSettlement />
}
