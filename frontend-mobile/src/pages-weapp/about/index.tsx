import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import About from '../../pages/About'
export default function AboutWithGuard() {
  useIdentityGuard()
  return <About />
}
