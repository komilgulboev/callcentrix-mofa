import React, { useCallback, useEffect, useState } from 'react'
import {
  CCard, CCardBody, CCardHeader, CCol, CRow, CSpinner, CBadge,
  CButton, CTable, CTableBody, CTableDataCell, CTableHead, CTableHeaderCell, CTableRow,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilMediaPause, cilMediaPlay, cilMediaStop } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { tickets, cdr, monitor as monitorApi, providers as providersApi } from 'src/api'
import useAuthStore from 'src/store/auth'
import { statusBadge, PROVIDER_STATUS_POLL_MS } from 'src/utils/providerStatus'
import TasksBoard from './TasksBoard'

const MONITOR_POLL_MS = 3000

function ProvidersStatusCard() {
  const { t } = useTranslation()
  const [providersList, setProvidersList] = useState(null) // null = loading

  useEffect(() => {
    const load = () => providersApi.list().then(d => setProvidersList(d.providers || [])).catch(() => setProvidersList([]))
    load()
    const timer = setInterval(load, PROVIDER_STATUS_POLL_MS)
    return () => clearInterval(timer)
  }, [])

  return (
    <CCard className="h-100">
      <CCardBody className="py-2">
        <div className="text-muted small fw-semibold mb-1">{t('dashboard.providers_status')}</div>
        {providersList === null ? (
          <div className="text-center py-1"><CSpinner size="sm" /></div>
        ) : !providersList.length ? (
          <div className="text-muted small">{t('dashboard.no_providers')}</div>
        ) : (
          <div className="d-flex flex-column gap-1" style={{ fontSize: '0.8rem', maxHeight: 90, overflowY: 'auto' }}>
            {providersList.map(p => {
              const badge = statusBadge(p.status)
              return (
                <div key={p.id} className="d-flex justify-content-between align-items-center">
                  <span>{p.name} <span className="text-muted">({p.host}:{p.port})</span></span>
                  {badge ? <CBadge color={badge.color} style={{ fontSize: '0.65rem' }}>{badge.label}</CBadge> : <span className="text-muted">—</span>}
                </div>
              )
            })}
          </div>
        )}
      </CCardBody>
    </CCard>
  )
}

function StatCard({ title, value, color, icon }) {
  return (
    <CCard className={`border-top border-top-${color} border-top-3 h-100`}>
      <CCardBody className="py-2">
        <div className="d-flex align-items-center justify-content-between">
          <div>
            <div className="fs-4 fw-bold">{value ?? <CSpinner size="sm" />}</div>
            <div className="text-muted small">{title}</div>
          </div>
          <div className={`fs-2 text-${color} opacity-25`}>{icon}</div>
        </div>
      </CCardBody>
    </CCard>
  )
}

function fmtSecs(s) {
  if (!s) return '—'
  const m = Math.floor(s / 60)
  return `${m}m ${s % 60}s`
}

// Unlike fmtSecs, 0 is a real, current value here (a caller who just joined
// the queue) rather than "hasn't happened yet" — so it must render as a
// duration, not '—'.
function fmtWait(s) {
  const sec = s || 0
  const m = Math.floor(sec / 60)
  return `${m}m ${sec % 60}s`
}

// Combines status + call direction into one label — "ringing" alone doesn't
// say whether the agent is placing or receiving that call.
function agentStateLabel(a, t) {
  if (a.status === 'paused')  return { text: t('monitor.status_paused'),  color: 'warning',  icon: '⏸' }
  if (a.status === 'ringing' && a.direction === 'out') return { text: t('dashboard.direction_out'), color: 'info', icon: '📤' }
  if (a.status === 'ringing' && a.direction === 'in')  return { text: t('dashboard.direction_in'),  color: 'info', icon: '📥' }
  if (a.status === 'busy')    return { text: t('monitor.status_busy'),    color: 'danger',   icon: '💬' }
  if (a.status === 'available') return { text: t('monitor.status_available'), color: 'success', icon: '✅' }
  return { text: t('monitor.status_offline'), color: 'secondary', icon: '⚪' }
}

// Polls the tenant-scoped dashboard monitor endpoint and exposes derived
// counts plus agent-management actions — separate from the presentational
// AgentsCard below so its hooks live in one obvious place.
function useAgentsMonitor() {
  const [snapshot, setSnapshot] = useState(null) // null = loading
  const [acting, setActing] = useState(null) // sipNo currently being acted on

  const load = useCallback(() => {
    monitorApi.tenantSnapshot().then(setSnapshot).catch(() => {})
  }, [])

  useEffect(() => {
    load()
    const timer = setInterval(load, MONITOR_POLL_MS)
    return () => clearInterval(timer)
  }, [load])

  const agents = Object.values(snapshot?.agents || {})
  const queues = Object.values(snapshot?.queues || {})
  const waitingCallers = snapshot?.waitingCallers || []

  const handlePause = async (a) => {
    setActing(a.sipNo)
    try { await (a.status === 'paused' ? monitorApi.unpause(a.sipNo) : monitorApi.pause(a.sipNo)); load() }
    finally { setActing(null) }
  }
  const handleHangup = async (a) => {
    if (!a.channel) return
    setActing(a.sipNo)
    try { await monitorApi.hangup(a.channel); load() }
    finally { setActing(null) }
  }

  return {
    loading: snapshot === null,
    agents,
    waitingCallers,
    onlineCount:     agents.filter(a => a.status !== 'offline').length,
    activeCallCount: Object.keys(snapshot?.calls || {}).length,
    waitingCount:    queues.reduce((s, q) => s + (q.waiting || 0), 0),
    acting,
    handlePause,
    handleHangup,
  }
}

function AgentsCard({ loading, agents, canManage, acting, onPause, onHangup }) {
  const { t } = useTranslation()
  return (
    <CCard>
      <CCardHeader>{t('monitor.agents')} ({agents.length})</CCardHeader>
      <CCardBody className="p-0">
        {loading ? (
          <div className="text-center py-5"><CSpinner size="sm" /></div>
        ) : (
          <CTable hover responsive className="mb-0">
            <CTableHead>
              <CTableRow>
                <CTableHeaderCell>{t('monitor.col_agent')}</CTableHeaderCell>
                <CTableHeaderCell>{t('monitor.col_status')}</CTableHeaderCell>
                <CTableHeaderCell>{t('dashboard.col_call')}</CTableHeaderCell>
                <CTableHeaderCell>{t('dashboard.col_first_online')}</CTableHeaderCell>
                <CTableHeaderCell>{t('dashboard.col_paused_today')}</CTableHeaderCell>
                {canManage && <CTableHeaderCell></CTableHeaderCell>}
              </CTableRow>
            </CTableHead>
            <CTableBody>
              {agents.map((a) => {
                const label = agentStateLabel(a, t)
                const inCall = ['ringing', 'busy'].includes(a.status)
                return (
                  <CTableRow key={a.sipNo}>
                    <CTableDataCell className="fw-semibold">{a.name || a.sipNo}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={label.color}>{label.icon} {label.text}</CBadge>
                    </CTableDataCell>
                    <CTableDataCell className="small">
                      {inCall
                        ? <>{a.callerNumber || '—'} → {a.calleeNumber || '—'} {a.status === 'busy' && <span className="text-muted">({fmtSecs(a.callDuration)})</span>}</>
                        : '—'}
                    </CTableDataCell>
                    <CTableDataCell className="text-muted small">{a.firstOnlineToday || '—'}</CTableDataCell>
                    <CTableDataCell className="text-muted small">{fmtSecs(a.pausedTodaySeconds)}</CTableDataCell>
                    {canManage && (
                      <CTableDataCell className="text-end">
                        <div className="d-flex gap-1 justify-content-end">
                          <CButton size="sm" color={a.status === 'paused' ? 'success' : 'warning'}
                            disabled={acting === a.sipNo}
                            onClick={() => onPause(a)}
                            title={a.status === 'paused' ? t('monitor.resume') : t('monitor.pause')}>
                            <CIcon icon={a.status === 'paused' ? cilMediaPlay : cilMediaPause} />
                          </CButton>
                          {inCall && (
                            <CButton size="sm" color="danger" disabled={acting === a.sipNo}
                              onClick={() => onHangup(a)} title={t('monitor.hangup')}>
                              <CIcon icon={cilMediaStop} />
                            </CButton>
                          )}
                        </div>
                      </CTableDataCell>
                    )}
                  </CTableRow>
                )
              })}
              {!agents.length && (
                <CTableRow>
                  <CTableDataCell colSpan={canManage ? 6 : 5} className="text-center text-muted py-4">
                    {t('monitor.no_agents')}
                  </CTableDataCell>
                </CTableRow>
              )}
            </CTableBody>
          </CTable>
        )}
      </CCardBody>
    </CCard>
  )
}

function WaitingCallersCard({ loading, waitingCallers }) {
  const { t } = useTranslation()
  return (
    <CCard>
      <CCardHeader>{t('dashboard.waiting_title')} ({waitingCallers.length})</CCardHeader>
      <CCardBody className="p-0">
        {loading ? (
          <div className="text-center py-5"><CSpinner size="sm" /></div>
        ) : (
          <CTable hover responsive className="mb-0">
            <CTableHead>
              <CTableRow>
                <CTableHeaderCell>{t('dashboard.col_caller')}</CTableHeaderCell>
                <CTableHeaderCell>{t('dashboard.col_queue')}</CTableHeaderCell>
                <CTableHeaderCell>{t('dashboard.col_wait_time')}</CTableHeaderCell>
              </CTableRow>
            </CTableHead>
            <CTableBody>
              {waitingCallers.map((wc) => (
                <CTableRow key={wc.channel || `${wc.queue}-${wc.callerId}`}>
                  <CTableDataCell className="fw-semibold font-monospace">{wc.callerId || '—'}</CTableDataCell>
                  <CTableDataCell className="text-muted small">{wc.queue}</CTableDataCell>
                  <CTableDataCell>
                    <CBadge color={wc.waitSeconds >= 60 ? 'danger' : wc.waitSeconds >= 30 ? 'warning' : 'secondary'}>
                      {fmtWait(wc.waitSeconds)}
                    </CBadge>
                  </CTableDataCell>
                </CTableRow>
              ))}
              {!waitingCallers.length && (
                <CTableRow>
                  <CTableDataCell colSpan={3} className="text-center text-muted py-4">
                    {t('dashboard.no_waiting')}
                  </CTableDataCell>
                </CTableRow>
              )}
            </CTableBody>
          </CTable>
        )}
      </CCardBody>
    </CCard>
  )
}

export default function Dashboard() {
  const { t } = useTranslation()
  const { isSuperAdmin, isTenantAdmin, isSupervisor } = useAuthStore()
  const canManage = isSuperAdmin() || isTenantAdmin() || isSupervisor()
  const [openTickets,     setOpenTickets]     = useState(null)
  const [todayCalls,      setTodayCalls]      = useState(null)
  const [todayAnswered,   setTodayAnswered]   = useState(null)

  useEffect(() => {
    tickets.list({ status: 'open', limit: 1 })
      .then((d) => setOpenTickets(d.total ?? d.length ?? 0))
      .catch(() => setOpenTickets('—'))

    const today = new Date().toISOString().slice(0, 10)
    cdr.list({ date_from: today, limit: 1 })
      .then((d) => setTodayCalls(d.total ?? d.length ?? 0))
      .catch(() => setTodayCalls('—'))
    cdr.list({ date_from: today, limit: 1, disposition: 'ANSWERED' })
      .then((d) => setTodayAnswered(d.total ?? d.length ?? 0))
      .catch(() => setTodayAnswered('—'))
  }, [])

  const { loading, agents, waitingCallers, onlineCount, activeCallCount, waitingCount, acting, handlePause, handleHangup } = useAgentsMonitor()
  const todayMissed = (typeof todayCalls === 'number' && typeof todayAnswered === 'number')
    ? todayCalls - todayAnswered
    : null

  return (
    <div>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('dashboard.title')}</h4>
      </div>

      <CRow className="g-3">
        <CCol sm={6} xl={3}>
          <StatCard title={t('dashboard.agents_online')}  value={onlineCount}     color="success" icon="👤" />
        </CCol>
        <CCol sm={6} xl={3}>
          <StatCard title={t('dashboard.active_calls')}   value={activeCallCount} color="primary" icon="📞" />
        </CCol>
        <CCol sm={6} xl={3}>
          <StatCard title={t('dashboard.waiting')}        value={waitingCount}    color="warning" icon="⏳" />
        </CCol>
        <CCol sm={6} xl={3}>
          <StatCard title={t('dashboard.open_tickets')}   value={openTickets}     color="danger"  icon="🎫" />
        </CCol>
      </CRow>

      <CRow className="g-3 mt-1">
        <CCol xl={isSuperAdmin() ? 6 : 12}>
          <CCard className="h-100">
            <CCardBody className="py-2">
              <div className="text-muted small fw-semibold mb-1">{t('dashboard.todays_calls')}</div>
              <div className="d-flex align-items-baseline gap-2">
                <div className="fs-5 fw-bold">{todayCalls ?? <CSpinner size="sm" />}</div>
                <div className="text-muted small">{t('dashboard.total_calls_today')}</div>
              </div>
              <div className="d-flex gap-3 small mt-1">
                <span>{t('cdr.disposition_answered')}: <strong>{todayAnswered ?? '—'}</strong></span>
                <span>{t('phone.missed')}: <strong>{todayMissed ?? '—'}</strong></span>
              </div>
            </CCardBody>
          </CCard>
        </CCol>
        {isSuperAdmin() && (
          <CCol xl={6}>
            <ProvidersStatusCard />
          </CCol>
        )}
      </CRow>

      <CRow className="g-3 mt-1">
        <CCol xl={12}>
          <AgentsCard
            loading={loading} agents={agents} canManage={canManage}
            acting={acting} onPause={handlePause} onHangup={handleHangup}
          />
        </CCol>
      </CRow>

      <CRow className="g-3 mt-1">
        <CCol xl={12}>
          <WaitingCallersCard loading={loading} waitingCallers={waitingCallers} />
        </CCol>
      </CRow>

      <CRow className="g-3 mt-1">
        <CCol xl={12}>
          <TasksBoard />
        </CCol>
      </CRow>
    </div>
  )
}
