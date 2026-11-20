import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TechRepairWorkbench from '../../pages/TechRepairWorkbench'
import PageShell from '../../components/PageShell'
export default function TechRepairWorkbenchWithGuard() {
  useIdentityGuard()
  return <PageShell><TechRepairWorkbench /></PageShell>
}
