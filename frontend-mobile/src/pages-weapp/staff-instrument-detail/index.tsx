import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffInstrumentDetail from '../../pages/StaffInstrumentDetail'
import PageShell from '../../components/PageShell'
export default function StaffInstrumentDetailWithGuard() {
  useIdentityGuard()
  return <PageShell><StaffInstrumentDetail /></PageShell>
}
