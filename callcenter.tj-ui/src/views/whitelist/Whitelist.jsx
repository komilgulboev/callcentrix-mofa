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
import { whitelist as whitelistApi, tenants as tenantsApi } from 'src/api'
import useAuthStore from 'src/store/auth'

const EMPTY = { phone: '', description: '', active: true }

export default function Whitelist() {
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
    whitelistApi.list(tid, search)
      .then(d => setRows(d.entries ?? []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [tid])

  const handleSearch = (e) => { e.preventDefault(); load() }

  const openCreate = () => { setEditing(null); setForm(EMPTY); setModal(true) }
  const openEdit   = (e) => {
    setEditing(e)
    setForm({ phone: e.phone, description: e.description, active: e.active })
    setModal(true)
  }

  const handleSave = async () => {
    setSaving(true)
    try {
      if (editing) await whitelistApi.update(tid, editing.id, form)
      else         await whitelistApi.create(tid, form)
      setModal(false)
      load()
    } catch (e) {
      if (e.message === 'phone_exists') setError(t('whitelist.phone_exists'))
      else setError(e.message)
    }
    finally { setSaving(false) }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('whitelist.delete_confirm'))) return
    try { await whitelistApi.remove(tid, id); load() }
    catch (e) { setError(e.message) }
  }

  const handleToggle = async (id) => {
    try { await whitelistApi.toggle(tid, id); load() }
    catch (e) { setError(e.message) }
  }

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <div>
          <h4 className="mb-0">{t('whitelist.title')}</h4>
          <div className="text-muted small mt-1">{t('whitelist.subtitle')}</div>
        </div>
        <CButton color="primary" onClick={openCreate} disabled={!tid}>
          <CIcon icon={cilPlus} className="me-2" />{t('whitelist.add_number')}
        </CButton>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {superAdmin && (
        <div className="mb-3" style={{ maxWidth: 340 }}>
          <CFormLabel>{t('common.tenant')}</CFormLabel>
          <CFormSelect value={tid} onChange={e => setSelectedTid(e.target.value)}>
            <option value="">{t('whitelist.select_tenant')}</option>
            {tenantsList.map(tp => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
          </CFormSelect>
        </div>
      )}

      <div className="mb-3">
        <form onSubmit={handleSearch} className="d-flex gap-2">
          <CInputGroup style={{ maxWidth: 360 }}>
            <CInputGroupText><CIcon icon={cilSearch} /></CInputGroupText>
            <CFormInput
              placeholder={t('whitelist.search_placeholder')}
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
            <div className="text-center text-muted py-5">{t('whitelist.select_tenant_hint')}</div>
          ) : loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive>
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell>#</CTableHeaderCell>
                  <CTableHeaderCell>{t('whitelist.col_phone')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('whitelist.col_description')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('whitelist.col_status')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('whitelist.col_added')}</CTableHeaderCell>
                  <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map(e => (
                  <CTableRow key={e.id}>
                    <CTableDataCell className="text-muted">{e.id}</CTableDataCell>
                    <CTableDataCell className="fw-semibold font-monospace">{e.phone}</CTableDataCell>
                    <CTableDataCell className="text-muted">{e.description || '—'}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={e.active ? 'success' : 'secondary'}>
                        {e.active ? t('whitelist.active') : t('whitelist.inactive')}
                      </CBadge>
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
                ))}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={6} className="text-center text-muted py-4">
                      {search ? t('whitelist.empty_search') : t('whitelist.empty')}
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
          {t('whitelist.total', { total: rows.length, active: rows.filter(r => r.active).length })}
        </div>
      )}

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader>
          <CModalTitle>{editing ? t('whitelist.edit_number') : t('whitelist.new_number')}</CModalTitle>
        </CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <div>
              <CFormLabel>{t('whitelist.phone_label')} <span className="text-danger">*</span></CFormLabel>
              <CFormInput
                value={form.phone}
                onChange={e => setForm(f => ({ ...f, phone: e.target.value }))}
                placeholder="+992XXXXXXXXX"
                autoFocus
              />
              <div className="text-muted small mt-1">{t('whitelist.phone_hint')}</div>
            </div>
            <div>
              <CFormLabel>{t('whitelist.description_label')}</CFormLabel>
              <CFormInput
                value={form.description}
                onChange={e => setForm(f => ({ ...f, description: e.target.value }))}
                placeholder={t('whitelist.description_placeholder')}
              />
            </div>
            <CFormCheck
              label={t('whitelist.active_label')}
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
