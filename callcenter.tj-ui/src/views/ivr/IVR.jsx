import React, { useEffect, useState } from 'react'
import {
  CCard, CCardBody, CCardHeader, CFormLabel, CFormSelect,
  CTable, CTableHead, CTableHeaderCell, CTableBody, CTableRow,
  CTableDataCell, CBadge, CAlert, CSpinner, CButton,
} from '@coreui/react'
import { useTranslation } from 'react-i18next'
import { kcNumbers as kcNumbersApi } from 'src/api'
import useAuthStore from 'src/store/auth'
import NumberEditor from './NumberEditor'

// ─── Page: overview table of the tenant's KC numbers ──────────────────────
export default function IVR() {
  const { t } = useTranslation()
  const { user, isSuperAdmin } = useAuthStore()
  const superAdmin = isSuperAdmin()

  const [tenantId,    setTenantId]    = useState(superAdmin ? '' : String(user?.tenantId ?? ''))
  const [tenantsList, setTenantsList] = useState([])
  const [numbers,     setNumbers]     = useState([])
  const [loading,     setLoading]     = useState(true)
  const [error,       setError]       = useState('')
  const [selected,    setSelected]    = useState(null) // kc number object or null

  const tid = tenantId

  const load = () => {
    if (!tid) return
    setLoading(true)
    kcNumbersApi.list(tid)
      .then(d => setNumbers(d.numbers || []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => {
    if (superAdmin) {
      import('src/api').then(m => m.tenants.list())
        .then(d => {
          const list = d.tenants ?? []
          setTenantsList(list)
          if (list.length > 0 && !tid) setTenantId(String(list[0].id))
        })
        .catch(() => {})
    }
  }, [])

  useEffect(load, [tid])

  if (selected) {
    return <NumberEditor kcNumber={selected} onBack={() => { setSelected(null); load() }} />
  }

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('ivr.title')}</h4>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {superAdmin && (
        <div className="mb-3" style={{ maxWidth: 340 }}>
          <CFormLabel>{t('common.tenant')}</CFormLabel>
          <CFormSelect value={tid} onChange={e => setTenantId(e.target.value)}>
            <option value="">{t('topics.select_tenant')}</option>
            {tenantsList.map(tn => <option key={tn.id} value={tn.id}>{tn.name}</option>)}
          </CFormSelect>
        </div>
      )}

      {!tid ? (
        <CAlert color="info">{t('common.select_tenant_hint')}</CAlert>
      ) : loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : (
        <CCard>
          <CCardHeader>{t('kc_numbers.tab_title')}</CCardHeader>
          <CCardBody className="p-0">
            {numbers.length === 0 ? (
              <div className="text-center text-muted py-5">
                <div style={{ fontSize: 40 }}>☎️</div>
                <div className="mt-2">{t('kc_numbers.empty_for_tenant')}</div>
                <div className="small mt-1">{t('kc_numbers.added_by_superadmin_hint')}</div>
              </div>
            ) : (
              <CTable hover responsive className="mb-0">
                <CTableHead>
                  <CTableRow>
                    <CTableHeaderCell>{t('kc_numbers.col_number')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('kc_numbers.col_greeting')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('kc_numbers.col_menu')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('kc_numbers.col_agents')}</CTableHeaderCell>
                    <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                  </CTableRow>
                </CTableHead>
                <CTableBody>
                  {numbers.map(n => (
                    <CTableRow key={n.id}>
                      <CTableDataCell className="fw-semibold">{n.number}</CTableDataCell>
                      <CTableDataCell>
                        <CBadge color={n.hasGreeting ? 'success' : 'secondary'}>{n.hasGreeting ? t('kc_numbers.yes') : t('kc_numbers.no')}</CBadge>
                      </CTableDataCell>
                      <CTableDataCell>{n.optionsCount}</CTableDataCell>
                      <CTableDataCell>{n.queueMembers}</CTableDataCell>
                      <CTableDataCell className="text-end">
                        <CButton size="sm" color="primary" onClick={() => setSelected(n)}>
                          {t('kc_numbers.configure')}
                        </CButton>
                      </CTableDataCell>
                    </CTableRow>
                  ))}
                </CTableBody>
              </CTable>
            )}
          </CCardBody>
        </CCard>
      )}
    </>
  )
}
