import React, { useEffect, useState } from 'react'
import {
  CButton, CCard, CCardBody, CModal, CModalBody, CModalFooter,
  CModalHeader, CModalTitle, CForm, CFormInput, CFormLabel,
  CFormSelect, CFormCheck, CTable, CTableBody, CTableDataCell,
  CTableHead, CTableHeaderCell, CTableRow, CBadge, CAlert,
  CSpinner, CInputGroup, CInputGroupText,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilPencil, cilTrash, cilSearch, cilCheckCircle, cilBan } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { blacklist as blacklistApi, tenants as tenantsApi } from 'src/api'
import useAuthStore from 'src/store/auth'

// Preset blacklist terms, in days; null = permanent.
const DURATIONS = [1, 3, 7, 30, 90, null]

const EMPTY = { phone: '', comment: '', active: true, durationDays: 7 }

// Picks the closest preset duration for an existing entry's remaining time,
// so the edit modal opens with a sensible default instead of always resetting.
function durationFromExpiresAt(expiresAt) {
  if (!expiresAt) return null
  const days = Math.ceil((new Date(expiresAt) - new Date()) / 86400000)
  const presets = DURATIONS.filter(d => d !== null)
  return presets.reduce((best, d) => (Math.abs(d - days) < Math.abs(best - days) ? d : best), presets[0])
}

export default function Blacklist() {
  const { user, isSuperAdmin } = useAuthStore()
  const { t } = useTranslation()
  const superAdmin = isSuperAdmin()

  const [rows,         setRows]         = useState([])
  const [loading,      setLoading]      = useState(true)
  const [error,        setError]        = useState('')
  const [modal,        setModal]        = useState(false)
  const [editing,      setEditing]      = useState(null)
  const [form,         setForm]         = useState(EMPTY)
  const [saving,       setSaving]       = useState(false)
  const [search,       setSearch]       = useState('')
  const [tenantsList,  setTenantsList]  = useState([])
  const [selectedTid,  setSelectedTid]  = useState(superAdmin ? '' : String(user?.tenantId ?? ''))

  const tid = selectedTid

  useEffect(() => {
    if (!superAdmin) return
    tenantsApi.list()
      .then(d => {
        const list = d.tenants ?? []
        setTenantsList(list)
        if (list.length > 0 && !tid) setSelectedTid(String(list[0].id))
      })
      .catch(() => {})
  }, [])

  const load = () => {
    if (!tid) return
    setLoading(true)
    blacklistApi.list(tid, search)
      .then(d => setRows(d.entries ?? []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [tid])

  const handleSearch = (e) => { e.preventDefault(); load() }

  const openCreate = () => { setEditing(null); setForm(EMPTY); setModal(true) }
  const openEdit   = (e) => {
    setEditing(e)
    setForm({ phone: e.phone, comment: e.comment, active: e.active, durationDays: durationFromExpiresAt(e.expiresAt) })
    setModal(true)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      if (editing) await blacklistApi.update(tid, editing.id, form)
      else         await blacklistApi.create(tid, form)
      setModal(false)
      load()
    } catch (e) {
      if (e.message === 'phone_exists') setError(t('blacklist.phone_exists'))
      else setError(e.message)
    }
    finally { setSaving(false) }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('blacklist.delete_confirm'))) return
    try { await blacklistApi.remove(tid, id); load() }
    catch (e) { setError(e.message) }
  }

  const handleToggle = async (id) => {
    try { await blacklistApi.toggle(tid, id); load() }
    catch (e) { setError(e.message) }
  }

  const durationLabel = (d) => d === null ? t('blacklist.duration_permanent') : t(`blacklist.duration_${d}d`)

  const entryStatus = (e) => {
    if (!e.active) return { color: 'secondary', label: t('blacklist.inactive') }
    if (e.expiresAt && new Date(e.expiresAt) <= new Date()) return { color: 'warning', label: t('blacklist.expired') }
    return { color: 'danger', label: t('blacklist.active') }
  }

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <div>
          <h4 className="mb-0">{t('blacklist.title')}</h4>
          <div className="text-muted small mt-1">{t('blacklist.subtitle')}</div>
        </div>
        <CButton color="primary" onClick={openCreate} disabled={!tid}>
          <CIcon icon={cilPlus} className="me-2" />{t('blacklist.add_number')}
        </CButton>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {superAdmin && (
        <div className="mb-3" style={{ maxWidth: 340 }}>
          <CFormLabel>{t('common.tenant')}</CFormLabel>
          <CFormSelect value={tid} onChange={e => setSelectedTid(e.target.value)}>
            <option value="">{t('blacklist.select_tenant')}</option>
            {tenantsList.map(tp => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
          </CFormSelect>
        </div>
      )}

      <div className="mb-3">
        <form onSubmit={handleSearch} className="d-flex gap-2">
          <CInputGroup style={{ maxWidth: 360 }}>
            <CInputGroupText><CIcon icon={cilSearch} /></CInputGroupText>
            <CFormInput
              placeholder={t('blacklist.search_placeholder')}
              value={search}
              onChange={e => setSearch(e.target.value)}
            />
          </CInputGroup>
          <CButton type="submit" color="light">{t('common.search')}</CButton>
          {search && (
            <CButton color="light" onClick={() => { setSearch(''); setTimeout(load, 0) }}>✕</CButton>
          )}
        </form>
      </div>

      <CCard>
        <CCardBody className="p-0">
          {!tid ? (
            <div className="text-center text-muted py-5">{t('common.select_tenant_hint')}</div>
          ) : loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive>
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell>#</CTableHeaderCell>
                  <CTableHeaderCell>{t('blacklist.col_phone')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('blacklist.col_comment')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('blacklist.col_status')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('blacklist.col_expires')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('blacklist.col_added')}</CTableHeaderCell>
                  <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map(e => {
                  const status = entryStatus(e)
                  return (
                    <CTableRow key={e.id}>
                      <CTableDataCell className="text-muted">{e.id}</CTableDataCell>
                      <CTableDataCell className="fw-semibold font-monospace">{e.phone}</CTableDataCell>
                      <CTableDataCell className="text-muted">{e.comment || '—'}</CTableDataCell>
                      <CTableDataCell>
                        <CBadge color={status.color}>{status.label}</CBadge>
                      </CTableDataCell>
                      <CTableDataCell className="text-muted small">
                        {e.expiresAt ? new Date(e.expiresAt).toLocaleDateString() : t('blacklist.duration_permanent')}
                      </CTableDataCell>
                      <CTableDataCell className="text-muted small">
                        {new Date(e.createdAt).toLocaleDateString()}
                      </CTableDataCell>
                      <CTableDataCell className="text-end">
                        <div className="d-flex gap-1 justify-content-end">
                          <CButton size="sm" color="light" onClick={() => openEdit(e)}>
                            <CIcon icon={cilPencil} />
                          </CButton>
                          <CButton
                            size="sm"
                            color={e.active ? 'warning' : 'success'}
                            onClick={() => handleToggle(e.id)}
                          >
                            <CIcon icon={e.active ? cilBan : cilCheckCircle} />
                          </CButton>
                          <CButton size="sm" color="danger" onClick={() => handleDelete(e.id)}>
                            <CIcon icon={cilTrash} />
                          </CButton>
                        </div>
                      </CTableDataCell>
                    </CTableRow>
                  )
                })}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={7} className="text-center text-muted py-4">
                      {search ? t('blacklist.empty_search') : t('blacklist.empty')}
                    </CTableDataCell>
                  </CTableRow>
                )}
              </CTableBody>
            </CTable>
          )}
        </CCardBody>
      </CCard>

      {rows.length > 0 && (
        <div className="text-muted small mt-2 text-end">
          {t('blacklist.total', { total: rows.length, active: rows.filter(r => r.active).length })}
        </div>
      )}

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader>
          <CModalTitle>{editing ? t('blacklist.edit_number') : t('blacklist.new_number')}</CModalTitle>
        </CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <div>
              <CFormLabel>{t('blacklist.phone_label')} <span className="text-danger">*</span></CFormLabel>
              <CFormInput
                value={form.phone}
                onChange={e => setForm(f => ({ ...f, phone: e.target.value }))}
                placeholder="+992XXXXXXXXX"
                autoFocus
              />
              <div className="text-muted small mt-1">{t('blacklist.phone_hint')}</div>
            </div>
            <div>
              <CFormLabel>{t('blacklist.comment_label')}</CFormLabel>
              <CFormInput
                value={form.comment}
                onChange={e => setForm(f => ({ ...f, comment: e.target.value }))}
                placeholder={t('blacklist.comment_placeholder')}
              />
            </div>
            <div>
              <CFormLabel>{t('blacklist.duration_label')} <span className="text-danger">*</span></CFormLabel>
              <CFormSelect
                value={form.durationDays === null ? 'permanent' : String(form.durationDays)}
                onChange={e => setForm(f => ({ ...f, durationDays: e.target.value === 'permanent' ? null : Number(e.target.value) }))}
              >
                {DURATIONS.map(d => (
                  <option key={d ?? 'permanent'} value={d ?? 'permanent'}>{durationLabel(d)}</option>
                ))}
              </CFormSelect>
            </div>
            <CFormCheck
              label={t('blacklist.active_label')}
              checked={form.active}
              onChange={e => setForm(f => ({ ...f, active: e.target.checked }))}
            />
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving || !form.phone}>
            {saving ? <CSpinner size="sm" /> : t('common.save')}
          </CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
