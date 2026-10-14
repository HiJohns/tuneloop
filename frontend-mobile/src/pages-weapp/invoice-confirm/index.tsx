import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import InvoiceConfirm from '../../pages/InvoiceConfirm'
export default function InvoiceConfirmWithGuard() {
  useIdentityGuard()
  return <InvoiceConfirm />
}
