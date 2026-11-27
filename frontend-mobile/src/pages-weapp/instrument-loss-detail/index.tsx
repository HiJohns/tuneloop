import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import InstrumentLossDetail from '../../pages/InstrumentLossDetail'
import PageShell from '../../components/PageShell'
export default function InstrumentLossDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><InstrumentLossDetail /></PageShell>
}
