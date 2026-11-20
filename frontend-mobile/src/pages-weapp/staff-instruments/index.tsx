import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffInstruments from '../../pages/StaffInstruments'
import PageShell from '../../components/PageShell'
export default function StaffInstrumentsWithGuard() {
  useIdentityGuard()
  return <PageShell><StaffInstruments /></PageShell>
}
