import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffInstruments from '../../pages/StaffInstruments'
export default function StaffInstrumentsWithGuard() {
  useIdentityGuard()
  return <StaffInstruments />
}
