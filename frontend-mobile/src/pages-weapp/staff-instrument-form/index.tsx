import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffInstrumentForm from '../../pages/StaffInstrumentForm'
export default function StaffInstrumentFormWithGuard() {
  useIdentityGuard()
  return <StaffInstrumentForm />
}
