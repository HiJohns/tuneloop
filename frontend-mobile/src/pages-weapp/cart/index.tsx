import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Cart from '../../pages/Cart'
export default function CartWithGuard() {
  useIdentityGuard()
  return <Cart />
}
