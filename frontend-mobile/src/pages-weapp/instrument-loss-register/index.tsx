import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import InstrumentLossRegister from '../../pages/InstrumentLossRegister'
import PageShell from '../../components/PageShell'
export default function InstrumentLossRegisterWithGuard() {
  useIdentityGuard()
  return <PageShell><InstrumentLossRegister /></PageShell>
}
