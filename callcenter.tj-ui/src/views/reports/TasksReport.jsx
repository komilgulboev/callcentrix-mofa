import React, { useEffect, useState } from 'react'
import {
  CCard, CCardBody, CAlert, CSpinner, CTable, CTableBody, CTableDataCell,
  CTableHead, CTableHeaderCell, CTableRow, CBadge, CButton, CFormInput, CFormSelect, CFormLabel,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilCloudDownload } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import * as XLSX from 'xlsx'
import { reports as reportsApi, tenants as tenantsApi } from 'src/api'
import useAuthStore from 'src/store/auth'

const STATUS_COLOR = { todo: 'secondary', in_progress: 'info', waiting: 'warning', resolved: 'success' }
const STATUSES = ['todo', 'in_progress', 'waiting', 'resolved']

function isoDate(offsetDays) {
  const d = new Date()
  d.setDate(d.getDate() + offsetDays)
  return d.toISOString().slice(0, 10)
}

// One tile per status — the "по количеству статусов" half of the report.
// flex-nowrap + flex-fill keeps all five tiles on a single row at any width
// (shrinking together) instead of the old 12-column grid, which wrapped the
// 5th ("Всего") tile onto its own row once 4 tiles already filled the row.
function StatusCounts({ counts, statusLabel }) {
  const total = STATUSES.reduce((sum, s) => sum + (counts[s] || 0), 0)
  const tiles = [
    ...STATUSES.map((s) => ({ key: s, color: STATUS_COLOR[s], value: counts[s] || 0, label: statusLabel(s) })),
    { key: 'total', color: 'dark', value: total, label: statusLabel('total') },
  ]
  return (
    <div className="d-flex flex-nowrap gap-2 mb-3">
      {tiles.map((tile) => (
        <CCard key={tile.key} className={`flex-fill border-top border-top-${tile.color} border-top-3`} style={{ minWidth: 0 }}>
          <CCardBody className="py-1 px-2">
            <div className="fs-6 fw-bold">{tile.value}</div>
            <div className="text-muted text-truncate" style={{ fontSize: '0.72rem' }}>{tile.label}</div>
          </CCardBody>
        </CCard>
      ))}
    </div>
  )
}

export default function TasksReport() {
  const { t } = useTranslation()
  const { user, isSuperAdmin } = useAuthStore()
  const superAdmin = isSuperAdmin()

  const [rows,         setRows]         = useState([])
  const [statusCounts, setStatusCounts] = useState({})
  const [loading,      setLoading]      = useState(true)
  const [error,        setError]        = useState('')
  const [dateFrom,     setDateFrom]     = useState(() => isoDate(-30))
  const [dateTo,       setDateTo]       = useState(() => isoDate(0))
  const [statusFilter, setStatusFilter] = useState('')
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
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const load = () => {
    if (superAdmin && !selectedTid) return
    setLoading(true)
    reportsApi.tasks({
      date_from: dateFrom,
      date_to:   dateTo,
      status:    statusFilter || undefined,
      tenantId:  superAdmin ? selectedTid : undefined,
    })
      .then((d) => { setRows(d.tasks ?? []); setStatusCounts(d.statusCounts ?? {}) })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [dateFrom, dateTo, statusFilter, selectedTid])

  const statusLabel = (s) => (s === 'total' ? t('reports.total_short') : t(`tasks.status_${s}`, { defaultValue: s }))

  const handleExport = () => {
    const data = rows.map((row) => ({
      '#': row.id,
      [t('tasks.task_title_label')]: row.title,
      [t('tasks.col_status')]: statusLabel(row.status),
      [t('tasks.created_by')]: row.createdBy || '',
      [t('tasks.assigned_to')]: row.assignees || '',
      [t('tickets.col_created')]: new Date(row.createdAt).toLocaleString(),
      [t('tasks.resolved_by')]: row.resolvedBy || '',
      [t('reports.resolved_at')]: row.resolvedAt ? new Date(row.resolvedAt).toLocaleString() : '',
    }))
    const sheet = XLSX.utils.json_to_sheet(data)
    const book = XLSX.utils.book_new()
    XLSX.utils.book_append_sheet(book, sheet, t('reports.tasks_title').slice(0, 31))
    XLSX.writeFile(book, `${t('reports.tasks_title')} ${dateFrom} - ${dateTo}.xlsx`)
  }

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('reports.tasks_title')}</h4>
        <CButton color="success" variant="outline" size="sm" onClick={handleExport} disabled={loading || !rows.length}>
          <CIcon icon={cilCloudDownload} className="me-2" />{t('reports.export_excel')}
        </CButton>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {superAdmin && (
        <div className="mb-3" style={{ maxWidth: 340 }}>
          <CFormLabel>{t('common.tenant')}</CFormLabel>
          <CFormSelect value={selectedTid} onChange={(e) => setSelectedTid(e.target.value)}>
            <option value="">{t('reports.select_tenant')}</option>
            {tenantsList.map((tn) => <option key={tn.id} value={tn.id}>{tn.name}</option>)}
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
          <label className="small text-muted d-block mb-1">{t('tasks.col_status')}</label>
          <CFormSelect style={{ width: 160 }} value={statusFilter} onChange={(e) => setStatusFilter(e.target.value)}>
            <option value="">{t('tickets.all_statuses')}</option>
            {STATUSES.map((s) => <option key={s} value={s}>{statusLabel(s)}</option>)}
          </CFormSelect>
        </div>
      </div>

      {loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : (
        <>
          <StatusCounts counts={statusCounts} statusLabel={statusLabel} />
          <CCard>
            <CCardBody className="p-0">
              <CTable hover responsive className="mb-0">
                <CTableHead>
                  <CTableRow>
                    <CTableHeaderCell>#</CTableHeaderCell>
                    <CTableHeaderCell>{t('tasks.task_title_label')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('tasks.col_status')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('tasks.created_by')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('tasks.assigned_to')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('tickets.col_created')}</CTableHeaderCell>
                    <CTableHeaderCell>{t('tasks.resolved_by')}</CTableHeaderCell>
                  </CTableRow>
                </CTableHead>
                <CTableBody>
                  {rows.map((row) => (
                    <CTableRow key={row.id}>
                      <CTableDataCell className="text-muted">#{row.id}</CTableDataCell>
                      <CTableDataCell className="fw-semibold">{row.title}</CTableDataCell>
                      <CTableDataCell>
                        <CBadge color={STATUS_COLOR[row.status] ?? 'secondary'}>{statusLabel(row.status)}</CBadge>
                      </CTableDataCell>
                      <CTableDataCell className="text-muted small">{row.createdBy || '—'}</CTableDataCell>
                      <CTableDataCell className="text-muted small">{row.assignees || '—'}</CTableDataCell>
                      <CTableDataCell className="text-muted small">{new Date(row.createdAt).toLocaleString()}</CTableDataCell>
                      <CTableDataCell className="text-muted small">
                        {row.resolvedAt
                          ? `${row.resolvedBy || '—'} (${new Date(row.resolvedAt).toLocaleString()})`
                          : '—'}
                      </CTableDataCell>
                    </CTableRow>
                  ))}
                  {!rows.length && (
                    <CTableRow>
                      <CTableDataCell colSpan={7} className="text-center text-muted py-4">
                        {t('reports.tasks_empty')}
                      </CTableDataCell>
                    </CTableRow>
                  )}
                </CTableBody>
              </CTable>
            </CCardBody>
          </CCard>
          {rows.length > 0 && (
            <div className="text-muted small mt-2 text-end">
              {t('reports.tasks_total', { total: rows.length })}
            </div>
          )}
        </>
      )}
    </>
  )
}
