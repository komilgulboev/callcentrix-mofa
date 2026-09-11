import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  CButton, CCard, CCardBody, CModal, CModalBody, CModalFooter,
  CModalHeader, CModalTitle, CForm, CFormInput, CFormLabel,
  CFormSelect, CTable, CTableBody, CTableDataCell, CTableHead,
  CTableHeaderCell, CTableRow, CBadge, CAlert, CSpinner,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilPencil, cilTrash, cilCheckCircle, cilBan, cilPhone, cilUserFollow } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { users as usersApi } from 'src/api'
import useAuthStore from 'src/store/auth'

const ROLE_COLORS = ['danger', 'primary', 'warning', 'info']
const EMPTY = { username: '', password: '', userType: '3', sipNo: '', firstName: '', lastName: '', telegramChatId: '', email: '' }

export default function Users() {
  const { t } = useTranslation()
  const [rows,           setRows]           = useState([])
  const [loading,        setLoading]        = useState(true)
  const [error,          setError]          = useState('')
  const [modal,          setModal]          = useState(false)
  const [editing,        setEditing]        = useState(null)
  const [form,           setForm]           = useState(EMPTY)
  const [saving,         setSaving]         = useState(false)
  const [sipExistsWarn,  setSipExistsWarn]  = useState(false)
  const [linkCode,       setLinkCode]       = useState(null)
  const [linkLoading,    setLinkLoading]    = useState(false)
  const [linkError,      setLinkError]      = useState('')
  const currentUser = useAuthStore((s) => s.user)
  const navigate = useNavigate()

  const load = () => {
    setLoading(true)
    usersApi.list()
      .then((d) => setRows(d.users ?? d))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const openCreate = () => {
    setEditing(null); setForm(EMPTY); setSipExistsWarn(false)
    setLinkCode(null); setLinkError('')
    setModal(true)
  }
  const openEdit   = (u) => {
    setEditing(u)
    setForm({
      username: u.username, password: '',
      userType: String(u.userType),
      sipNo: u.sipNo ?? '',
      firstName: u.firstName ?? '',
      lastName:  u.lastName ?? '',
      telegramChatId: u.telegramChatId ?? '',
      email: u.email ?? '',
    })
    setLinkCode(null); setLinkError('')
    setModal(true)
  }

  // Self-service Telegram linking: generates a one-time code (see
  // GenerateTelegramLinkCode), then polls this user's own record until
  // telegramChatId shows up — set by HandleTelegramMessage once they send
  // the code to the bot themselves. Reading it straight from Telegram this
  // way is what keeps every user's chat id correctly their own, instead of
  // an admin having to type in a number nobody can verify.
  const handleGetLinkCode = async () => {
    if (!editing) return
    setLinkLoading(true)
    setLinkError('')
    try {
      const d = await usersApi.telegramLinkCode(editing.id)
      setLinkCode(d)
    } catch (e) {
      setLinkError(e.message)
    } finally {
      setLinkLoading(false)
    }
  }

  useEffect(() => {
    if (!linkCode || !editing) return
    const timer = setInterval(async () => {
      try {
        const u = await usersApi.get(editing.id)
        if (u.telegramChatId) {
          setForm((f) => ({ ...f, telegramChatId: u.telegramChatId }))
          setLinkCode(null)
        }
      } catch { /* keep polling silently */ }
    }, 3000)
    return () => clearInterval(timer)
  }, [linkCode, editing])

  const handleSave = async () => {
    setSaving(true)
    setSipExistsWarn(false)
    try {
      const payload = { ...form, userType: Number(form.userType) }
      if (!payload.password) delete payload.password
      if (!payload.sipNo) payload.sipNo = payload.username
      if (editing) await usersApi.update(editing.id, payload)
      else         await usersApi.create(payload)
      setModal(false)
      load()
    } catch (e) {
      if (e.message === 'sip_exists') {
        setSipExistsWarn(true)
      } else if (e.message === 'username_exists') {
        setError(t('users.username_exists'))
      } else if (e.message === 'telegram_chat_id_taken') {
        setError(t('users.telegram_chat_id_taken'))
      } else {
        setError(e.message)
      }
    }
    finally { setSaving(false) }
  }

  const handleToggle = async (u) => {
    try {
      if (u.active) await usersApi.deactivate(u.id)
      else          await usersApi.activate(u.id)
      load()
    } catch (e) { setError(e.message) }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('users.delete_confirm'))) return
    try { await usersApi.remove(id); load() }
    catch (e) { setError(e.message) }
  }

  const availableRoles = currentUser?.userType === 0
    ? [
        { value: '0', label: t('roles.0') },
        { value: '1', label: t('roles.1') },
        { value: '2', label: t('roles.2') },
        { value: '3', label: t('roles.3') },
      ]
    : [
        { value: '2', label: t('roles.2') },
        { value: '3', label: t('roles.3') },
      ]

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <div>
          <h4 className="mb-0">{t('users.title')}</h4>
          <small className="text-muted">{t('users.subtitle')}</small>
        </div>
        <CButton color="primary" onClick={openCreate}>
          <CIcon icon={cilPlus} className="me-2" />{t('users.new_user')}
        </CButton>
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
                  <CTableHeaderCell>{t('users.col_username')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('users.col_name')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('users.col_role')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('users.col_status')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('users.col_sip')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('users.col_server')}</CTableHeaderCell>
                  <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map((u) => (
                  <CTableRow key={u.id}>
                    <CTableDataCell>
                      <div className="fw-semibold">{u.username}</div>
                      <div className="text-muted small d-flex align-items-center gap-1">
                        <CIcon icon={cilPhone} size="sm" />
                        {u.sipNo || u.username}
                      </div>
                    </CTableDataCell>
                    <CTableDataCell>
                      {[u.firstName, u.lastName].filter(Boolean).join(' ') || '—'}
                    </CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={ROLE_COLORS[u.userType] ?? 'secondary'}>
                        {t(`roles.${u.userType}`, { defaultValue: String(u.userType) })}
                      </CBadge>
                    </CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={u.active ? 'success' : 'secondary'}>
                        {u.active ? t('users.active') : t('users.inactive')}
                      </CBadge>
                    </CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={u.active ? 'success' : 'light'} textColor={u.active ? undefined : 'muted'}>
                        {u.active ? t('users.registered') : t('users.no_sip')}
                      </CBadge>
                    </CTableDataCell>
                    <CTableDataCell className="text-muted">{u.serverName || '—'}</CTableDataCell>
                    <CTableDataCell className="text-end">
                      <div className="d-flex gap-1 justify-content-end">
                        <CButton size="sm" color="light" onClick={() => openEdit(u)}>
                          <CIcon icon={cilPencil} />
                        </CButton>
                        <CButton
                          size="sm"
                          color={u.active ? 'warning' : 'success'}
                          onClick={() => handleToggle(u)}
                        >
                          <CIcon icon={u.active ? cilBan : cilCheckCircle} />
                        </CButton>
                        <CButton size="sm" color="danger" onClick={() => handleDelete(u.id)}>
                          <CIcon icon={cilTrash} />
                        </CButton>
                      </div>
                    </CTableDataCell>
                  </CTableRow>
                ))}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={7} className="text-center text-muted py-4">
                      {t('users.empty')}
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
          <CModalTitle>{editing ? t('users.edit_user') : t('users.new_user')}</CModalTitle>
        </CModalHeader>
        <CModalBody>
          {sipExistsWarn && (
            <CAlert color="warning" className="mb-3">
              <div className="d-flex gap-2 align-items-start">
                <CIcon icon={cilUserFollow} className="mt-1 flex-shrink-0" />
                <div>
                  <strong>{t('users.sip_exists_title')}</strong>
                  <div className="mt-1 small">{t('users.sip_exists_body')}</div>
                  <CButton
                    size="sm" color="warning" className="mt-2"
                    onClick={() => { setModal(false); navigate('/tenant-users') }}
                  >
                    <CIcon icon={cilUserFollow} className="me-1" />
                    {t('users.go_to_assignment')}
                  </CButton>
                </div>
              </div>
            </CAlert>
          )}
          <CAlert color="info" className="small py-2">
            {t('users.sip_info')}
          </CAlert>
          <CForm className="d-flex flex-column gap-3 mt-2">
            <div className="row g-2">
              <div className="col">
                <CFormLabel>{t('common.first_name')}</CFormLabel>
                <CFormInput value={form.firstName}
                  onChange={(e) => setForm({ ...form, firstName: e.target.value })} />
              </div>
              <div className="col">
                <CFormLabel>{t('common.last_name')}</CFormLabel>
                <CFormInput value={form.lastName}
                  onChange={(e) => setForm({ ...form, lastName: e.target.value })} />
              </div>
            </div>
            <div>
              <CFormLabel>{t('users.username_label')}</CFormLabel>
              <CFormInput
                value={form.username}
                onChange={(e) => setForm({ ...form, username: e.target.value })}
                placeholder="1001"
              />
            </div>
            <div>
              <CFormLabel>{editing ? t('users.new_password') : t('common.password')}</CFormLabel>
              <CFormInput
                type="password"
                value={form.password}
                onChange={(e) => setForm({ ...form, password: e.target.value })}
                placeholder={editing ? '••••••' : t('users.enter_password')}
              />
            </div>
            <div>
              <CFormLabel>{t('common.role')}</CFormLabel>
              <CFormSelect value={form.userType}
                onChange={(e) => setForm({ ...form, userType: e.target.value })}>
                {availableRoles.map((r) => (
                  <option key={r.value} value={r.value}>{r.label}</option>
                ))}
              </CFormSelect>
            </div>
            <div>
              <CFormLabel>{t('users.email_label')}</CFormLabel>
              <CFormInput type="email" value={form.email}
                onChange={(e) => setForm({ ...form, email: e.target.value })}
                placeholder="name@example.com" />
              <div className="form-text">{t('users.email_hint')}</div>
            </div>
            <div>
              <CFormLabel>{t('users.telegram_chat_id_label')}</CFormLabel>
              <CFormInput value={form.telegramChatId}
                onChange={(e) => setForm({ ...form, telegramChatId: e.target.value })}
                placeholder="123456789" />
              <div className="form-text">{t('users.telegram_chat_id_hint')}</div>
              {editing && (
                <div className="mt-2">
                  <CButton size="sm" color="info" variant="outline" onClick={handleGetLinkCode} disabled={linkLoading}>
                    {linkLoading ? <CSpinner size="sm" className="me-1" /> : null}
                    {linkLoading ? t('users.telegram_link_generating') : t('users.telegram_link_button')}
                  </CButton>
                  {linkError && <div className="text-danger small mt-1">{linkError}</div>}
                  {linkCode && (
                    <CAlert color="info" className="small mt-2 mb-0">
                      <div>{t('users.telegram_link_instructions')}</div>
                      <div className="fw-bold font-monospace my-1">/start {linkCode.code}</div>
                      {linkCode.botUsername && (
                        <a href={`https://t.me/${linkCode.botUsername}?start=${linkCode.code}`}
                          target="_blank" rel="noreferrer">
                          {t('users.telegram_link_open_bot')}
                        </a>
                      )}
                      <div className="text-muted mt-1">
                        {t('users.telegram_link_expires', { time: new Date(linkCode.expiresAt).toLocaleTimeString() })}
                      </div>
                    </CAlert>
                  )}
                </div>
              )}
            </div>
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave}
            disabled={saving || !form.username || (!editing && !form.password)}>
            {saving ? <CSpinner size="sm" /> : editing ? t('common.save') : t('common.create')}
          </CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
