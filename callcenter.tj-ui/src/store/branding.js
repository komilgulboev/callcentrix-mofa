import { create } from 'zustand'
import { settings as settingsApi } from 'src/api'

export const DEFAULT_PLATFORM_NAME = 'CallCentrix'

const useBrandingStore = create((set) => ({
  platformName: DEFAULT_PLATFORM_NAME,
  systemInfo: '',
  hasLogo: false,
  updatedAt: '',
  registrationEnabled: false,
  loaded: false,

  load() {
    settingsApi.branding()
      .then((b) => set({
        platformName: b.platformName || DEFAULT_PLATFORM_NAME,
        systemInfo: b.systemInfo || '',
        hasLogo: !!b.hasLogo,
        updatedAt: b.updatedAt || '',
        registrationEnabled: !!b.registrationEnabled,
        loaded: true,
      }))
      .catch(() => set({ loaded: true }))
  },
}))

export default useBrandingStore
