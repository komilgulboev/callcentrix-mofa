import React, { useEffect, useState } from 'react'
import {
  CButton, CCard, CCardBody, CModal, CModalBody, CModalFooter,
  CModalHeader, CModalTitle, CForm, CFormInput, CFormLabel,
  CFormSelect, CFormCheck, CTable, CTableBody, CTableDataCell,
  CTableHead, CTableHeaderCell, CTableRow, CBadge, CAlert, CSpinner,
  CButtonGroup,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilPencil, cilTrash } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { topics as topicsApi, tenants as tenantsApi } from 'src/api'
import useAuthStore from 'src/store/auth'

const LANGS = ['ru', 'tj', 'en']
const LANG_LABEL = { ru: 'RU', tj: 'TJ', en: 'EN' }
const EMPTY_NAMES = { ru: '', tj: '', en: '' }

export function topicName(topic, lang) {
  if (!topic?.names) return '—'
  return topic.names[lang] || topic.names.ru || topic.names.tj || topic.names.en || '—'
}

export default function Topics() {
  const { user, isSuperAdmin } = useAuthStore()
  const { t, i18n } = useTranslation()
  const superAdmin = isSuperAdmin()

  const [tenantsList,     setTenantsList]     = useState([])
  const [selectedTenant,  setSelectedTenant]  = useState(superAdmin ? '' : String(user?.tenantId ?? ''))
  const [rows,            setRows]            = useState([])
  const [loading,         setLoading]         = useState(false)
  const [error,           setError]           = useState('')
  const [modal,           setModal]           = useState(false)
  const [editing,         setEditing]         = useState(null)
  const [form,            setForm]            = useState({ names: { ...EMPTY_NAMES }, active: true })
  const [saving,          setSaving]          = useState(false)
  const [displayLang,     setDisplayLang]     = useState(i18n.language)

  useEffect(() => {
    if (!superAdmin) return
    tenantsApi.list()
      .then((d) => {
        const list = d.tenants ?? d
        setTenantsList(list)
        if (list.length > 0 && !selectedTenant) setSelectedTenant(String(list[0].id))
      })
      .catch((e) => setError(e.message))
  }, [])

  const tenantId = superAdmin ? selectedTenant : String(user?.tenantId ?? '')

  const load = () => {
    if (!tenantId) return
    setLoading(true)
    topicsApi.list(tenantId)
      .then((d) => setRows(d.topics ?? d))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { load() }, [tenantId])

  const openCreate = () => {
    setEditing(null)
    setForm({ names: { ...EMPTY_NAMES }, active: true })
    setModal(true)
  }

  const openEdit = (tp) => {
    setEditing(tp)
    setForm({ names: { ...EMPTY_NAMES, ...tp.names }, active: tp.active ?? true })
    setModal(true)
  }

  const setName = (l, val) => setForm((f) => ({ ...f, names: { ...f.names, [l]: val } }))

  const handleSave = async () => {
    setSaving(true)
    try {
      if (editing) await topicsApi.update(tenantId, editing.id, { names: form.names, active: form.active })
      else         await topicsApi.create(tenantId, { names: form.names, active: form.active })
      setModal(false)
      load()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('topics.delete_confirm'))) return
    try { await topicsApi.remove(tenantId, id); load() }
    catch (e) { setError(e.message) }
  }

  const hasName = LANGS.some((l) => form.names[l].trim())

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('topics.title')}</h4>
        <div className="d-flex gap-2 align-items-center">
          <CButtonGroup size="sm">
            {LANGS.map((l) => (
              <CButton key={l} color={displayLang === l ? 'primary' : 'light'} onClick={() => setDisplayLang(l)}>
                {LANG_LABEL[l]}
              </CButton>
            ))}
          </CButtonGroup>
          <CButton color="primary" onClick={openCreate} disabled={!tenantId}>
            <CIcon icon={cilPlus} className="me-2" />{t('topics.new_topic')}
          </CButton>
        </div>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {superAdmin && (
        <div className="mb-3" style={{ maxWidth: 340 }}>
          <CFormLabel>{t('common.tenant')}</CFormLabel>
          <CFormSelect value={selectedTenant} onChange={(e) => setSelectedTenant(e.target.value)}>
            <option value="">{t('topics.select_tenant')}</option>
            {tenantsList.map((tp) => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
          </CFormSelect>
        </div>
      )}

      <CCard>
        <CCardBody className="p-0">
          {loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive>
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell style={{ width: 60 }}>#</CTableHeaderCell>
                  <CTableHeaderCell>{t('topics.col_name')} ({LANG_LABEL[displayLang]})</CTableHeaderCell>
                  <CTableHeaderCell>RU</CTableHeaderCell>
                  <CTableHeaderCell>TJ</CTableHeaderCell>
                  <CTableHeaderCell>EN</CTableHeaderCell>
                  <CTableHeaderCell>{t('topics.col_status')}</CTableHeaderCell>
                  <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map((tp) => (
                  <CTableRow key={tp.id}>
                    <CTableDataCell className="text-muted">{tp.id}</CTableDataCell>
                    <CTableDataCell className="fw-semibold">{topicName(tp, displayLang)}</CTableDataCell>
                    <CTableDataCell className="text-muted small">{tp.names?.ru || '—'}</CTableDataCell>
                    <CTableDataCell className="text-muted small">{tp.names?.tj || '—'}</CTableDataCell>
                    <CTableDataCell className="text-muted small">{tp.names?.en || '—'}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={tp.active ? 'success' : 'secondary'}>
                        {tp.active ? t('topics.active') : t('topics.inactive')}
                      </CBadge>
                    </CTableDataCell>
                    <CTableDataCell className="text-end">
                      <div className="d-flex gap-1 justify-content-end">
                        <CButton size="sm" color="light" onClick={() => openEdit(tp)}>
                          <CIcon icon={cilPencil} />
                        </CButton>
                        <CButton size="sm" color="danger" onClick={() => handleDelete(tp.id)}>
                          <CIcon icon={cilTrash} />
                        </CButton>
                      </div>
                    </CTableDataCell>
                  </CTableRow>
                ))}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={7} className="text-center text-muted py-4">
                      {tenantId ? t('topics.empty') : t('topics.empty_select')}
                    </CTableDataCell>
                  </CTableRow>
                )}
              </CTableBody>
            </CTable>
          )}
        </CCardBody>
      </CCard>

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader>
          <CModalTitle>{editing ? t('topics.edit') : t('topics.new_topic')}</CModalTitle>
        </CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            {LANGS.map((l) => (
              <div key={l}>
                <CFormLabel>{t('topics.name_in_language', { lang: t(`header.lang_${l}`) })}</CFormLabel>
                <CFormInput
                  value={form.names[l]}
                  onChange={(e) => setName(l, e.target.value)}
                  placeholder={t('topics.placeholder_in_language', { lang: t(`header.lang_${l}`) })}
                />
              </div>
            ))}
            <CFormCheck
              label={t('topics.active_label')}
              checked={form.active}
              onChange={(e) => setForm((f) => ({ ...f, active: e.target.checked }))}
            />
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving || !hasName}>
            {saving ? <CSpinner size="sm" /> : t('common.save')}
          </CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
