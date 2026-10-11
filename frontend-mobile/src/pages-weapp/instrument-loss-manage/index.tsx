import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import InstrumentLossManage from '../../pages/InstrumentLossManage'
export default function InstrumentLossManageWithGuard() {
  useIdentityGuard()
  return <InstrumentLossManage />
}
