import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import {
  CAlert, CButton, CCard, CCardBody, CCardHeader,
  CBadge, CFormSelect, CFormTextarea, CFormCheck, CSpinner,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilArrowLeft, cilSend } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { tickets as ticketsApi } from 'src/api'
import { siteName } from 'src/views/sites/Sites'
import useAuthStore from 'src/store/auth'

const STATUS_COLOR = { new: 'primary', open: 'warning', pending: 'info', resolved: 'success', closed: 'secondary' }
const STATUSES     = ['new', 'open', 'pending', 'resolved', 'closed']

export default function TicketDetail() {
  const { id }   = useParams()
  const navigate = useNavigate()
  const user     = useAuthStore((s) => s.user)
  const { t, i18n } = useTranslation()
  const lang     = i18n.language

  const [ticket,   setTicket]   = useState(null)
  const [comments, setComments] = useState([])
  const [history,  setHistory]  = useState([])
  const [loading,  setLoading]  = useState(true)
  const [error,    setError]    = useState('')
  const [comment,  setComment]  = useState('')
  const [sending,  setSending]  = useState(false)
  const [status,   setStatus]   = useState('')
  const [assigneeIds, setAssigneeIds] = useState([])
  const [primaryId,   setPrimaryId]   = useState('')
  const [assignableUsers, setAssignableUsers] = useState([])
  const [assigning, setAssigning] = useState(false)

  const load = () => {
    Promise.all([ticketsApi.get(id), ticketsApi.comments(id), ticketsApi.history(id)])
      .then(([tk, c, h]) => {
        setTicket(tk)
        setStatus(tk.status)
        setAssigneeIds((tk.assignees || []).map((a) => a.userId))
        const primary = (tk.assignees || []).find((a) => a.isPrimary)
        setPrimaryId(primary ? String(primary.userId) : '')
        setComments(c.comments ?? c)
        setHistory(h.history ?? h)
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [id])
  useEffect(() => {
    ticketsApi.assignableUsers().then((d) => setAssignableUsers(d.users ?? [])).catch(() => {})
  }, [])

  const handleStatusChange = async (s) => {
    const prev = status
    setStatus(s)
    try {
      await ticketsApi.update(id, { status: s })
      const h = await ticketsApi.history(id)
      setHistory(h.history ?? h)
    } catch (e) {
      setStatus(prev)
      setError(e.message)
    }
  }

  const saveAssignees = async (ids, primary) => {
    setAssigning(true)
    try { await ticketsApi.assign(id, ids, primary ? Number(primary) : null) }
    catch (e) { setError(e.message) }
    finally { setAssigning(false) }
  }

  const toggleAssignee = (uid) => {
    const next = assigneeIds.includes(uid) ? assigneeIds.filter((x) => x !== uid) : [...assigneeIds, uid]
    const nextPrimary = next.includes(Number(primaryId)) ? primaryId : ''
    setAssigneeIds(next)
    setPrimaryId(nextPrimary)
    saveAssignees(next, nextPrimary)
  }

  const handlePrimaryChange = (val) => {
    setPrimaryId(val)
    saveAssignees(assigneeIds, val)
  }

  const handleComment = async () => {
    if (!comment.trim()) return
    setSending(true)
    try {
      await ticketsApi.comment(id, comment)
      setComment('')
      const c = await ticketsApi.comments(id)
      setComments(c.comments ?? c)
    } catch (e) { setError(e.message) }
    finally { setSending(false) }
  }

  const statusLabel = (s) => t(`tickets.status_${s}`, { defaultValue: s })
  const priorityLabel = (p) => t(`tickets.priority_${p}`, { defaultValue: p })
  // Mirrors ticketWriteLocked on the backend: once a ticket is resolved or
  // closed, an operator can still see it but can't touch it any further —
  // status, assignment, comments — only a supervisor/admin still can.
  const locked = (status === 'resolved' || status === 'closed') && user?.userType === 3

  if (loading) return <div className="text-center py-5"><CSpinner /></div>
  if (!ticket)  return <CAlert color="danger">{t('ticket_detail.not_found')}</CAlert>

  return (
    <>
      <div className="d-flex align-items-center gap-3 mb-4">
        <CButton color="light" onClick={() => navigate('/tickets')}>
          <CIcon icon={cilArrowLeft} />
        </CButton>
        <h4 className="mb-0">#{ticket.id} — {ticket.subject}</h4>
        <CBadge color={STATUS_COLOR[status] ?? 'secondary'} className="ms-auto">{statusLabel(status)}</CBadge>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <div className="row g-3">
        <div className="col-lg-8">
          <CCard className="mb-3">
            <CCardHeader>{t('ticket_detail.description')}</CCardHeader>
            <CCardBody>
              <p className="mb-0" style={{ whiteSpace: 'pre-wrap' }}>
                {ticket.body || t('ticket_detail.no_description')}
              </p>
            </CCardBody>
          </CCard>

          <CCard>
            <CCardHeader>{t('ticket_detail.comments')} ({comments.length})</CCardHeader>
            <CCardBody>
              {comments.length === 0 && <p className="text-muted">{t('ticket_detail.no_comments')}</p>}
              {comments.map((c, i) => (
                <div key={i} className={`d-flex gap-3 mb-3 ${c.userId === user?.id ? 'flex-row-reverse' : ''}`}>
                  <div
                    className={`px-3 py-2 rounded-3 ${c.userId === user?.id ? 'bg-primary text-white' : 'bg-light'}`}
                    style={{ maxWidth: '75%' }}
                  >
                    <div className="small fw-semibold mb-1 opacity-75">{c.username}</div>
                    <div style={{ whiteSpace: 'pre-wrap' }}>{c.text}</div>
                    <div className="small opacity-50 mt-1">{new Date(c.createdAt).toLocaleString()}</div>
                  </div>
                </div>
              ))}

              {locked ? (
                <div className="text-muted small mt-3">{t('ticket_detail.locked_hint')}</div>
              ) : (
                <div className="d-flex gap-2 mt-3">
                  <CFormTextarea
                    rows={2}
                    placeholder={t('ticket_detail.write_comment')}
                    value={comment}
                    onChange={(e) => setComment(e.target.value)}
                    onKeyDown={(e) => e.ctrlKey && e.key === 'Enter' && handleComment()}
                  />
                  <CButton color="primary" onClick={handleComment} disabled={sending || !comment.trim()}>
                    <CIcon icon={cilSend} />
                  </CButton>
                </div>
              )}
            </CCardBody>
          </CCard>
        </div>

        <div className="col-lg-4">
          <CCard>
            <CCardHeader>{t('ticket_detail.details')}</CCardHeader>
            <CCardBody>
              <div className="mb-3">
                <label className="small text-muted d-block">{t('ticket_detail.status')}</label>
                <CFormSelect value={status} disabled={locked} onChange={(e) => handleStatusChange(e.target.value)}>
                  {STATUSES.map((s) => <option key={s} value={s}>{statusLabel(s)}</option>)}
                </CFormSelect>
                {locked && <div className="text-muted small mt-1">{t('ticket_detail.locked_hint')}</div>}
              </div>
              <div className="mb-3">
                <label className="small text-muted d-block mb-1">{t('ticket_detail.assigned_to')}</label>
                <div className="border rounded p-2" style={{ maxHeight: 160, overflowY: 'auto' }}>
                  {assignableUsers.map((u) => (
                    <CFormCheck
                      key={u.id}
                      id={`ticket-assignee-${u.id}`}
                      label={[u.firstName, u.lastName].filter(Boolean).join(' ') || u.username}
                      checked={assigneeIds.includes(u.id)}
                      disabled={assigning || locked}
                      onChange={() => toggleAssignee(u.id)}
                    />
                  ))}
                  {assignableUsers.length === 0 && (
                    <div className="text-muted small">{t('tasks.no_assignable_users')}</div>
                  )}
                </div>
                {assigneeIds.length > 1 && (
                  <CFormSelect
                    className="mt-2" size="sm" value={primaryId} disabled={assigning || locked}
                    onChange={(e) => handlePrimaryChange(e.target.value)}
                  >
                    <option value="">{t('tasks.no_primary')}</option>
                    {assignableUsers.filter((u) => assigneeIds.includes(u.id)).map((u) => (
                      <option key={u.id} value={u.id}>
                        {[u.firstName, u.lastName].filter(Boolean).join(' ') || u.username}
                      </option>
                    ))}
                  </CFormSelect>
                )}
              </div>
              <div className="mb-2">
                <span className="small text-muted">{t('ticket_detail.caller')}</span>
                <div>{ticket.callerNo || '—'}</div>
                {ticket.callerName && <div className="text-success small">{ticket.callerName}</div>}
              </div>
              {ticket.site && (
                <div className="mb-2">
                  <span className="small text-muted">{t('ticket_detail.site')}</span>
                  <div>{siteName(ticket.site, lang)}</div>
                </div>
              )}
              <div className="mb-2">
                <span className="small text-muted">{t('ticket_detail.priority')}</span>
                <div><CBadge color="info">{priorityLabel(ticket.priority)}</CBadge></div>
              </div>
              <div className="mb-2">
                <span className="small text-muted">{t('ticket_detail.created')}</span>
                <div>{new Date(ticket.createdAt).toLocaleString()}</div>
              </div>
              <div>
                <span className="small text-muted">{t('ticket_detail.updated')}</span>
                <div>{new Date(ticket.updatedAt).toLocaleString()}</div>
              </div>
            </CCardBody>
          </CCard>

          <CCard className="mt-3">
            <CCardHeader>{t('ticket_detail.history')}</CCardHeader>
            <CCardBody>
              {history.length === 0 && <p className="text-muted mb-0">{t('ticket_detail.no_history')}</p>}
              <div className="d-flex flex-column gap-2">
                {history.map((e) => (
                  <div key={`${e.kind}-${e.id}`} className="small">
                    <div>
                      <strong>{e.username || '—'}</strong>{' '}
                      {e.kind === 'assignment'
                        ? (e.assignees
                            ? t('ticket_detail.event_assigned', { names: e.assignees })
                            : t('ticket_detail.event_unassigned'))
                        : (e.oldStatus === ''
                            ? t('ticket_detail.event_opened')
                            : t('ticket_detail.event_status_changed', { from: statusLabel(e.oldStatus), to: statusLabel(e.newStatus) }))}
                    </div>
                    <div className="text-muted">{new Date(e.createdAt).toLocaleString()}</div>
                  </div>
                ))}
              </div>
            </CCardBody>
          </CCard>
        </div>
      </div>
    </>
  )
}
