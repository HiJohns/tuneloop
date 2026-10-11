import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import Checkout from '../Checkout'
export default function CheckoutWithGuard() {
  useIdentityGuard()
  return <Checkout />
}
