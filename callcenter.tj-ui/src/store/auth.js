import { create } from 'zustand'
import { persist } from 'zustand/middleware'

function parseJwt(token) {
  try {
    const claims = JSON.parse(atob(token.split('.')[1]))
    if (claims.sub && !claims.id) claims.id = claims.sub
    return claims
  } catch {
    return null
  }
}

const useAuthStore = create(
  persist(
    (set, get) => ({
      token: localStorage.getItem('accessToken'),
      user: parseJwt(localStorage.getItem('accessToken')),

      setToken(token) {
        localStorage.setItem('accessToken', token)
        set({ token, user: parseJwt(token) })
      },

      logout() {
        localStorage.removeItem('accessToken')
        localStorage.removeItem('sipPassword')
        set({ token: null, user: null })
      },

      isAuthenticated() {
        const { user } = get()
        if (!user) return false
        return user.exp * 1000 > Date.now()
      },

      isSuperAdmin()  { return get().user?.userType === 0 },
      isTenantAdmin() { return get().user?.userType === 1 },
      isSupervisor()  { return get().user?.userType === 2 },
      isOperator()    { return get().user?.userType === 3 },
    }),
    { name: 'cx-auth', partialize: (s) => ({ token: s.token }) },
  ),
)

export default useAuthStore
