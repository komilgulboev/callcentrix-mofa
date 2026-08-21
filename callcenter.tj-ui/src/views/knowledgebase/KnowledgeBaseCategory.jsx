import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { CButton, CCard, CCardBody, CAlert, CSpinner, CBadge, CRow, CCol } from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilArrowLeft, cilPlus } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { knowledgeBase as kbApi } from 'src/api'
import { catName } from './KnowledgeBase'
import useAuthStore from 'src/store/auth'

export default function KnowledgeBaseCategory() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const { isTenantAdmin } = useAuthStore()
  const tenantAdmin = isTenantAdmin()

  const [category, setCategory] = useState(null)
  const [articles, setArticles] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setLoading(true)
    Promise.all([kbApi.categories(), kbApi.articles({ categoryId: id })])
      .then(([cats, arts]) => {
        setCategory((cats.categories ?? []).find((c) => String(c.id) === String(id)) || null)
        setArticles(arts.articles ?? [])
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  return (
    <>
      <div className="d-flex align-items-center gap-3 mb-4 flex-wrap">
        <CButton color="light" onClick={() => navigate('/knowledge-base')}>
          <CIcon icon={cilArrowLeft} />
        </CButton>
        <h4 className="mb-0">{category ? catName(category, i18n.language) : t('knowledge_base.title')}</h4>
        {tenantAdmin && (
          <CButton color="primary" className="ms-auto" onClick={() => navigate(`/knowledge-base/new?categoryId=${id}`)}>
            <CIcon icon={cilPlus} className="me-2" />{t('knowledge_base.new_article')}
          </CButton>
        )}
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      {loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : articles.length === 0 ? (
        <div className="text-center text-muted py-5">{t('knowledge_base.no_articles')}</div>
      ) : (
        <CRow className="g-3">
          {articles.map((a) => (
            <CCol md={6} lg={4} key={a.id}>
              <CCard className="h-100" style={{ cursor: 'pointer' }} onClick={() => navigate(`/knowledge-base/article/${a.id}`)}>
                <CCardBody>
                  <div className="fw-semibold mb-2">{a.title}</div>
                  <div className="d-flex flex-wrap gap-1 mb-2">
                    {(a.tags || []).map((tg) => (
                      <CBadge key={tg} color="light" className="text-dark border">#{tg}</CBadge>
                    ))}
                  </div>
                  <div className="text-muted small">{new Date(a.updatedAt).toLocaleDateString()}</div>
                </CCardBody>
              </CCard>
            </CCol>
          ))}
        </CRow>
      )}
    </>
  )
}
