import React, { Suspense } from 'react'
import { Navigate, Route, Routes } from 'react-router-dom'
import { CContainer, CSpinner } from '@coreui/react'
import routes from 'src/routes'
import useAuthStore from 'src/store/auth'

export default function AppContent() {
  const user = useAuthStore((s) => s.user)

  return (
    <CContainer className="px-4" lg>
      <Suspense fallback={<div className="d-flex justify-content-center py-5"><CSpinner /></div>}>
        <Routes>
          {routes.map((route, i) => {
            // Role gate: if route has roles and user doesn't match, skip rendering
            if (route.roles && user && !route.roles.includes(user.userType)) {
              return (
                <Route key={i} path={route.path} element={<Navigate to="/dashboard" replace />} />
              )
            }
            return (
              route.element && (
                <Route key={i} path={route.path} element={<route.element />} />
              )
            )
          })}
          <Route path="/" element={<Navigate to="/dashboard" replace />} />
        </Routes>
      </Suspense>
    </CContainer>
  )
}
