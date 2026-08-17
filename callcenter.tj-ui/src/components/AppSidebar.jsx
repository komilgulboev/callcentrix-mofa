import React from 'react'
import {
  CCloseButton, CSidebar, CSidebarBrand,
  CSidebarFooter, CSidebarHeader, CSidebarToggler,
} from '@coreui/react'
import AppSidebarNav from './AppSidebarNav'
import { filterNav } from 'src/utils/navFilter'
import useAuthStore from 'src/store/auth'
import useUIStore from 'src/store/ui'
import navigation from 'src/_nav'

export default function AppSidebar() {
  const { sidebarOpen, sidebarUnfoldable, setSidebarOpen, toggleUnfoldable } = useUIStore()
  const user = useAuthStore((s) => s.user)

  const filteredNav = user ? filterNav(navigation, user.userType) : []

  return (
    <CSidebar
      className="border-end"
      colorScheme="dark"
      position="fixed"
      unfoldable={sidebarUnfoldable}
      visible={sidebarOpen}
      onVisibleChange={setSidebarOpen}
    >
      <CSidebarHeader className="border-bottom">
        <CSidebarBrand>
          <span className="fw-bold fs-5 text-white">
            {sidebarUnfoldable ? 'CC' : '📞 CallCentrix'}
          </span>
        </CSidebarBrand>
        <CCloseButton className="d-lg-none" dark onClick={() => setSidebarOpen(false)} />
      </CSidebarHeader>

      <AppSidebarNav items={filteredNav} />

      <CSidebarFooter className="border-top d-none d-lg-flex">
        <CSidebarToggler onClick={toggleUnfoldable} />
      </CSidebarFooter>
    </CSidebar>
  )
}
