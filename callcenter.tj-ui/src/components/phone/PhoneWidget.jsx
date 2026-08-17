import React, { useEffect, useState } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { CBadge, CButton } from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPhone, cilMediaStop, cilMicrophone, cilVolumeOff, cilMediaPause, cilMediaPlay, cilX } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import usePhoneStore from 'src/store/phone'
import useAuthStore from 'src/store/auth'

const STATUS_COLOR = {
  idle:        'secondary',
  connecting:  'warning',
  registered:  'primary',
  ringing_in:  'warning',
  ringing_out: 'info',
  active:      'success',
  on_hold:     'warning',
  failed:      'danger',
}
const STATUS_LABEL_KEY = {
  idle: 'phone.status_idle', connecting: 'common.connecting', registered: 'phone.status_registered',
  ringing_in: 'phone.status_ringing_in', ringing_out: 'phone.status_ringing_out',
  active: 'phone.status_active', on_hold: 'phone.status_on_hold', failed: 'phone.status_failed',
}

function fmtDuration(s) {
  const m = Math.floor(s / 60)
  return `${String(m).padStart(2, '0')}:${String(s % 60).padStart(2, '0')}`
}

export default function PhoneWidget() {
  const { t } = useTranslation()
  const [open, setOpen] = useState(false)
  const [dial, setDial] = useState('')
  const user     = useAuthStore((s) => s.user)
  const navigate = useNavigate()
  const location = useLocation()

  const {
    status, session, remoteNumber, callDuration, isMuted, configError, retryInit,
    call, answer, hangup, toggleMute, toggleHold, sendDtmf,
  } = usePhoneStore()

  // No live JsSIP session means this call was rehydrated after a page
  // reload — the browser's WebRTC audio didn't survive that, only the
  // display + AMI-backed hangup did, so mute/hold (which need a live
  // RTCSession) aren't available.
  const reattached = !session && ['active', 'on_hold'].includes(status)

  // Auto-open on incoming call or connection failure
  useEffect(() => {
    if (status === 'ringing_in' || status === 'failed') setOpen(true)
  }, [status])

  // Don't show on /webphone page (it has its own UI for the same call)
  if (!user || user.userType === 0) return null
  if (location.pathname === '/webphone') return null

  const inCall   = ['ringing_in', 'ringing_out', 'active', 'on_hold'].includes(status)
  const fabClass = status === 'ringing_in' ? 'ringing' : inCall ? 'active' : 'idle'

  const handleDial = () => {
    if (dial.trim()) { call(dial.trim()); setDial(''); setOpen(true) }
  }
  const handleDtmf = (digit) => {
    setDial((d) => d + digit)
    if (status === 'active') sendDtmf(digit)
  }

  return (
    <div className="phone-widget">
      <button
        className={`phone-fab ${fabClass}`}
        onClick={() => inCall ? setOpen(true) : navigate('/webphone')}
        title={`${t('nav.webphone')}: ${t(STATUS_LABEL_KEY[status] ?? status)}`}
      >
        <CIcon icon={cilPhone} size="lg" style={{ color: '#fff' }} />
      </button>

      <CBadge color={STATUS_COLOR[status] ?? 'secondary'} shape="rounded-pill"
        className="position-absolute" style={{ top: 0, right: 0, fontSize: 9, pointerEvents: 'none' }}>
        {status === 'registered' ? '✓' : status === 'failed' ? '✗' : '…'}
      </CBadge>

      {/* Mini panel during active/incoming call, or when the phone connection failed */}
      {open && (inCall || status === 'failed') && (
        <div className="phone-panel">
          <div className="d-flex align-items-center justify-content-between px-3 py-2 border-bottom">
            <span className="fw-semibold small">{status === 'failed' ? t('phone.connection_title') : t('phone.call_in_progress')}</span>
            <CButton size="sm" variant="ghost" onClick={() => setOpen(false)}>
              <CIcon icon={cilX} />
            </CButton>
          </div>

          {status === 'failed' && (
            <div className="p-3 text-center">
              <div className="text-danger small mb-2">
                {configError || t('phone.connect_failed')}
              </div>
              <div className="d-flex gap-2 justify-content-center">
                <CButton color="primary" size="sm" onClick={() => retryInit?.()}>
                  {t('common.retry')}
                </CButton>
              </div>
            </div>
          )}

          {status === 'ringing_in' && (
            <div className="p-3 text-center">
              <div style={{ fontSize: 32 }}>📲</div>
              <div className="fw-bold fs-5 my-1">{remoteNumber}</div>
              <div className="d-flex gap-2 justify-content-center mt-2">
                <CButton color="success" onClick={answer}>
                  <CIcon icon={cilPhone} className="me-1" />{t('phone.answer')}
                </CButton>
                <CButton color="danger" onClick={hangup}>
                  <CIcon icon={cilMediaStop} className="me-1" />{t('phone.decline')}
                </CButton>
              </div>
            </div>
          )}

          {['ringing_out', 'active', 'on_hold'].includes(status) && (
            <div className="p-3 text-center">
              <div className="text-muted small">
                {status === 'ringing_out' ? `⏳ ${t('phone.status_ringing_out')}` : status === 'on_hold' ? `⏸ ${t('phone.status_on_hold')}` : `🔊 ${t('phone.status_active')}`}
              </div>
              <div className="fs-4 fw-bold">{remoteNumber}</div>
              {status === 'active' && <div className="text-muted small mb-2">{fmtDuration(callDuration)}</div>}
              {reattached && (
                <div className="text-warning small mb-2">
                  {t('phone.reattached_notice')}
                </div>
              )}
              <div className="d-flex gap-2 justify-content-center">
                <CButton color={isMuted ? 'warning' : 'secondary'} size="sm" onClick={toggleMute} disabled={reattached}>
                  <CIcon icon={isMuted ? cilVolumeOff : cilMicrophone} />
                </CButton>
                <CButton color={status === 'on_hold' ? 'info' : 'secondary'} size="sm" onClick={toggleHold} disabled={reattached}>
                  <CIcon icon={status === 'on_hold' ? cilMediaPlay : cilMediaPause} />
                </CButton>
                <CButton color="danger" size="sm" onClick={hangup}>
                  <CIcon icon={cilMediaStop} />
                </CButton>
              </div>
            </div>
          )}
        </div>
      )}
    </div>
  )
}
