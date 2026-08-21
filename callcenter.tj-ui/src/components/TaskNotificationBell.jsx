import React from 'react'
import {
  CDropdown, CDropdownToggle, CDropdownMenu, CDropdownItem,
  CDropdownHeader, CDropdownDivider, CBadge,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilBell } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import useTaskNotifications from 'src/hooks/useTaskNotifications'

export default function TaskNotificationBell({ enabled }) {
  const { t } = useTranslation()
  const { items, unreadCount, markRead, markAllRead } = useTaskNotifications(enabled)

  if (!enabled) return null

  return (
    <CDropdown variant="nav-item" placement="bottom-end" className="me-2">
      <CDropdownToggle caret={false} className="position-relative py-0">
        <CIcon icon={cilBell} size="lg" />
        {unreadCount > 0 && (
          <CBadge color="danger" shape="rounded-pill" position="top-end" style={{ fontSize: 10 }}>
            {unreadCount > 9 ? '9+' : unreadCount}
          </CBadge>
        )}
      </CDropdownToggle>
      <CDropdownMenu style={{ minWidth: 300, maxHeight: 360, overflowY: 'auto' }}>
        <CDropdownHeader>{t('tasks.notifications_title')}</CDropdownHeader>
        {unreadCount > 0 && (
          <CDropdownItem onClick={markAllRead} className="small text-primary">
            {t('tasks.mark_all_read')}
          </CDropdownItem>
        )}
        <CDropdownDivider />
        {!items.length && (
          <div className="text-muted small text-center py-3">{t('tasks.no_notifications')}</div>
        )}
        {items.map((n) => (
          <CDropdownItem
            key={n.id}
            onClick={() => !n.isRead && markRead(n.id)}
            className={n.isRead ? 'text-muted' : 'fw-semibold'}
            style={{ whiteSpace: 'normal' }}
          >
            <div className="small">{n.message}</div>
            <div className="text-muted" style={{ fontSize: 11 }}>
              {new Date(n.createdAt).toLocaleString()}
            </div>
          </CDropdownItem>
        ))}
      </CDropdownMenu>
    </CDropdown>
  )
}
