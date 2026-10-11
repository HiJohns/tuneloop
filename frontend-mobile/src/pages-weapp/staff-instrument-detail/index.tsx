import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import StaffInstrumentDetail from '../../pages/StaffInstrumentDetail'
export default function StaffInstrumentDetailWithGuard() {
  useIdentityGuard()
  return <StaffInstrumentDetail />
}
