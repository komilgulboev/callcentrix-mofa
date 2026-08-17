import React, { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import {
  CButton, CCard, CCardBody, CCol, CContainer,
  CForm, CFormInput, CInputGroup, CInputGroupText, CRow, CAlert,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilLockLocked, cilUser, cilPhone } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { auth, settings as settingsApi } from 'src/api'
import useAuthStore from 'src/store/auth'
import useBrandingStore from 'src/store/branding'

const REGISTER_ERROR_KEYS = {
  registration_disabled: 'register.registration_disabled',
  username_exists: 'register.phone_exists',
  sip_exists: 'register.phone_exists',
  invalid_code: 'register.invalid_code',
}

export default function Login() {
  const [mode, setMode] = useState('login') // 'login' | 'register' | 'verify'

  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [error,    setError]    = useState('')
  const [loading,  setLoading]  = useState(false)

  const [regForm, setRegForm] = useState({ firstName: '', lastName: '', phone: '', password: '', confirm: '' })
  const [regUsername, setRegUsername] = useState('')
  const [code, setCode] = useState('')
  const [regError, setRegError] = useState('')
  const [regLoading, setRegLoading] = useState(false)

  const platformName = useBrandingStore((s) => s.platformName)
  const systemInfo = useBrandingStore((s) => s.systemInfo)
  const hasLogo = useBrandingStore((s) => s.hasLogo)
  const updatedAt = useBrandingStore((s) => s.updatedAt)
  const registrationEnabled = useBrandingStore((s) => s.registrationEnabled)
  const setToken = useAuthStore((s) => s.setToken)
  const navigate = useNavigate()
  const { t } = useTranslation()

  const regErrorMessage = (err) => t(REGISTER_ERROR_KEYS[err.message] || 'register.generic_error')

  const handleSubmit = async (e) => {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const data = await auth.login(username, password)
      setToken(data.token || data.accessToken)
      localStorage.setItem('sipPassword', password)
      navigate('/dashboard')
    } catch (err) {
      setError(err.message || t('login.invalid_credentials'))
    } finally {
      setLoading(false)
    }
  }

  const handleRegister = async (e) => {
    e.preventDefault()
    setRegError('')
    if (regForm.password !== regForm.confirm) {
      setRegError(t('register.passwords_mismatch'))
      return
    }
    setRegLoading(true)
    try {
      const data = await auth.register({
        firstName: regForm.firstName,
        lastName: regForm.lastName,
        phone: regForm.phone,
        password: regForm.password,
      })
      setRegUsername(data.username)
      setMode('verify')
    } catch (err) {
      setRegError(regErrorMessage(err))
    } finally {
      setRegLoading(false)
    }
  }

  const handleVerify = async (e) => {
    e.preventDefault()
    setRegError('')
    setRegLoading(true)
    try {
      const data = await auth.verifyCode(regUsername, code)
      setToken(data.token || data.accessToken)
      localStorage.setItem('sipPassword', regForm.password)
      navigate('/dashboard')
    } catch (err) {
      setRegError(regErrorMessage(err))
    } finally {
      setRegLoading(false)
    }
  }

  const logoSrc = hasLogo ? `${settingsApi.logoUrl()}?v=${encodeURIComponent(updatedAt)}` : null

  return (
    <div className="bg-body-tertiary min-vh-100">
      <CRow className="g-0 min-vh-100">
        <CCol md={6} className="d-none d-md-flex flex-column align-items-center justify-content-center text-white p-5 bg-dark">
          {logoSrc
            ? <img src={logoSrc} alt={platformName} style={{ maxWidth: 180, maxHeight: 180, objectFit: 'contain' }} className="mb-4" />
            : <div className="mb-4" style={{ fontSize: '4rem' }}>📞</div>}
          <h1 className="fw-bold text-center">{platformName}</h1>
          {systemInfo && (
            <p className="text-center text-white-50 mt-3" style={{ whiteSpace: 'pre-line', maxWidth: 420 }}>
              {systemInfo}
            </p>
          )}
        </CCol>

        <CCol xs={12} md={6} className="d-flex align-items-center justify-content-center p-4">
          <CContainer>
            <CRow className="justify-content-center">
              <CCol xs={12} sm={9} lg={7}>
                <div className="text-center mb-4 d-md-none">
                  {logoSrc
                    ? <img src={logoSrc} alt={platformName} style={{ maxWidth: 96, maxHeight: 96, objectFit: 'contain' }} className="mb-2" />
                    : <div className="mb-2" style={{ fontSize: '2.5rem' }}>📞</div>}
                  <h2 className="fw-bold">{platformName}</h2>
                </div>

                {mode === 'login' && (
                  <>
                    <div className="text-center mb-4">
                      <p className="text-muted">{t('login.subtitle')}</p>
                    </div>
                    <CCard className="shadow-sm">
                      <CCardBody className="p-4">
                        {error && <CAlert color="danger">{error}</CAlert>}
                        <CForm onSubmit={handleSubmit}>
                          <CInputGroup className="mb-3">
                            <CInputGroupText><CIcon icon={cilUser} /></CInputGroupText>
                            <CFormInput
                              placeholder={t('login.username_placeholder')}
                              autoComplete="username"
                              value={username}
                              onChange={(e) => setUsername(e.target.value)}
                              required
                            />
                          </CInputGroup>
                          <CInputGroup className="mb-4">
                            <CInputGroupText><CIcon icon={cilLockLocked} /></CInputGroupText>
                            <CFormInput
                              type="password"
                              placeholder={t('login.password_placeholder')}
                              autoComplete="current-password"
                              value={password}
                              onChange={(e) => setPassword(e.target.value)}
                              required
                            />
                          </CInputGroup>
                          <CButton type="submit" color="primary" className="w-100" disabled={loading}>
                            {loading ? t('login.signing_in') : t('login.sign_in')}
                          </CButton>
                        </CForm>
                        {registrationEnabled && (
                          <div className="text-center mt-3">
                            <CButton color="link" size="sm" onClick={() => { setMode('register'); setError(''); setRegError('') }}>
                              {t('register.switch_to_register')}
                            </CButton>
                          </div>
                        )}
                      </CCardBody>
                    </CCard>
                  </>
                )}

                {mode === 'register' && (
                  <>
                    <div className="text-center mb-4">
                      <p className="text-muted">{t('register.subtitle')}</p>
                    </div>
                    <CCard className="shadow-sm">
                      <CCardBody className="p-4">
                        {regError && <CAlert color="danger">{regError}</CAlert>}
                        <CForm onSubmit={handleRegister} className="d-flex flex-column gap-3">
                          <div className="row g-2">
                            <div className="col">
                              <CFormInput
                                placeholder={t('register.first_name')}
                                value={regForm.firstName}
                                onChange={(e) => setRegForm({ ...regForm, firstName: e.target.value })}
                                required
                              />
                            </div>
                            <div className="col">
                              <CFormInput
                                placeholder={t('register.last_name')}
                                value={regForm.lastName}
                                onChange={(e) => setRegForm({ ...regForm, lastName: e.target.value })}
                                required
                              />
                            </div>
                          </div>
                          <CInputGroup>
                            <CInputGroupText><CIcon icon={cilPhone} /></CInputGroupText>
                            <CFormInput
                              placeholder={t('register.phone_placeholder')}
                              value={regForm.phone}
                              onChange={(e) => setRegForm({ ...regForm, phone: e.target.value })}
                              required
                            />
                          </CInputGroup>
                          <CInputGroup>
                            <CInputGroupText><CIcon icon={cilLockLocked} /></CInputGroupText>
                            <CFormInput
                              type="password"
                              placeholder={t('login.password_placeholder')}
                              value={regForm.password}
                              onChange={(e) => setRegForm({ ...regForm, password: e.target.value })}
                              required
                            />
                          </CInputGroup>
                          <CInputGroup>
                            <CInputGroupText><CIcon icon={cilLockLocked} /></CInputGroupText>
                            <CFormInput
                              type="password"
                              placeholder={t('register.confirm_password_placeholder')}
                              value={regForm.confirm}
                              onChange={(e) => setRegForm({ ...regForm, confirm: e.target.value })}
                              required
                            />
                          </CInputGroup>
                          <CButton type="submit" color="primary" className="w-100" disabled={regLoading}>
                            {regLoading ? t('register.submitting') : t('register.submit')}
                          </CButton>
                        </CForm>
                        <div className="text-center mt-3">
                          <CButton color="link" size="sm" onClick={() => { setMode('login'); setRegError('') }}>
                            {t('register.switch_to_login')}
                          </CButton>
                        </div>
                      </CCardBody>
                    </CCard>
                  </>
                )}

                {mode === 'verify' && (
                  <>
                    <div className="text-center mb-4">
                      <p className="text-muted">{t('register.verify_subtitle', { phone: regUsername })}</p>
                    </div>
                    <CCard className="shadow-sm">
                      <CCardBody className="p-4">
                        {regError && <CAlert color="danger">{regError}</CAlert>}
                        <CForm onSubmit={handleVerify} className="d-flex flex-column gap-3">
                          <CFormInput
                            placeholder={t('register.code_placeholder')}
                            value={code}
                            onChange={(e) => setCode(e.target.value)}
                            autoComplete="one-time-code"
                            required
                          />
                          <CButton type="submit" color="primary" className="w-100" disabled={regLoading}>
                            {regLoading ? t('register.submitting') : t('register.verify_submit')}
                          </CButton>
                        </CForm>
                      </CCardBody>
                    </CCard>
                  </>
                )}
              </CCol>
            </CRow>
          </CContainer>
        </CCol>
      </CRow>
    </div>
  )
}
