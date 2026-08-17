import React, { Suspense, useEffect } from 'react'
import { HashRouter, Route, Routes } from 'react-router-dom'
import { CSpinner } from '@coreui/react'
import PrivateRoute from 'src/components/PrivateRoute'
import PublicRoute  from 'src/components/PublicRoute'
import useBrandingStore from 'src/store/branding'

const DefaultLayout = React.lazy(() => import('src/layout/DefaultLayout'))
const Login         = React.lazy(() => import('src/views/pages/login/Login'))
const Page404       = React.lazy(() => import('src/views/pages/page404/Page404'))
const Page500       = React.lazy(() => import('src/views/pages/page500/Page500'))

export default function App() {
  const platformName = useBrandingStore((s) => s.platformName)
  const load         = useBrandingStore((s) => s.load)

  useEffect(() => { load() }, [load])
  useEffect(() => { document.title = platformName }, [platformName])

  return (
    <HashRouter>
      <Suspense
        fallback={
          <div className="d-flex justify-content-center align-items-center vh-100">
            <CSpinner color="primary" />
          </div>
        }
      >
        <Routes>
          <Route path="/login" element={<PublicRoute><Login /></PublicRoute>} />
          <Route path="/404"   element={<Page404 />} />
          <Route path="/500"   element={<Page500 />} />
          <Route path="*"      element={<PrivateRoute><DefaultLayout /></PrivateRoute>} />
        </Routes>
      </Suspense>
    </HashRouter>
  )
}
