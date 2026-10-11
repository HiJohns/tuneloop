import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairQuote from '../../pages/RepairQuote'
export default function RepairQuoteWithGuard() {
  useIdentityGuard()
  return <RepairQuote />
}
