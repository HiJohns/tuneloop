import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ContentPage from '../../pages/ContentPage'
import PageShell from '../../components/PageShell'
export default function ContentPageWithGuard() {
  useIdentityGuard()
  return <PageShell><ContentPage /></PageShell>
}
