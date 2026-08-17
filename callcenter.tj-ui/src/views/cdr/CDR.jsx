import React, { useEffect, useState } from 'react'
import {
  CCard, CCardBody, CAlert, CSpinner, CTable, CTableBody, CTableDataCell,
  CTableHead, CTableHeaderCell, CTableRow, CBadge, CButton,
  CFormInput, CFormSelect, CInputGroup, CInputGroupText,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilSearch, cilMediaPlay } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { cdr as cdrApi } from 'src/api'

const DISPOSITION_COLOR = { ANSWERED: 'success', 'NO ANSWER': 'warning', BUSY: 'danger', FAILED: 'secondary' }
const DISPOSITION_KEY = {
  ANSWERED: 'cdr.disposition_answered',
  'NO ANSWER': 'cdr.disposition_no_answer',
  BUSY: 'cdr.disposition_busy',
  FAILED: 'cdr.disposition_failed',
}

function fmtDur(s) {
  if (!s) return '—'
  const m = Math.floor(s / 60)
  return `${m}:${String(s % 60).padStart(2, '0')}`
}

export default function CDR() {
  const { t } = useTranslation()
  const [rows,    setRows]    = useState([])
  const [loading, setLoading] = useState(true)
  const [error,   setError]   = useState('')
  const [search,  setSearch]  = useState('')
  const [dateFrom,setDateFrom]= useState(() => new Date().toISOString().slice(0, 10))
  const [dateTo,  setDateTo]  = useState(() => new Date().toISOString().slice(0, 10))
  const [disp,    setDisp]    = useState('')

  const load = () => {
    setLoading(true)
    cdrApi.list({ search: search || undefined, date_from: dateFrom, date_to: dateTo, disposition: disp || undefined })
      .then((d) => setRows(d.records ?? d))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [dateFrom, dateTo, disp])

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('cdr.title')}</h4>
        <span className="text-muted small">{t('cdr.records', { count: rows.length })}</span>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

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
          <label className="small text-muted d-block mb-1">{t('cdr.disposition')}</label>
          <CFormSelect value={disp} onChange={(e) => setDisp(e.target.value)} style={{ width: 160 }}>
            <option value="">{t('cdr.disposition_all')}</option>
            <option value="ANSWERED">{t('cdr.disposition_answered')}</option>
            <option value="NO ANSWER">{t('cdr.disposition_no_answer')}</option>
            <option value="BUSY">{t('cdr.disposition_busy')}</option>
          </CFormSelect>
        </div>
        <div className="flex-grow-1">
          <label className="small text-muted d-block mb-1">{t('common.search')}</label>
          <form onSubmit={(e) => { e.preventDefault(); load() }} className="d-flex gap-2">
            <CInputGroup>
              <CInputGroupText><CIcon icon={cilSearch} /></CInputGroupText>
              <CFormInput placeholder={t('cdr.search_placeholder')} value={search} onChange={(e) => setSearch(e.target.value)} />
            </CInputGroup>
            <CButton type="submit" color="light">{t('common.search')}</CButton>
          </form>
        </div>
      </div>

      <CCard>
        <CCardBody className="p-0">
          {loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive>
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell>{t('cdr.col_datetime')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('cdr.col_from')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('cdr.col_to')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('cdr.col_duration')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('cdr.col_result')}</CTableHeaderCell>
                  <CTableHeaderCell></CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {rows.map((r, i) => (
                  <CTableRow key={i}>
                    <CTableDataCell className="text-muted small">
                      {new Date(r.callDate ?? r.start).toLocaleString()}
                    </CTableDataCell>
                    <CTableDataCell>{r.src}</CTableDataCell>
                    <CTableDataCell>{r.dst}</CTableDataCell>
                    <CTableDataCell>{fmtDur(r.duration)}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={DISPOSITION_COLOR[r.disposition] ?? 'secondary'}>
                        {t(DISPOSITION_KEY[r.disposition] ?? r.disposition, { defaultValue: r.disposition })}
                      </CBadge>
                    </CTableDataCell>
                    <CTableDataCell>
                      {r.recording && (
                        <CButton size="sm" color="light" href={cdrApi.audio(r.id)} target="_blank" title={t('cdr.play_recording')}>
                          <CIcon icon={cilMediaPlay} />
                        </CButton>
                      )}
                    </CTableDataCell>
                  </CTableRow>
                ))}
                {!rows.length && (
                  <CTableRow>
                    <CTableDataCell colSpan={6} className="text-center text-muted py-4">{t('cdr.empty')}</CTableDataCell>
                  </CTableRow>
                )}
              </CTableBody>
            </CTable>
          )}
        </CCardBody>
      </CCard>
    </>
  )
}
