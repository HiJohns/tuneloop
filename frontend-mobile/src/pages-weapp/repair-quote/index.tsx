import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairQuote from '../../pages/RepairQuote'
import PageShell from '../../components/PageShell'
export default function RepairQuoteWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairQuote /></PageShell>
}
