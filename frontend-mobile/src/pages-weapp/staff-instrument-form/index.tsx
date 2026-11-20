import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffInstrumentForm from '../../pages/StaffInstrumentForm'
import PageShell from '../../components/PageShell'
export default function StaffInstrumentFormWithGuard() {
  useIdentityGuard()
  return <PageShell><StaffInstrumentForm /></PageShell>
}
