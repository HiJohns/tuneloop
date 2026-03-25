import { createContext, useContext, useState, useEffect } from 'react'

const UserContext = createContext(null)

function parseJwt(token) {
  try {
    const base64Url = token.split('.')[1]
    const base64 = base64Url.replace(/-/g, '+').replace(/_/g, '/')
    const jsonPayload = decodeURIComponent(
      atob(base64)
        .split('')
        .map((c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2))
        .join('')
    )
    return JSON.parse(jsonPayload)
  } catch (error) {
    console.error('Failed to parse JWT:', error)
    return null
  }
}

export function UserProvider({ children }) {
  const [user, setUser] = useState(null)
  const [role, setRole] = useState(null)
  const [loading, setLoading] = useState(true)

  const getToken = () => {
    const token = localStorage.getItem('token')
    const expiry = localStorage.getItem('token_expiry')
    
    if (!token || !expiry) return null
    
    if (new Date().getTime() > parseInt(expiry)) {
      localStorage.removeItem('token')
      localStorage.removeItem('token_expiry')
      localStorage.removeItem('user_info')
      return null
    }
    
    return token
  }

  const updateUserFromToken = () => {
    const token = getToken()
    if (token) {
      const decoded = parseJwt(token)
      if (decoded) {
        setUser(decoded)
        setRole(decoded.role || 'USER')
      }
    } else {
      setUser(null)
      setRole(null)
    }
    setLoading(false)
  }

  useEffect(() => {
    updateUserFromToken()
    
    const handleStorageChange = () => {
      updateUserFromToken()
    }

    window.addEventListener('storage', handleStorageChange)
    return () => window.removeEventListener('storage', handleStorageChange)
  }, [])

  const isTechnician = () => role === 'TECHNICIAN'
  const isAdmin = () => role === 'ADMIN' || role === 'SYSADMIN'
  const isUser = () => role === 'USER' || !role

  return (
    <UserContext.Provider value={{ 
      user, 
      role, 
      loading, 
      isTechnician, 
      isAdmin, 
      isUser,
      updateUserFromToken 
    }}>
      {children}
    </UserContext.Provider>
  )
}

export const useUser = () => {
  const context = useContext(UserContext)
  if (!context) {
    throw new Error('useUser must be used within a UserProvider')
  }
  return context
}