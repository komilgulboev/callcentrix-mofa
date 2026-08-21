import React, { useEffect, useRef, useState } from 'react'
import {
  CCard, CCardBody, CCardHeader, CForm, CFormInput, CFormTextarea,
  CFormLabel, CFormSwitch, CButton, CAlert, CSpinner,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilCloudUpload } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import useAuthStore from 'src/store/auth'
import useBrandingStore from 'src/store/branding'
import { auth as authApi, settings as settingsApi } from 'src/api'

const PASSWORD_ERROR_KEYS = {
  invalid_current_password: 'settings.invalid_current_password',
  password_too_short:       'settings.password_too_short',
}

export default function Settings() {
  const isSuperAdmin = useAuthStore((s) => s.isSuperAdmin())
  const { t } = useTranslation()
  const [current,  setCurrent]  = useState('')
  const [pwd,      setPwd]      = useState('')
  const [confirm,  setConfirm]  = useState('')
  const [error,    setError]    = useState('')
  const [success,  setSuccess]  = useState('')
  const [saving,   setSaving]   = useState(false)

  const handleChangePwd = async (e) => {
    e.preventDefault()
    if (pwd !== confirm) { setError(t('settings.passwords_mismatch')); return }
    if (pwd.length < 6)  { setError(t('settings.password_too_short')); return }
    setSaving(true)
    setError('')
    try {
      await authApi.changePassword(current, pwd)
      setSuccess(t('settings.password_updated'))
      setCurrent('')
      setPwd('')
      setConfirm('')
    } catch (e) { setError(t(PASSWORD_ERROR_KEYS[e.message] || 'settings.password_change_failed')) }
    finally { setSaving(false) }
  }

  return (
    <>
      <h4 className="mb-4">{t('settings.title')}</h4>

      <div className="d-flex flex-column gap-4">
        {isSuperAdmin && <BrandingCard t={t} />}
        {isSuperAdmin && <SmppCard t={t} />}
        {isSuperAdmin && <TelegramCard t={t} />}

        <CCard style={{ maxWidth: 480 }}>
          <CCardHeader>{t('settings.change_password')}</CCardHeader>
          <CCardBody>
            {error   && <CAlert color="danger"  dismissible onClose={() => setError('')}>{error}</CAlert>}
            {success && <CAlert color="success" dismissible onClose={() => setSuccess('')}>{success}</CAlert>}
            <CForm onSubmit={handleChangePwd} className="d-flex flex-column gap-3">
              <div>
                <CFormLabel>{t('settings.current_password')}</CFormLabel>
                <CFormInput type="password" autoComplete="current-password"
                  value={current} onChange={(e) => setCurrent(e.target.value)} required />
              </div>
              <div>
                <CFormLabel>{t('settings.new_password')}</CFormLabel>
                <CFormInput type="password" autoComplete="new-password"
                  value={pwd} onChange={(e) => setPwd(e.target.value)} required />
              </div>
              <div>
                <CFormLabel>{t('settings.confirm_password')}</CFormLabel>
                <CFormInput type="password" value={confirm} onChange={(e) => setConfirm(e.target.value)} required />
              </div>
              <CButton type="submit" color="primary" disabled={saving}>
                {saving ? <CSpinner size="sm" /> : t('settings.update_password')}
              </CButton>
            </CForm>
          </CCardBody>
        </CCard>
      </div>
    </>
  )
}

function BrandingCard({ t }) {
  const fileRef = useRef(null)
  const [platformName, setPlatformName] = useState('')
  const [systemInfo,   setSystemInfo]   = useState('')
  const [hasLogo,      setHasLogo]      = useState(false)
  const [updatedAt,    setUpdatedAt]    = useState('')
  const [registrationEnabled, setRegistrationEnabled] = useState(false)
  const [loading,      setLoading]      = useState(true)
  const [saving,       setSaving]       = useState(false)
  const [uploading,    setUploading]    = useState(false)
  const [error,        setError]        = useState('')
  const [success,      setSuccess]      = useState('')

  const load = () => {
    settingsApi.branding()
      .then((b) => {
        setPlatformName(b.platformName || '')
        setSystemInfo(b.systemInfo || '')
        setHasLogo(!!b.hasLogo)
        setUpdatedAt(b.updatedAt || '')
        setRegistrationEnabled(!!b.registrationEnabled)
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const handleSave = async (e) => {
    e.preventDefault()
    setSaving(true); setError(''); setSuccess('')
    try {
      await settingsApi.updateBranding({ platformName, systemInfo, registrationEnabled })
      useBrandingStore.getState().load()
      setSuccess(t('settings.branding_saved'))
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleUploadLogo = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    setUploading(true); setError(''); setSuccess('')
    try {
      await settingsApi.uploadLogo(file)
      load()
      useBrandingStore.getState().load()
      setSuccess(t('settings.branding_saved'))
    } catch (e) { setError(e.message) }
    finally { setUploading(false); e.target.value = '' }
  }

  const logoSrc = hasLogo ? `${settingsApi.logoUrl()}?v=${encodeURIComponent(updatedAt)}` : null

  return (
    <CCard style={{ maxWidth: 480 }}>
      <CCardHeader>{t('settings.branding_title')}</CCardHeader>
      <CCardBody>
        {error   && <CAlert color="danger"  dismissible onClose={() => setError('')}>{error}</CAlert>}
        {success && <CAlert color="success" dismissible onClose={() => setSuccess('')}>{success}</CAlert>}

        {loading ? <CSpinner size="sm" /> : (
          <>
            <div className="mb-3">
              <CFormLabel>{t('settings.logo')}</CFormLabel>
              <div className="d-flex align-items-center gap-3">
                {logoSrc
                  ? <img src={logoSrc} alt="" style={{ width: 64, height: 64, objectFit: 'contain' }} className="border rounded p-1" />
                  : <span className="text-muted">{t('settings.no_logo')}</span>}
                <CButton color="secondary" variant="outline" onClick={() => fileRef.current?.click()} disabled={uploading}>
                  {uploading
                    ? <CSpinner size="sm" className="me-2" />
                    : <CIcon icon={cilCloudUpload} className="me-2" />}
                  {uploading ? t('settings.uploading') : (hasLogo ? t('settings.change_logo') : t('settings.upload_logo'))}
                </CButton>
                <input ref={fileRef} type="file" accept=".png,.jpg,.jpeg,.svg,.webp"
                  className="d-none" onChange={handleUploadLogo} />
              </div>
            </div>

            <CForm onSubmit={handleSave} className="d-flex flex-column gap-3">
              <div>
                <CFormLabel>{t('settings.platform_name')}</CFormLabel>
                <CFormInput value={platformName} onChange={(e) => setPlatformName(e.target.value)} required />
              </div>
              <div>
                <CFormLabel>{t('settings.system_info')}</CFormLabel>
                <CFormTextarea rows={4} value={systemInfo} onChange={(e) => setSystemInfo(e.target.value)} />
              </div>
              <CFormSwitch
                label={t('settings.registration_enabled')}
                checked={registrationEnabled}
                onChange={(e) => setRegistrationEnabled(e.target.checked)}
              />
              <CButton type="submit" color="primary" disabled={saving}>
                {saving ? <CSpinner size="sm" /> : t('common.save')}
              </CButton>
            </CForm>
          </>
        )}
      </CCardBody>
    </CCard>
  )
}

function SmppCard({ t }) {
  const [host,     setHost]     = useState('')
  const [port,     setPort]     = useState(2775)
  const [systemId, setSystemId] = useState('')
  const [password, setPassword] = useState('')
  const [senderId, setSenderId] = useState('')
  const [hasPassword, setHasPassword] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving,  setSaving]  = useState(false)
  const [error,   setError]   = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    settingsApi.smpp()
      .then((s) => {
        setHost(s.host || '')
        setPort(s.port || 2775)
        setSystemId(s.systemId || '')
        setSenderId(s.senderId || '')
        setHasPassword(!!s.hasPassword)
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const handleSave = async (e) => {
    e.preventDefault()
    setSaving(true); setError(''); setSuccess('')
    try {
      await settingsApi.updateSmpp({ host, port: Number(port), systemId, password, senderId })
      setPassword('')
      setHasPassword(hasPassword || password !== '')
      setSuccess(t('settings.smpp_saved'))
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  return (
    <CCard style={{ maxWidth: 480 }}>
      <CCardHeader>{t('settings.smpp_title')}</CCardHeader>
      <CCardBody>
        {error   && <CAlert color="danger"  dismissible onClose={() => setError('')}>{error}</CAlert>}
        {success && <CAlert color="success" dismissible onClose={() => setSuccess('')}>{success}</CAlert>}

        {loading ? <CSpinner size="sm" /> : (
          <CForm onSubmit={handleSave} className="d-flex flex-column gap-3">
            <div className="row g-2">
              <div className="col-8">
                <CFormLabel>{t('settings.smpp_host')}</CFormLabel>
                <CFormInput value={host} onChange={(e) => setHost(e.target.value)} placeholder="smpp.example.com" required />
              </div>
              <div className="col-4">
                <CFormLabel>{t('settings.smpp_port')}</CFormLabel>
                <CFormInput type="number" value={port} onChange={(e) => setPort(e.target.value)} required />
              </div>
            </div>
            <div>
              <CFormLabel>{t('settings.smpp_login')}</CFormLabel>
              <CFormInput value={systemId} onChange={(e) => setSystemId(e.target.value)} required />
            </div>
            <div>
              <CFormLabel>{t('settings.smpp_password')}</CFormLabel>
              <CFormInput
                type="password"
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder={hasPassword ? '••••••' : ''}
              />
            </div>
            <div>
              <CFormLabel>{t('settings.smpp_sender_id')}</CFormLabel>
              <CFormInput value={senderId} onChange={(e) => setSenderId(e.target.value)} placeholder="CallCentrix" />
            </div>
            <CButton type="submit" color="primary" disabled={saving}>
              {saving ? <CSpinner size="sm" /> : t('common.save')}
            </CButton>
          </CForm>
        )}
      </CCardBody>
    </CCard>
  )
}

function TelegramCard({ t }) {
  const [botToken,    setBotToken]    = useState('')
  const [hasBotToken, setHasBotToken] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving,  setSaving]  = useState(false)
  const [error,   setError]   = useState('')
  const [success, setSuccess] = useState('')

  useEffect(() => {
    settingsApi.telegram()
      .then((s) => setHasBotToken(!!s.hasBotToken))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  const handleSave = async (e) => {
    e.preventDefault()
    setSaving(true); setError(''); setSuccess('')
    try {
      await settingsApi.updateTelegram({ botToken })
      setBotToken('')
      setHasBotToken(hasBotToken || botToken !== '')
      setSuccess(t('settings.telegram_saved'))
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  return (
    <CCard style={{ maxWidth: 480 }}>
      <CCardHeader>{t('settings.telegram_title')}</CCardHeader>
      <CCardBody>
        {error   && <CAlert color="danger"  dismissible onClose={() => setError('')}>{error}</CAlert>}
        {success && <CAlert color="success" dismissible onClose={() => setSuccess('')}>{success}</CAlert>}

        {loading ? <CSpinner size="sm" /> : (
          <CForm onSubmit={handleSave} className="d-flex flex-column gap-3">
            <div>
              <CFormLabel>{t('settings.telegram_bot_token')}</CFormLabel>
              <CFormInput
                type="password"
                value={botToken}
                onChange={(e) => setBotToken(e.target.value)}
                placeholder={hasBotToken ? '••••••' : '123456:ABC-DEF...'}
              />
              <div className="form-text">{t('settings.telegram_bot_token_hint')}</div>
            </div>
            <CButton type="submit" color="primary" disabled={saving}>
              {saving ? <CSpinner size="sm" /> : t('common.save')}
            </CButton>
          </CForm>
        )}
      </CCardBody>
    </CCard>
  )
}
