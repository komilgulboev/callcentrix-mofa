import React, { useState, useEffect, useCallback, useRef } from 'react'
import {
  CCard, CCardBody, CCardHeader, CBadge, CButton, CRow, CCol,
  CModal, CModalHeader, CModalTitle, CModalBody, CModalFooter,
  CForm, CFormLabel, CFormInput, CFormTextarea, CFormSelect,
  CSpinner, CNav, CNavItem, CNavLink, CTabContent, CTabPane,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import {
  cilPhone, cilMediaStop, cilMicrophone, cilVolumeOff,
  cilMediaPause, cilMediaPlay, cilPlus, cilExternalLink,
  cilReload, cilPencil, cilCommentSquare,
} from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import usePhoneStore from 'src/store/phone'
import useAuthStore from 'src/store/auth'
import { topics as topicsApi } from 'src/api'

const API_URL = import.meta.env.VITE_API_URL || window.location.origin

const STATUS_COLOR = {
  idle: 'secondary', connecting: 'warning', registered: 'success',
  ringing_in: 'warning', ringing_out: 'info', active: 'success',
  on_hold: 'warning', failed: 'danger',
}
const STATUS_LABEL_KEY = {
  idle: 'phone.status_idle', connecting: 'common.connecting', registered: 'phone.status_registered',
  ringing_in: 'phone.status_ringing_in', ringing_out: 'phone.status_ringing_out',
  active: 'phone.status_active', on_hold: 'phone.status_on_hold', failed: 'phone.status_failed',
}
const PRIORITY_COLOR      = { low: 'info', normal: 'secondary', high: 'warning', urgent: 'danger' }
const TICKET_STATUS_COLOR = { new: 'primary', open: 'warning', pending: 'info', resolved: 'success', closed: 'secondary' }

function fmtDuration(s) {
  const m = Math.floor(s / 60)
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

function getTopicName(topic, lang) {
  if (!topic?.names) return '—'
  return topic.names[lang] || topic.names.ru || topic.names.tj || topic.names.en || '—'
}

const KEYPAD = [
  ['1',''],['2','ABC'],['3','DEF'],
  ['4','GHI'],['5','JKL'],['6','MNO'],
  ['7','PQR'],['8','TUV'],['9','WXY'],
  ['*',''],['0','+'],['#',''],
]

function authHeaders() {
  return { 'Content-Type': 'application/json', Authorization: `Bearer ${localStorage.getItem('accessToken')}` }
}

// ─── Ticket Create Modal ──────────────────────────────────────
function TicketCreateModal({ visible, onClose, callerNo, calleeNo, onCreated }) {
  const { t } = useTranslation()
  const lang = localStorage.getItem('ui-lang') || 'ru'

  const [form, setForm]             = useState({ subject: '', body: '', priority: 'normal', status: 'new', topicId: '' })
  const [saving, setSaving]         = useState(false)
  const [error, setError]           = useState('')
  const [topicsList, setTopicsList] = useState([])
  const [topicsLoading, setTopicsLoading] = useState(false)

  useEffect(() => {
    if (!visible) return
    setForm({ subject: '', body: '', priority: 'normal', status: 'new', topicId: '' })
    setError('')
    setTopicsLoading(true)
    topicsApi.my()
      .then(d => setTopicsList(d.topics ?? []))
      .catch(() => setTopicsList([]))
      .finally(() => setTopicsLoading(false))
  }, [visible])

  const handleSubmit = async () => {
    if (!form.subject.trim()) { setError(t('phone.enter_subject')); return }
    setSaving(true); setError('')
    try {
      const payload = {
        subject: form.subject, body: form.body,
        callerNo, calleeNo,
        priority: form.priority, status: form.status,
      }
      if (form.topicId) payload.topicId = parseInt(form.topicId)
      const res = await fetch(`${API_URL}/api/tickets`, {
        method: 'POST', headers: authHeaders(),
        body: JSON.stringify(payload),
      })
      if (!res.ok) throw new Error(t('phone.save_error'))
      const data = await res.json()
      onCreated(data.id); onClose()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  return (
    <CModal visible={visible} onClose={onClose} size="lg">
      <CModalHeader><CModalTitle>{t('phone.create_ticket_from_call')}</CModalTitle></CModalHeader>
      <CModalBody>
        {error && <div className="alert alert-danger py-2">{error}</div>}
        <CForm>
          <CRow className="g-3">
            <CCol md={6}>
              <CFormLabel>{t('tickets.caller_number')}</CFormLabel>
              <CFormInput value={callerNo} readOnly className="bg-light" />
            </CCol>
            <CCol md={6}>
              <CFormLabel>{t('phone.extension')}</CFormLabel>
              <CFormInput value={calleeNo} readOnly className="bg-light" />
            </CCol>
            <CCol xs={12}>
              <CFormLabel>
                {t('phone.call_topic')}{' '}
                {topicsLoading && <CSpinner size="sm" className="ms-1" />}
              </CFormLabel>
              <CFormSelect
                value={form.topicId}
                onChange={(e) => setForm(f => ({ ...f, topicId: e.target.value }))}
                disabled={topicsLoading}
              >
                <option value="">{t('tickets.no_topic')}</option>
                {topicsList.map(t2 => (
                  <option key={t2.id} value={t2.id}>{getTopicName(t2, lang)}</option>
                ))}
              </CFormSelect>
              {!topicsLoading && topicsList.length === 0 && (
                <div className="text-muted small mt-1">{t('phone.topics_empty_hint')}</div>
              )}
            </CCol>
            <CCol xs={12}>
              <CFormLabel>{t('tickets.subject')} <span className="text-danger">*</span></CFormLabel>
              <CFormInput placeholder={t('tickets.subject_placeholder')} value={form.subject}
                onChange={(e) => setForm(f => ({ ...f, subject: e.target.value }))} autoFocus />
            </CCol>
            <CCol xs={12}>
              <CFormLabel>{t('tickets.description')}</CFormLabel>
              <CFormTextarea rows={3} placeholder={t('phone.call_details_placeholder')} value={form.body}
                onChange={(e) => setForm(f => ({ ...f, body: e.target.value }))} />
            </CCol>
            <CCol md={6}>
              <CFormLabel>{t('common.priority')}</CFormLabel>
              <CFormSelect value={form.priority} onChange={(e) => setForm(f => ({ ...f, priority: e.target.value }))}>
                <option value="low">{t('tickets.priority_low')}</option>
                <option value="normal">{t('tickets.priority_normal')}</option>
                <option value="high">{t('tickets.priority_high')}</option>
                <option value="urgent">{t('tickets.priority_urgent')}</option>
              </CFormSelect>
            </CCol>
            <CCol md={6}>
              <CFormLabel>{t('common.status')}</CFormLabel>
              <CFormSelect value={form.status} onChange={(e) => setForm(f => ({ ...f, status: e.target.value }))}>
                <option value="new">{t('tickets.status_new')}</option>
                <option value="open">{t('tickets.status_open')}</option>
                <option value="pending">{t('tickets.status_pending')}</option>
              </CFormSelect>
            </CCol>
          </CRow>
        </CForm>
      </CModalBody>
      <CModalFooter>
        <CButton color="secondary" onClick={onClose} disabled={saving}>{t('common.cancel')}</CButton>
        <CButton color="primary" onClick={handleSubmit} disabled={saving}>
          {saving ? <CSpinner size="sm" className="me-2" /> : <CIcon icon={cilPlus} className="me-2" />}
          {t('tickets.create_ticket')}
        </CButton>
      </CModalFooter>
    </CModal>
  )
}

// ─── Ticket Edit Modal ────────────────────────────────────────
function TicketEditModal({ ticketId, visible, onClose, onSaved }) {
  const { t } = useTranslation()
  const [activeTab, setActiveTab] = useState('edit')
  const [ticket, setTicket]       = useState(null)
  const [comments, setComments]   = useState([])
  const [loading, setLoading]     = useState(false)
  const [saving, setSaving]       = useState(false)
  const [error, setError]         = useState('')
  const [commentText, setCommentText] = useState('')
  const [sendingComment, setSendingComment] = useState(false)
  const [form, setForm] = useState({ subject: '', body: '', status: 'new', priority: 'normal' })

  useEffect(() => {
    if (!visible || !ticketId) return
    setLoading(true); setError(''); setActiveTab('edit'); setCommentText('')
    Promise.all([
      fetch(`${API_URL}/api/tickets/${ticketId}`, { headers: authHeaders() }).then(r => r.json()),
      fetch(`${API_URL}/api/tickets/${ticketId}/comments`, { headers: authHeaders() }).then(r => r.json()),
    ]).then(([ticketData, c]) => {
      setTicket(ticketData)
      setForm({ subject: ticketData.subject, body: ticketData.body, status: ticketData.status, priority: ticketData.priority })
      setComments(c.comments || [])
    }).catch(() => setError(t('phone.loading_error')))
    .finally(() => setLoading(false))
  }, [ticketId, visible])

  const handleSave = async () => {
    if (!form.subject.trim()) { setError(t('phone.enter_subject')); return }
    setSaving(true); setError('')
    try {
      const res = await fetch(`${API_URL}/api/tickets/${ticketId}`, {
        method: 'PUT', headers: authHeaders(),
        body: JSON.stringify({ subject: form.subject, body: form.body, callerNo: ticket?.callerNo || '', status: form.status, priority: form.priority }),
      })
      if (!res.ok) throw new Error(t('phone.save_error'))
      onSaved(); onClose()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleAddComment = async () => {
    if (!commentText.trim()) return
    setSendingComment(true)
    try {
      const res = await fetch(`${API_URL}/api/tickets/${ticketId}/comments`, {
        method: 'POST', headers: authHeaders(),
        body: JSON.stringify({ text: commentText }),
      })
      if (!res.ok) throw new Error(t('phone.save_error'))
      // Перезагружаем комментарии
      const c = await fetch(`${API_URL}/api/tickets/${ticketId}/comments`, { headers: authHeaders() }).then(r => r.json())
      setComments(c.comments || [])
      setCommentText('')
    } catch (e) { setError(e.message) }
    finally { setSendingComment(false) }
  }

  return (
    <CModal visible={visible} onClose={onClose} size="lg">
      <CModalHeader>
        <CModalTitle>
          {t('phone.ticket_hash', { id: ticketId })}
          {ticket && (
            <CBadge color={TICKET_STATUS_COLOR[ticket.status] || 'secondary'} className="ms-2">
              {ticket.status}
            </CBadge>
          )}
        </CModalTitle>
      </CModalHeader>
      <CModalBody>
        {error && <div className="alert alert-danger py-2 mb-3">{error}</div>}
        {loading ? (
          <div className="text-center py-4"><CSpinner /></div>
        ) : (
          <>
            <CNav variant="tabs" className="mb-3">
              <CNavItem>
                <CNavLink active={activeTab === 'edit'} onClick={() => setActiveTab('edit')} style={{ cursor: 'pointer' }}>
                  <CIcon icon={cilPencil} className="me-1" />{t('common.edit')}
                </CNavLink>
              </CNavItem>
              <CNavItem>
                <CNavLink active={activeTab === 'comments'} onClick={() => setActiveTab('comments')} style={{ cursor: 'pointer' }}>
                  <CIcon icon={cilCommentSquare} className="me-1" />
                  {t('ticket_detail.comments')}
                  {comments.length > 0 && <CBadge color="secondary" className="ms-1">{comments.length}</CBadge>}
                </CNavLink>
              </CNavItem>
            </CNav>

            <CTabContent>
              {/* ── Вкладка редактирования ── */}
              <CTabPane visible={activeTab === 'edit'}>
                <CForm>
                  <CRow className="g-3">
                    {ticket && (
                      <>
                        <CCol md={6}>
                          <CFormLabel className="text-muted small">{t('tickets.caller_number')}</CFormLabel>
                          <CFormInput value={ticket.callerNo || '—'} readOnly className="bg-light" />
                        </CCol>
                        <CCol md={6}>
                          <CFormLabel className="text-muted small">{t('phone.destination_number')}</CFormLabel>
                          <CFormInput value={ticket.calleeNo || '—'} readOnly className="bg-light" />
                        </CCol>
                      </>
                    )}
                    <CCol xs={12}>
                      <CFormLabel>{t('tickets.subject')} <span className="text-danger">*</span></CFormLabel>
                      <CFormInput value={form.subject}
                        onChange={(e) => setForm(f => ({ ...f, subject: e.target.value }))} />
                    </CCol>
                    <CCol xs={12}>
                      <CFormLabel>{t('tickets.description')}</CFormLabel>
                      <CFormTextarea rows={4} value={form.body}
                        onChange={(e) => setForm(f => ({ ...f, body: e.target.value }))} />
                    </CCol>
                    <CCol md={6}>
                      <CFormLabel>{t('common.status')}</CFormLabel>
                      <CFormSelect value={form.status} onChange={(e) => setForm(f => ({ ...f, status: e.target.value }))}>
                        <option value="new">{t('tickets.status_new')}</option>
                        <option value="open">{t('tickets.status_open')}</option>
                        <option value="pending">{t('tickets.status_pending')}</option>
                        <option value="resolved">{t('tickets.status_resolved')}</option>
                        <option value="closed">{t('tickets.status_closed')}</option>
                      </CFormSelect>
                    </CCol>
                    <CCol md={6}>
                      <CFormLabel>{t('common.priority')}</CFormLabel>
                      <CFormSelect value={form.priority} onChange={(e) => setForm(f => ({ ...f, priority: e.target.value }))}>
                        <option value="low">{t('tickets.priority_low')}</option>
                        <option value="normal">{t('tickets.priority_normal')}</option>
                        <option value="high">{t('tickets.priority_high')}</option>
                        <option value="urgent">{t('tickets.priority_urgent')}</option>
                      </CFormSelect>
                    </CCol>
                  </CRow>
                </CForm>
              </CTabPane>

              {/* ── Вкладка комментариев ── */}
              <CTabPane visible={activeTab === 'comments'}>
                <div style={{ maxHeight: 300, overflowY: 'auto', marginBottom: 16 }}>
                  {comments.length === 0 ? (
                    <div className="text-center text-muted py-4">{t('ticket_detail.no_comments')}</div>
                  ) : (
                    comments.map(c => (
                      <div key={c.id} className="mb-3 p-3 rounded" style={{ background: 'var(--cui-tertiary-bg, #f8f9fa)' }}>
                        <div className="d-flex justify-content-between mb-1">
                          <strong className="small">{c.username}</strong>
                          <span className="text-muted small">{new Date(c.createdAt).toLocaleString()}</span>
                        </div>
                        <div style={{ whiteSpace: 'pre-wrap' }}>{c.text}</div>
                      </div>
                    ))
                  )}
                </div>
                <div className="border-top pt-3">
                  <CFormLabel>{t('phone.add_comment_label')}</CFormLabel>
                  <CFormTextarea
                    rows={3}
                    placeholder={t('ticket_detail.write_comment')}
                    value={commentText}
                    onChange={(e) => setCommentText(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && e.ctrlKey) handleAddComment()
                    }}
                  />
                  <div className="d-flex justify-content-between align-items-center mt-2">
                    <span className="text-muted small">{t('phone.ctrl_enter_hint')}</span>
                    <CButton color="primary" size="sm" onClick={handleAddComment}
                      disabled={!commentText.trim() || sendingComment}>
                      {sendingComment ? <CSpinner size="sm" className="me-1" /> : <CIcon icon={cilCommentSquare} className="me-1" />}
                      {t('common.send')}
                    </CButton>
                  </div>
                </div>
              </CTabPane>
            </CTabContent>
          </>
        )}
      </CModalBody>
      <CModalFooter>
        <CButton color="secondary" onClick={onClose}>{t('common.close')}</CButton>
        {activeTab === 'edit' && (
          <CButton color="primary" onClick={handleSave} disabled={saving || loading}>
            {saving ? <CSpinner size="sm" className="me-2" /> : <CIcon icon={cilPencil} className="me-2" />}
            {t('phone.save_changes')}
          </CButton>
        )}
        <a href={`/#/tickets/${ticketId}`} target="_blank" rel="noreferrer">
          <CButton color="light">
            <CIcon icon={cilExternalLink} className="me-1" />{t('phone.open_full')}
          </CButton>
        </a>
      </CModalFooter>
    </CModal>
  )
}

// ─── Main Phone Component ─────────────────────────────────────
export default function Phone() {
  const { t } = useTranslation()
  const [dial, setDial]                     = useState('')
  const [showCreateModal, setShowCreateModal] = useState(false)
  const [editTicketId, setEditTicketId]     = useState(null)
  const [callerTickets, setCallerTickets]   = useState([])
  const [ticketsLoading, setTicketsLoading] = useState(false)
  const [activeCallerNo, setActiveCallerNo] = useState('')
  const [cdrToday, setCdrToday]             = useState([])
  const [cdrLoading, setCdrLoading]         = useState(false)
  const [ticketRefresh, setTicketRefresh]   = useState(0)

  const user = useAuthStore(s => s.user)
  const { status, session, remoteNumber, callDuration, isMuted, call, answer, hangup, toggleMute, toggleHold, sendDtmf } = usePhoneStore()

  const inCall          = ['ringing_in','ringing_out','active','on_hold'].includes(status)
  const canCreateTicket = ['ringing_in','active','on_hold'].includes(status)
  // No live JsSIP session means this call was rehydrated after a page reload —
  // WebRTC audio doesn't survive that, only the display + AMI-backed hangup do.
  const reattached      = !session && ['active','on_hold'].includes(status)

  useEffect(() => {
    if (remoteNumber) setActiveCallerNo(remoteNumber)
  }, [remoteNumber])

  useEffect(() => {
    if (!activeCallerNo) return
    setTicketsLoading(true)
    fetch(`${API_URL}/api/tickets?caller_no=${encodeURIComponent(activeCallerNo)}`, { headers: authHeaders() })
      .then(r => r.json())
      .then(d => setCallerTickets(d.tickets || []))
      .catch(() => {})
      .finally(() => setTicketsLoading(false))
  }, [activeCallerNo, ticketRefresh])

  const loadCDR = useCallback(() => {
    setCdrLoading(true)
    const today = new Date().toISOString().slice(0, 10)
    fetch(`${API_URL}/api/cdr?date_from=${today}&limit=500`, { headers: authHeaders() })
      .then(r => r.json())
      .then(d => setCdrToday(d.records || []))
      .catch(() => {})
      .finally(() => setCdrLoading(false))
  }, [])

  useEffect(() => { loadCDR() }, [])

  const prevStatus = useRef(status)
  useEffect(() => {
    if (prevStatus.current !== 'registered' && status === 'registered') loadCDR()
    prevStatus.current = status
  }, [status])

  const handleKey = (key) => {
    setDial(d => d + key)
    if (status === 'active') sendDtmf(key)
  }
  const handleDial = () => {
    if (!dial.trim()) return
    call(dial.trim()); setDial('')
  }

  return (
    <div>
      <div className="d-flex align-items-center gap-3 mb-4">
        <h4 className="mb-0">{t('nav.webphone')}</h4>
        <CBadge color={STATUS_COLOR[status] ?? 'secondary'} className="px-3 py-2">
          {t(STATUS_LABEL_KEY[status] ?? status)}
        </CBadge>
        {user && <span className="text-muted small ms-auto">{t('phone.extension')}: <strong>{user.username}</strong></span>}
      </div>

      <CRow className="g-3">

        {/* ── Левая колонка: диалпад ── */}
        <CCol lg={4} xl={3}>

          {status === 'ringing_in' && (
            <CCard className="border-warning mb-3">
              <CCardBody className="text-center py-4">
                <div style={{ fontSize: 48 }}>📲</div>
                <div className="text-muted mt-1">{t('phone.status_ringing_in')}</div>
                <div className="fs-2 fw-bold my-2">{remoteNumber}</div>
                <div className="d-flex gap-3 justify-content-center mt-3">
                  <CButton color="success" size="lg" onClick={answer} className="px-4">
                    <CIcon icon={cilPhone} className="me-2" />{t('phone.answer')}
                  </CButton>
                  <CButton color="danger" size="lg" onClick={hangup} className="px-4">
                    <CIcon icon={cilMediaStop} className="me-2" />{t('phone.decline')}
                  </CButton>
                </div>
              </CCardBody>
            </CCard>
          )}

          {['ringing_out','active','on_hold'].includes(status) && (
            <CCard className={`mb-3 border-${STATUS_COLOR[status]}`}>
              <CCardBody className="text-center py-3">
                <div className="text-muted small">
                  {status === 'ringing_out' ? `⏳ ${t('phone.status_ringing_out')}` : status === 'on_hold' ? `⏸ ${t('phone.status_on_hold')}` : `🔊 ${t('phone.status_active')}`}
                </div>
                <div className="fs-2 fw-bold my-2">{remoteNumber}</div>
                {status === 'active' && <div className="fs-4 text-muted mb-3">{fmtDuration(callDuration)}</div>}
                {reattached && (
                  <div className="text-warning small mb-3">
                    {t('phone.reattached_notice')}
                  </div>
                )}
                <div className="d-flex gap-2 justify-content-center flex-wrap">
                  <CButton color={isMuted ? 'warning' : 'light'} onClick={toggleMute} className="px-3" disabled={reattached}>
                    <CIcon icon={isMuted ? cilVolumeOff : cilMicrophone} />
                    <span className="ms-1 small">{isMuted ? t('phone.unmute') : t('phone.mute')}</span>
                  </CButton>
                  <CButton color={status === 'on_hold' ? 'info' : 'light'} onClick={toggleHold} className="px-3" disabled={reattached}>
                    <CIcon icon={status === 'on_hold' ? cilMediaPlay : cilMediaPause} />
                    <span className="ms-1 small">{status === 'on_hold' ? t('phone.resume') : t('phone.hold')}</span>
                  </CButton>
                  <CButton color="danger" onClick={hangup} className="px-3">
                    <CIcon icon={cilMediaStop} />
                    <span className="ms-1 small">{t('phone.end_call')}</span>
                  </CButton>
                </div>
              </CCardBody>
            </CCard>
          )}

          {canCreateTicket && (
            <CButton color="primary" className="w-100 mb-3" onClick={() => setShowCreateModal(true)}>
              <CIcon icon={cilPlus} className="me-2" />{t('tickets.create_ticket')}
            </CButton>
          )}

          <CCard>
            <CCardHeader>{t('phone.dial_number')}</CCardHeader>
            <CCardBody>
              <div className="text-muted small mb-2">{t('phone.dial_9_hint')}</div>
              <div className="input-group mb-3">
                <input
                  className="form-control text-center fs-5 fw-bold"
                  value={inCall ? remoteNumber : dial}
                  onChange={(e) => !inCall && setDial(e.target.value)}
                  onKeyDown={(e) => e.key === 'Enter' && !inCall && handleDial()}
                  placeholder={t('phone.enter_number_placeholder')}
                  readOnly={inCall}
                />
                {!inCall && dial && (
                  <CButton color="light" onClick={() => setDial(d => d.slice(0,-1))}>⌫</CButton>
                )}
              </div>
              <div style={{ display:'grid', gridTemplateColumns:'repeat(3,1fr)', gap:8 }}>
                {KEYPAD.map(([key, sub]) => (
                  <CButton key={key} color="light" onClick={() => handleKey(key)} className="py-3"
                    style={{ display:'flex', flexDirection:'column', alignItems:'center' }}>
                    <span className="fw-bold fs-5 lh-1">{key}</span>
                    {sub && <span style={{ fontSize:9, opacity:0.5, letterSpacing:1 }}>{sub}</span>}
                  </CButton>
                ))}
              </div>
              <CButton color="success" className="w-100 mt-3" size="lg"
                onClick={handleDial} disabled={!dial.trim() || inCall}>
                <CIcon icon={cilPhone} className="me-2" />{t('phone.call_button')}
              </CButton>
            </CCardBody>
          </CCard>
        </CCol>

        {/* ── Правая колонка: тикеты абонента ── */}
        <CCol lg={8} xl={9}>
          <CCard style={{ height: '100%' }}>
            <CCardHeader className="d-flex align-items-center justify-content-between">
              <span>
                {t('phone.caller_tickets')}
                {activeCallerNo && <strong className="ms-2 text-primary">{activeCallerNo}</strong>}
              </span>
              <div className="d-flex align-items-center gap-2">
                <CBadge color={callerTickets.length > 0 ? 'warning' : 'secondary'}>{callerTickets.length}</CBadge>
                {canCreateTicket && (
                  <CButton size="sm" color="primary" onClick={() => setShowCreateModal(true)}>
                    <CIcon icon={cilPlus} className="me-1" />{t('tickets.new_ticket')}
                  </CButton>
                )}
                {activeCallerNo && (
                  <CButton size="sm" color="light" onClick={() => setTicketRefresh(k => k+1)}
                    disabled={ticketsLoading} title={t('phone.refresh')}>
                    <CIcon icon={cilReload} />
                  </CButton>
                )}
              </div>
            </CCardHeader>
            <CCardBody className="p-0" style={{ minHeight: 220, maxHeight: 420, overflowY: 'auto' }}>
              {!activeCallerNo ? (
                <div className="text-center text-muted py-5">
                  <div style={{ fontSize: 40 }}>📞</div>
                  <div className="mt-2">{t('phone.tickets_will_appear')}</div>
                  <div className="small mt-1">{t('phone.or_click_icon_below')}</div>
                </div>
              ) : ticketsLoading ? (
                <div className="text-center py-5"><CSpinner size="sm" /></div>
              ) : callerTickets.length === 0 ? (
                <div className="text-center text-muted py-5">
                  <div style={{ fontSize: 36 }}>📋</div>
                  <div className="mt-2">{t('phone.no_tickets_for', { number: activeCallerNo })}</div>
                  {canCreateTicket && (
                    <CButton color="primary" size="sm" className="mt-3" onClick={() => setShowCreateModal(true)}>
                      <CIcon icon={cilPlus} className="me-1" />{t('phone.create_first_ticket')}
                    </CButton>
                  )}
                </div>
              ) : (
                <table className="table table-hover mb-0">
                  <thead className="table-light">
                    <tr>
                      <th>#</th>
                      <th>{t('tickets.col_subject')}</th>
                      <th>{t('phone.call_topic')}</th>
                      <th>{t('common.status')}</th>
                      <th>{t('common.priority')}</th>
                      <th>{t('tickets.col_created')}</th>
                      <th>{t('common.actions')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {callerTickets.map(t3 => (
                      <tr key={t3.id}>
                        <td className="text-muted">{t3.id}</td>
                        <td style={{ maxWidth:200, overflow:'hidden', textOverflow:'ellipsis', whiteSpace:'nowrap' }}>
                          {t3.subject}
                        </td>
                        <td className="text-muted small">
                          {t3.topic ? getTopicName(t3.topic, localStorage.getItem('ui-lang') || 'ru') : '—'}
                        </td>
                        <td><CBadge color={TICKET_STATUS_COLOR[t3.status] || 'secondary'}>{t3.status}</CBadge></td>
                        <td><CBadge color={PRIORITY_COLOR[t3.priority] || 'secondary'}>{t3.priority}</CBadge></td>
                        <td className="text-muted small">{new Date(t3.createdAt).toLocaleString()}</td>
                        <td>
                          <div className="d-flex gap-1">
                            <CButton size="sm" color="light" title={t('phone.edit_comment_title')}
                              onClick={() => setEditTicketId(t3.id)}>
                              <CIcon icon={cilPencil} size="sm" />
                            </CButton>
                            <a href={`/#/tickets/${t3.id}`} target="_blank" rel="noreferrer">
                              <CButton size="sm" color="light" title={t('phone.open_new_tab')}>
                                <CIcon icon={cilExternalLink} size="sm" />
                              </CButton>
                            </a>
                          </div>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              )}
            </CCardBody>
          </CCard>
        </CCol>
      </CRow>

      {/* ── Звонки за сегодня ── */}
      <CRow className="mt-3">
        <CCol xs={12}>
          <CCard>
            <CCardHeader className="d-flex align-items-center justify-content-between">
              <span>{t('dashboard.todays_calls')}</span>
              <div className="d-flex align-items-center gap-2">
                <CBadge color="secondary">{cdrToday.length}</CBadge>
                <CButton size="sm" color="light" onClick={loadCDR} disabled={cdrLoading} title={t('phone.refresh')}>
                  {cdrLoading ? <CSpinner size="sm" /> : <CIcon icon={cilReload} />}
                </CButton>
              </div>
            </CCardHeader>
            <CCardBody className="p-0" style={{ maxHeight: 420, overflowY: 'auto' }}>
              {cdrLoading && cdrToday.length === 0 ? (
                <div className="text-center py-4"><CSpinner size="sm" /></div>
              ) : cdrToday.length === 0 ? (
                <div className="text-center text-muted py-5">
                  <div style={{ fontSize: 36 }}>📋</div>
                  <div className="mt-2">{t('phone.no_calls_today')}</div>
                </div>
              ) : (
                <table className="table table-hover table-sm mb-0">
                  <thead className="table-light">
                    <tr>
                      <th>{t('phone.col_type')}</th>
                      <th>{t('cdr.col_from')}</th>
                      <th>{t('cdr.col_to')}</th>
                      <th>{t('phone.col_time')}</th>
                      <th>{t('phone.col_talk')}</th>
                      <th>{t('phone.col_total_duration')}</th>
                      <th>{t('common.status')}</th>
                      <th>{t('nav.tickets')}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {cdrToday.map(c => {
                      const isOutbound = c.src === user?.username
                      const callerNum  = isOutbound ? c.dst : c.src
                      return (
                        <tr key={c.id}>
                          <td>
                            <CBadge color={isOutbound ? 'info' : 'success'}>
                              {isOutbound ? t('phone.outbound_short') : t('phone.inbound_short')}
                            </CBadge>
                          </td>
                          <td className="fw-semibold text-primary" style={{ cursor:'pointer' }}
                            onClick={() => !inCall && setDial(callerNum)} title={t('phone.click_to_redial')}>
                            {c.src}
                          </td>
                          <td className="text-muted">{c.dst}</td>
                          <td className="text-muted">{new Date(c.callDate).toLocaleTimeString()}</td>
                          <td>{c.billsec > 0 ? fmtDuration(c.billsec) : '—'}</td>
                          <td>{c.duration > 0 ? fmtDuration(c.duration) : '—'}</td>
                          <td>
                            <CBadge color={c.disposition === 'ANSWERED' ? 'success' : 'danger'}>
                              {c.disposition === 'ANSWERED' ? t('cdr.disposition_answered') : t('phone.missed')}
                            </CBadge>
                          </td>
                          <td>
                            <CButton size="sm" color="light"
                              onClick={() => setActiveCallerNo(callerNum)}
                              title={t('phone.show_caller_tickets')}>
                              📋
                            </CButton>
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              )}
            </CCardBody>
          </CCard>
        </CCol>
      </CRow>

      {/* Модалки */}
      <TicketCreateModal
        visible={showCreateModal}
        onClose={() => setShowCreateModal(false)}
        callerNo={activeCallerNo}
        calleeNo={user?.username || ''}
        onCreated={() => setTicketRefresh(k => k+1)}
      />
      <TicketEditModal
        ticketId={editTicketId}
        visible={!!editTicketId}
        onClose={() => setEditTicketId(null)}
        onSaved={() => { setTicketRefresh(k => k+1); setEditTicketId(null) }}
      />
    </div>
  )
}
