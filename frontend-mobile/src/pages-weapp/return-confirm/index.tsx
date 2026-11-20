import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ReturnConfirm from '../../pages/ReturnConfirm'
import PageShell from '../../components/PageShell'
export default function ReturnConfirmWithGuard() {
  useIdentityGuard()
  return <PageShell><ReturnConfirm /></PageShell>
}
