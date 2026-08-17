import React, { useEffect, useState } from 'react'
import {
  CCard, CCardBody, CCardHeader, CAlert, CSpinner, CTable, CTableBody, CTableDataCell,
  CTableHead, CTableHeaderCell, CTableRow, CBadge, CButton, CFormInput, CFormSelect, CFormLabel,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilMediaPlay } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { reports as reportsApi, topics as topicsApi, tenants as tenantsApi, cdr as cdrApi } from 'src/api'
import { topicName } from 'src/views/topics/Topics'
import useAuthStore from 'src/store/auth'

const STATUS_COLOR = { new: 'primary', open: 'warning', pending: 'info', resolved: 'success', closed: 'secondary' }
const STATUSES = ['new', 'open', 'pending', 'resolved', 'closed']

function isoDate(offsetDays) {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}

// Groups the flat report rows by topic — the report's "группировка" view.
// Ungrouped tickets land in one "no topic" bucket rather than being dropped.
function groupByTopic(rows, noTopicLabel, lang) {
  const map = new Map()
  for (const row of rows) {
    const key = row.topicId ?? 'none'
    const label = row.topic ? topicName(row.topic, lang) : noTopicLabel
    if (!map.has(key)) map.set(key, { key, label, rows: [] })
    map.get(key).rows.push(row)
  }
  return Array.from(map.values())
}

function TicketsTable({ rows, showTopic, getTopicLabel, statusLabel, t }) {
  return (
    <CTable hover responsive className="mb-0">
      <CTableHead>
        <CTableRow>
          <CTableHeaderCell>#</CTableHeaderCell>
          <CTableHeaderCell>{t('tickets.col_subject')}</CTableHeaderCell>
          {showTopic && <CTableHeaderCell>{t('tickets.col_topic')}</CTableHeaderCell>}
          <CTableHeaderCell>{t('reports.col_caller')}</CTableHeaderCell>
          <CTableHeaderCell>{t('reports.col_handled_by')}</CTableHeaderCell>
          <CTableHeaderCell>{t('reports.col_assigned_to')}</CTableHeaderCell>
          <CTableHeaderCell>{t('tickets.col_status')}</CTableHeaderCell>
          <CTableHeaderCell>{t('tickets.col_created')}</CTableHeaderCell>
          <CTableHeaderCell>{t('reports.col_recording')}</CTableHeaderCell>
        </CTableRow>
      </CTableHead>
      <CTableBody>
        {rows.map((row) => (
          <CTableRow key={row.id}>
            <CTableDataCell className="text-muted">#{row.id}</CTableDataCell>
            <CTableDataCell className="fw-semibold">{row.subject}</CTableDataCell>
            {showTopic && <CTableDataCell className="text-muted small">{getTopicLabel(row)}</CTableDataCell>}
            <CTableDataCell>{row.callerNo || '—'}</CTableDataCell>
            <CTableDataCell className="text-muted small">{row.handledBy || '—'}</CTableDataCell>
            <CTableDataCell className="text-muted small">{row.assignedTo || '—'}</CTableDataCell>
            <CTableDataCell>
              <CBadge color={STATUS_COLOR[row.status] ?? 'secondary'}>{statusLabel(row.status)}</CBadge>
            </CTableDataCell>
            <CTableDataCell className="text-muted small">{new Date(row.createdAt).toLocaleString()}</CTableDataCell>
            <CTableDataCell>
              {row.cdrId ? (
                <CButton size="sm" color="light" href={cdrApi.audio(row.cdrId)} target="_blank" title={t('cdr.play_recording')}>
                  <CIcon icon={cilMediaPlay} />
                </CButton>
              ) : '—'}
            </CTableDataCell>
          </CTableRow>
        ))}
        {!rows.length && (
          <CTableRow>
            <CTableDataCell colSpan={showTopic ? 9 : 8} className="text-center text-muted py-4">
              {t('reports.empty')}
            </CTableDataCell>
          </CTableRow>
        )}
      </CTableBody>
    </CTable>
  )
}

export default function TicketsReport() {
  const { t, i18n } = useTranslation()
  const lang = i18n.language
  const { user, isSuperAdmin } = useAuthStore()
  const superAdmin = isSuperAdmin()

  const [rows,         setRows]         = useState([])
  const [loading,      setLoading]      = useState(true)
  const [error,        setError]        = useState('')
  const [dateFrom,     setDateFrom]     = useState(() => isoDate(-7))
  const [dateTo,       setDateTo]       = useState(() => isoDate(0))
  const [topicFilter,  setTopicFilter]  = useState('')
  const [statusFilter, setStatusFilter] = useState('')
  const [view,         setView]         = useState('list') // 'list' | 'group'
  const [topicsList,   setTopicsList]   = useState([])
  const [tenantsList,  setTenantsList]  = useState([])
  const [selectedTid,  setSelectedTid]  = useState(superAdmin ? '' : String(user?.tenantId ?? ''))

  useEffect(() => {
    if (!superAdmin) return
    tenantsApi.list()
      .then((d) => {
        const list = d.tenants ?? []
        setTenantsList(list)
        if (list.length > 0 && !selectedTid) setSelectedTid(String(list[0].id))
      })
      .catch(() => {})
  }, [])

  useEffect(() => {
    if (superAdmin) {
      if (!selectedTid) { setTopicsList([]); return }
      topicsApi.list(selectedTid).then((d) => setTopicsList(d.topics ?? [])).catch(() => {})
    } else {
      topicsApi.my().then((d) => setTopicsList(d.topics ?? [])).catch(() => {})
    }
  }, [superAdmin, selectedTid])

  const load = () => {
    if (superAdmin && !selectedTid) return
    setLoading(true)
    reportsApi.tickets({
      date_from: dateFrom,
      date_to:   dateTo,
      topicId:   topicFilter  || undefined,
      status:    statusFilter || undefined,
      tenantId:  superAdmin ? selectedTid : undefined,
    })
      .then((d) => setRows(d.tickets ?? []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [dateFrom, dateTo, topicFilter, statusFilter, selectedTid])

  const statusLabel = (s) => t(`tickets.status_${s}`, { defaultValue: s })
  const getTopicLabel = (row) => (row.topic ? topicName(row.topic, lang) : t('reports.no_topic'))

  const groups = view === 'group' ? groupByTopic(rows, t('reports.no_topic'), lang) : null

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('reports.tickets_title')}</h4>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {superAdmin && (
        <div className="mb-3" style={{ maxWidth: 340 }}>
          <CFormLabel>{t('common.tenant')}</CFormLabel>
          <CFormSelect value={selectedTid} onChange={(e) => setSelectedTid(e.target.value)}>
            <option value="">{t('reports.select_tenant')}</option>
            {tenantsList.map((tp) => <option key={tp.id} value={tp.id}>{tp.name}</option>)}
          </CFormSelect>
        </div>
      )}

      <div className="d-flex gap-2 mb-3 flex-wrap align-items-end">
        <div>
          <label className="small text-muted d-block mb-1">{t('cdr.filter_from')}</label>
          <CFormInput type="date" value={dateFrom} onChange={(e) => setDateFrom(e.target.value)} />
        </div>
        <div>
          <label className="small text-muted d-block mb-1">{t('cdr.filter_to')}</label>
          <CFormInput type="date" value={dateTo} onChange={(e) => setDateTo(e.target.value)} />
        </div>
        <div>
          <label className="small text-muted d-block mb-1">{t('common.topic')}</label>
          <CFormSelect style={{ width: 200 }} value={topicFilter} onChange={(e) => setTopicFilter(e.target.value)}>
            <option value="">{t('tickets.all_topics')}</option>
            {topicsList.map((tp) => <option key={tp.id} value={tp.id}>{topicName(tp, lang)}</option>)}
          </CFormSelect>
        </div>
        <div>
          <label className="small text-muted d-block mb-1">{t('tickets.col_status')}</label>
          <CFormSelect style={{ width: 160 }} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
            <option value="">{t('tickets.all_statuses')}</option>
            {STATUSES.map((s) => <option key={s} value={s}>{statusLabel(s)}</option>)}
          </CFormSelect>
        </div>
        <div className="ms-auto">
          <label className="small text-muted d-block mb-1">{t('reports.view_mode')}</label>
          <div className="btn-group">
            <CButton color={view === 'list' ? 'primary' : 'light'} onClick={() => setView('list')}>
              {t('reports.view_list')}
            </CButton>
            <CButton color={view === 'group' ? 'primary' : 'light'} onClick={() => setView('group')}>
              {t('reports.view_grouped')}
            </CButton>
          </div>
        </div>
      </div>

      {loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : view === 'list' ? (
        <CCard>
          <CCardBody className="p-0">
            <TicketsTable rows={rows} showTopic getTopicLabel={getTopicLabel} statusLabel={statusLabel} t={t} />
          </CCardBody>
        </CCard>
      ) : (
        <div className="d-flex flex-column gap-3">
          {groups.map((g) => (
            <CCard key={g.key}>
              <CCardHeader>{g.label} ({g.rows.length})</CCardHeader>
              <CCardBody className="p-0">
                <TicketsTable rows={g.rows} showTopic={false} getTopicLabel={getTopicLabel} statusLabel={statusLabel} t={t} />
              </CCardBody>
            </CCard>
          ))}
          {!groups.length && (
            <CCard><CCardBody className="text-center text-muted py-4">{t('reports.empty')}</CCardBody></CCard>
          )}
        </div>
      )}

      {!loading && rows.length > 0 && (
        <div className="text-muted small mt-2 text-end">
          {t('reports.total', { total: rows.length })}
        </div>
      )}
    </>
  )
}
