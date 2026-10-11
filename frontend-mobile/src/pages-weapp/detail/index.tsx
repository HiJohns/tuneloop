import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Detail from '../Detail'
export default function DetailWithGuard() {
  useIdentityGuard()
  return <Detail />
}
