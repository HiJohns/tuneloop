import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Setting from '../../pages/Setting'
export default function SettingWithGuard() {
  useIdentityGuard()
  return <Setting />
}
