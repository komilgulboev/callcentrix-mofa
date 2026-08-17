import React from 'react'
import { useNavigate } from 'react-router-dom'
import {
  CAvatar, CContainer, CDropdown, CDropdownDivider,
  CDropdownHeader, CDropdownItem, CDropdownMenu, CDropdownToggle,
  CHeader, CHeaderNav, CHeaderToggler, CNavItem,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilMenu, cilAccountLogout, cilSettings, cilLanguage } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import useUIStore from 'src/store/ui'
import useAuthStore from 'src/store/auth'

const LANGS = ['ru', 'tj', 'en']

export default function AppHeader() {
  const { sidebarOpen, toggleSidebar } = useUIStore()
  const { user, logout } = useAuthStore()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()

  const handleLogout = () => {
    logout()
    navigate('/login')
  }

  const changeLang = (lang) => {
    i18n.changeLanguage(lang)
    localStorage.setItem('ui-lang', lang)
  }

  const roleLabel = user ? (t(`roles.${user.userType}`, { defaultValue: 'User' })) : ''
  const roleColors = ['danger', 'primary', 'warning', 'info']
  const roleColor  = user ? (roleColors[user.userType] ?? 'secondary') : 'secondary'
  const initials   = user?.username?.slice(0, 2).toUpperCase() ?? '?'

  return (
    <CHeader position="sticky" className="mb-4 p-0">
      <CContainer className="border-bottom px-4" fluid>
        <CHeaderToggler onClick={toggleSidebar} style={{ marginInlineStart: '-14px' }}>
          <CIcon icon={cilMenu} size="lg" />
        </CHeaderToggler>

        <CHeaderNav className="ms-auto">
          <CNavItem className="d-flex align-items-center me-3">
            <span className={`badge bg-${roleColor}`}>{roleLabel}</span>
          </CNavItem>

          <CDropdown variant="nav-item" placement="bottom-end">
            <CDropdownToggle caret={false} className="py-0 pe-0">
              <CAvatar color="primary" size="md" textColor="white">
                {initials}
              </CAvatar>
            </CDropdownToggle>
            <CDropdownMenu>
              <CDropdownItem className="fw-semibold" disabled>
                {user?.username}
              </CDropdownItem>
              <CDropdownDivider />
              <CDropdownItem onClick={() => navigate('/settings')}>
                <CIcon icon={cilSettings} className="me-2" /> {t('header.settings')}
              </CDropdownItem>
              <CDropdownDivider />
              <CDropdownHeader>{t('header.language')}</CDropdownHeader>
              {LANGS.map((l) => (
                <CDropdownItem
                  key={l}
                  active={i18n.language === l}
                  onClick={() => changeLang(l)}
                >
                  <CIcon icon={cilLanguage} className="me-2" /> {t(`header.lang_${l}`)}
                </CDropdownItem>
              ))}
              <CDropdownDivider />
              <CDropdownItem onClick={handleLogout} className="text-danger">
                <CIcon icon={cilAccountLogout} className="me-2" /> {t('header.logout')}
              </CDropdownItem>
            </CDropdownMenu>
          </CDropdown>
        </CHeaderNav>
      </CContainer>
    </CHeader>
  )
}
