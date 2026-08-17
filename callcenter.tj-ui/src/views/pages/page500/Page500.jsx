import React from 'react'
import { useNavigate } from 'react-router-dom'
import { CButton, CCol, CContainer, CRow } from '@coreui/react'
import { useTranslation } from 'react-i18next'

export default function Page500() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  return (
    <div className="bg-body-tertiary min-vh-100 d-flex align-items-center">
      <CContainer>
        <CRow className="justify-content-center text-center">
          <CCol md={6}>
            <h1 className="display-1 fw-bold text-danger">500</h1>
            <h4>{t('pages.server_error_title')}</h4>
            <p className="text-muted">{t('pages.server_error_body')}</p>
            <CButton color="primary" onClick={() => navigate('/dashboard')}>{t('pages.back_to_dashboard')}</CButton>
          </CCol>
        </CRow>
      </CContainer>
    </div>
  )
}
