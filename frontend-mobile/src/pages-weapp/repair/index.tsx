import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import RepairWorkflow from '../../pages/RepairWorkflow'
export default function RepairWorkflowWithGuard() {
  useIdentityGuard()
  return <RepairWorkflow />
}
