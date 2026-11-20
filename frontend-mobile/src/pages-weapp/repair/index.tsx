import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairWorkflow from '../../pages/RepairWorkflow'
import PageShell from '../../components/PageShell'
export default function RepairWorkflowWithGuard() {
  useIdentityGuard()
  return <PageShell><RepairWorkflow /></PageShell>
}
