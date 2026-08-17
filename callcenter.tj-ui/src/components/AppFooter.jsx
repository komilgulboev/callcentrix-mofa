import React, { useEffect, useState } from 'react'
import { CFooter } from '@coreui/react'
import { useTranslation } from 'react-i18next'
import { version as versionApi } from 'src/api'

function fmt(iso) {
  if (!iso) return '?'
  const d = new Date(iso)
  return isNaN(d) ? iso : d.toLocaleString()
}

export default function AppFooter() {
  const { t } = useTranslation()
  const [backend, setBackend] = useState(null)

  useEffect(() => {
    versionApi.get().then(setBackend).catch(() => setBackend(null))
  }, [])

  return (
    <CFooter className="px-4">
      <div className="text-muted small">
        UI {__BUILD_COMMIT__} · {fmt(__BUILD_TIME__)}
        {backend && <> &nbsp;|&nbsp; API {backend.commit} · {fmt(backend.buildTime)}</>}
      </div>
      <div className="ms-auto">
        <span className="me-1">{t('footer.powered_by')}</span>
        <strong>CallCentrix</strong>
      </div>
    </CFooter>
  )
}
