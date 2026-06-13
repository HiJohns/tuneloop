import { storage, navigation } from '../platform'

export function requireAuth() {
  const token = storage.getItem('token')
  if (!token) {
    navigation.redirect('/pages/login/index')
    return false
  }
  return true
}

export function getToken() {
  return storage.getItem('token')
}
