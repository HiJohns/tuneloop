import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ProfileComplete from './ProfileComplete'
export default function ProfileCompleteWithGuard() {
  useIdentityGuard()
  return <ProfileComplete />
}
