import { create } from 'zustand'
import { persist } from 'zustand/middleware'

const useUIStore = create(
  persist(
    (set, get) => ({
      sidebarOpen: true,
      sidebarUnfoldable: false,
      theme: 'light',

      toggleSidebar:     () => set((s) => ({ sidebarOpen: !s.sidebarOpen })),
      setSidebarOpen:    (v) => set({ sidebarOpen: v }),
      toggleUnfoldable:  () => set((s) => ({ sidebarUnfoldable: !s.sidebarUnfoldable })),
      setTheme:          (t) => set({ theme: t }),
    }),
    { name: 'cx-ui' },
  ),
)

export default useUIStore
