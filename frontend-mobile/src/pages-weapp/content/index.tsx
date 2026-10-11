import { useIdentityGuard } from '../../hooks/useIdentityGuard'
import ContentPage from '../../pages/ContentPage'
export default function ContentPageWithGuard() {
  useIdentityGuard()
  return <ContentPage />
}
