import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import TechRepairWorkbench from '../../pages/TechRepairWorkbench'
export default function TechRepairWorkbenchWithGuard() {
  useIdentityGuard()
  return <TechRepairWorkbench />
}
