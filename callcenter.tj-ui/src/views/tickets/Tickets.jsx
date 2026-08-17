import React, { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  CButton, CCard, CCardBody, CModal, CModalBody, CModalFooter,
  CModalHeader, CModalTitle, CForm, CFormInput, CFormLabel,
  CFormSelect, CFormTextarea, CTable, CTableBody, CTableDataCell,
  CTableHead, CTableHeaderCell, CTableRow, CBadge, CAlert, CSpinner,
  CInputGroup, CInputGroupText,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilSearch } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { tickets as ticketsApi, topics as topicsApi } from 'src/api'
import { topicName } from 'src/views/topics/Topics'

const STATUS_COLOR = { new: 'primary', open: 'warning', pending: 'info', resolved: 'success', closed: 'secondary' }
const PRIORITY_COLOR = { low: 'secondary', normal: 'info', high: 'warning', urgent: 'danger' }
const EMPTY = { subject: '', callerNo: '', body: '', priority: 'normal', status: 'new', topicId: '' }

export default function Tickets() {
  const { t, i18n } = useTranslation()
  const lang = i18n.language

  const [rows,         setRows]         = useState([])
  const [loading,      setLoading]      = useState(true)
  const [error,        setError]        = useState('')
  const [modal,        setModal]        = useState(false)
  const [form,         setForm]         = useState(EMPTY)
  const [saving,       setSaving]       = useState(false)
  const [search,       setSearch]       = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [topicFilter,  setTopicFilter]  = useState('')
  const [topicsList,   setTopicsList]   = useState([])
  const [topicsMap,    setTopicsMap]    = useState({})
  const navigate = useNavigate()

  const loadTopics = () => {
    topicsApi.my()
      .then((d) => {
        const list = d.topics ?? []
        const map = {}
        list.forEach((t) => { map[t.id] = t })
        setTopicsList(list)
        setTopicsMap(map)
      })
      .catch(() => {})
  }

  const load = () => {
    setLoading(true)
    ticketsApi.list({
      status:  statusFilter || undefined,
      topicId: topicFilter  || undefined,
      search:  search       || undefined,
    })
      .then((d) => setRows(d.tickets ?? d))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(() => { loadTopics() }, [])
  useEffect(load, [statusFilter, topicFilter])

  const handleSearch = (e) => { e.preventDefault(); load() }

  const openModal = () => { setForm(EMPTY); setModal(true) }

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload = { ...form }
      if (!payload.topicId) delete payload.topicId
      await ticketsApi.create(payload)
      setModal(false)
      setForm(EMPTY)
      load()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const getTopicLabel = (ticket) => {
    if (ticket.topic) return topicName(ticket.topic, lang)
    if (ticket.topicId && topicsMap[ticket.topicId]) return topicName(topicsMap[ticket.topicId], lang)
    return '—'
  }

  const statusLabel = (s) => t(`tickets.status_${s}`, { defaultValue: s })
  const priorityLabel = (p) => t(`tickets.priority_${p}`, { defaultValue: p })

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('tickets.title')}</h4>
        <CButton color="primary" onClick={openModal}>
          <CIcon icon={cilPlus} className="me-2" />{t('tickets.new_ticket')}
        </CButton>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <div className="d-flex gap-2 mb-3 flex-wrap">
        <form onSubmit={handleSearch} className="d-flex gap-2">
          <CInputGroup style={{ width: 260 }}>
            <CInputGroupText><CIcon icon={cilSearch} /></CInputGroupText>
            <CFormInput placeholder={t('common.search_placeholder')} value={search} onChange={(e) => setSearch(e.target.value)} />
          </CInputGroup>
          <CButton type="submit" color="light">{t('common.search')}</CButton>
        </form>
        <CFormSelect style={{ width: 160 }} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
          <option value="">{t('tickets.all_statuses')}</option>
          {['new', 'open', 'pending', 'resolved', 'closed'].map((s) => (
            <option key={s} value={s}>{statusLabel(s)}</option>
          ))}
        </CFormSelect>
        {topicsList.length > 0 && (
          <CFormSelect style={{ width: 200 }} value={topicFilter} onChange={(e) => setTopicFilter(e.target.value)}>
            <option value="">{t('tickets.all_topics')}</option>
            {topicsList.map((tp) => (
              <option key={tp.id} value={tp.id}>{topicName(tp, lang)}</option>
            ))}
          </CFormSelect>
        )}
      </div>

      <CCard>
        <CCardBody className="p-0">
          {loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive className="cursor-pointer">
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell>#</CTableHeaderCell>
                  <CTableHeaderCell>{t('tickets.col_subject')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('tickets.col_topic')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('tickets.col_caller')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('tickets.col_priority')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('tickets.col_status')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('tickets.col_created')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map((ticket) => (
                  <CTableRow key={ticket.id} onClick={() => navigate(`/tickets/${ticket.id}`)}>
                    <CTableDataCell className="text-muted">#{ticket.id}</CTableDataCell>
                    <CTableDataCell className="fw-semibold">{ticket.subject}</CTableDataCell>
                    <CTableDataCell className="text-muted small">{getTopicLabel(ticket)}</CTableDataCell>
                    <CTableDataCell>{ticket.callerNo || '—'}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={PRIORITY_COLOR[ticket.priority] ?? 'secondary'}>{priorityLabel(ticket.priority)}</CBadge>
                    </CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={STATUS_COLOR[ticket.status] ?? 'secondary'}>{statusLabel(ticket.status)}</CBadge>
                    </CTableDataCell>
                    <CTableDataCell className="text-muted small">
                      {new Date(ticket.createdAt).toLocaleDateString()}
                    </CTableDataCell>
                  </CTableRow>
                ))}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={7} className="text-center text-muted py-4">{t('tickets.empty')}</CTableDataCell>
                  </CTableRow>
                )}
              </CTableBody>
            </CTable>
          )}
        </CCardBody>
      </CCard>

      <CModal visible={modal} onClose={() => setModal(false)} size="lg">
        <CModalHeader><CModalTitle>{t('tickets.new_ticket')}</CModalTitle></CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <div>
              <CFormLabel>{t('tickets.subject')}</CFormLabel>
              <CFormInput
                value={form.subject}
                onChange={(e) => setForm({ ...form, subject: e.target.value })}
                placeholder={t('tickets.subject_placeholder')}
              />
            </div>
            <div className="row g-2">
              <div className="col">
                <CFormLabel>{t('tickets.caller_number')}</CFormLabel>
                <CFormInput
                  value={form.callerNo}
                  onChange={(e) => setForm({ ...form, callerNo: e.target.value })}
                  placeholder="+992…"
                />
              </div>
              <div className="col">
                <CFormLabel>{t('common.priority')}</CFormLabel>
                <CFormSelect value={form.priority} onChange={(e) => setForm({ ...form, priority: e.target.value })}>
                  <option value="low">{t('tickets.priority_low')}</option>
                  <option value="normal">{t('tickets.priority_normal')}</option>
                  <option value="high">{t('tickets.priority_high')}</option>
                  <option value="urgent">{t('tickets.priority_urgent')}</option>
                </CFormSelect>
              </div>
            </div>
            {topicsList.length > 0 && (
              <div>
                <CFormLabel>{t('common.topic')}</CFormLabel>
                <CFormSelect value={form.topicId} onChange={(e) => setForm({ ...form, topicId: e.target.value })}>
                  <option value="">{t('tickets.no_topic')}</option>
                  {topicsList.map((tp) => (
                    <option key={tp.id} value={tp.id}>{topicName(tp, lang)}</option>
                  ))}
                </CFormSelect>
              </div>
            )}
            <div>
              <CFormLabel>{t('tickets.description')}</CFormLabel>
              <CFormTextarea
                rows={4}
                value={form.body}
                onChange={(e) => setForm({ ...form, body: e.target.value })}
                placeholder={t('tickets.description_placeholder')}
              />
            </div>
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving || !form.subject}>
            {saving ? <CSpinner size="sm" /> : t('tickets.create_ticket')}
          </CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
