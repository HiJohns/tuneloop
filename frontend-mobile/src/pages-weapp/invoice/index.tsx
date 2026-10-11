import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Invoice from '../../pages/Invoice'
export default function InvoiceWithGuard() {
  useIdentityGuard()
  return <Invoice />
}
