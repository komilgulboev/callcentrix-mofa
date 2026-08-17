import React from 'react'
import { NavLink } from 'react-router-dom'
import { CBadge, CNavItem, CNavLink, CNavTitle, CSidebarNav } from '@coreui/react'
import CIcon from '@coreui/icons-react'
import SimpleBar from 'simplebar-react'
import { useTranslation } from 'react-i18next'

export default function AppSidebarNav({ items }) {
  const { t } = useTranslation()

  return (
    <CSidebarNav as={SimpleBar}>
      {items.map((item, i) => {
        if (item.component === 'CNavTitle') {
          return <CNavTitle key={i}>{t(item.name)}</CNavTitle>
        }
        return (
          <CNavItem key={i}>
            <CNavLink as={NavLink} to={item.to}>
              {item.icon && <CIcon customClassName="nav-icon" icon={item.icon} />}
              {t(item.name)}
              {item.badge && (
                <CBadge color={item.badge.color} className="ms-auto">
                  {item.badge.text}
                </CBadge>
              )}
            </CNavLink>
          </CNavItem>
        )
      })}
    </CSidebarNav>
  )
}
