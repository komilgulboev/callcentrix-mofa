import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import useAuthStore from 'src/store/auth'

// setTimeout delays are clamped to a 32-bit signed int internally; anything
// longer fires immediately. Session TTLs (e.g. a 12h operator shift) fit
// well under this, but cap it defensively anyway.
const MAX_TIMEOUT_MS = 2_147_483_647

// Actively logs the user out once their JWT expires, even if they never
// navigate or make another API call while the expired tab sits open — the
// existing 401 interceptor and route guard only catch expiry reactively,
// on the next request/navigation.
export default function useSessionExpiry() {
  const token = useAuthStore((s) => s.token)
  const exp = useAuthStore((s) => s.user?.exp)
  const logout = useAuthStore((s) => s.logout)
  const navigate = useNavigate()

  useEffect(() => {
    if (!token || !exp) return undefined

    const expire = () => {
      logout()
      navigate('/login', { replace: true })
    }

    const msRemaining = exp * 1000 - Date.now()
    if (msRemaining <= 0) {
      expire()
      return undefined
    }

    const timer = setTimeout(expire, Math.min(msRemaining, MAX_TIMEOUT_MS))
    return () => clearTimeout(timer)
  }, [token, exp, logout, navigate])
}
