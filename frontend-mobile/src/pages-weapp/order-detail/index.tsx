import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import OrderDetail from './OrderDetail'
export default function OrderDetailWithGuard() {
  useIdentityGuard()
  return <OrderDetail />
}
