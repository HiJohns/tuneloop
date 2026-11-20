import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Setting from '../../pages/Setting'
import PageShell from '../../components/PageShell'
export default function SettingWithGuard() {
  useIdentityGuard()
  return <PageShell><Setting /></PageShell>
}
