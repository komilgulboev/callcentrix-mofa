import React from 'react'
import { Navigate } from 'react-router-dom'
import useAuthStore from 'src/store/auth'

export default function PublicRoute({ children }) {
  const isAuthenticated = useAuthStore((s) => s.isAuthenticated())
  return isAuthenticated ? <Navigate to="/dashboard" replace /> : children
}
