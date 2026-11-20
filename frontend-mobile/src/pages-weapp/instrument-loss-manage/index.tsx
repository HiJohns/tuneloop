import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import InstrumentLossManage from '../../pages/InstrumentLossManage'
import PageShell from '../../components/PageShell'
export default function InstrumentLossManageWithGuard() {
  useIdentityGuard()
  return <PageShell><InstrumentLossManage /></PageShell>
}
