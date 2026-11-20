import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ProfileComplete from './ProfileComplete'
import PageShell from '../../components/PageShell'
export default function ProfileCompleteWithGuard() {
  useIdentityGuard()
  return <PageShell><ProfileComplete /></PageShell>
}
