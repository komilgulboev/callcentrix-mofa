import React, { useEffect, useState } from 'react'
import {
  CCard, CCardBody, CTable, CTableBody, CTableDataCell, CTableHead,
  CTableHeaderCell, CTableRow, CButton, CBadge, CAlert, CSpinner,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilCheckCircle } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { users as usersApi } from 'src/api'

export default function UnauthorizedUsers() {
  const { t } = useTranslation()
  const [rows,    setRows]    = useState([])
  const [loading, setLoading] = useState(true)
  const [error,   setError]   = useState('')
  const [saving,  setSaving]  = useState(null) // userId being authorized

  const load = () => {
    setLoading(true)
    usersApi.listUnauthorized()
      .then((d) => setRows(d.users ?? d))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const handleAuthorize = async (id) => {
    setSaving(id)
    try {
      await usersApi.authorize(id)
      load()
    } catch (e) { setError(e.message) }
    finally { setSaving(null) }
  }

  return (
    <>
      <div className="mb-4">
        <h4 className="mb-0">{t('unauthorized_users.title')}</h4>
        <small className="text-muted">{t('unauthorized_users.subtitle')}</small>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <CCard>
        <CCardBody className="p-0">
          {loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive>
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell>{t('unauthorized_users.col_name')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('unauthorized_users.col_phone')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('unauthorized_users.col_code')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('unauthorized_users.col_registered_at')}</CTableHeaderCell>
                  <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map((u) => (
                  <CTableRow key={u.id}>
                    <CTableDataCell>
                      {[u.firstName, u.lastName].filter(Boolean).join(' ') || '—'}
                    </CTableDataCell>
                    <CTableDataCell className="fw-semibold">{u.sipNo || u.username}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color="warning" textColor="dark">{u.authCode}</CBadge>
                    </CTableDataCell>
                    <CTableDataCell className="text-muted">{u.createdAt}</CTableDataCell>
                    <CTableDataCell className="text-end">
                      <CButton
                        size="sm" color="success"
                        onClick={() => handleAuthorize(u.id)}
                        disabled={saving === u.id}
                      >
                        {saving === u.id
                          ? <CSpinner size="sm" />
                          : <><CIcon icon={cilCheckCircle} className="me-1" />{t('unauthorized_users.authorize')}</>}
                      </CButton>
                    </CTableDataCell>
                  </CTableRow>
                ))}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={5} className="text-center text-muted py-4">
                      {t('unauthorized_users.empty')}
                    </CTableDataCell>
                  </CTableRow>
                )}
              </CTableBody>
            </CTable>
          )}
        </CCardBody>
      </CCard>
    </>
  )
}
