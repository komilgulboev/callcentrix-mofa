import React, { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { CAlert, CButton, CCard, CCardBody, CCardHeader, CBadge, CSpinner } from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilArrowLeft, cilPencil, cilTrash } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import DOMPurify from 'dompurify'
import { knowledgeBase as kbApi } from 'src/api'
import { catName } from './KnowledgeBase'
import useAuthStore from 'src/store/auth'

export default function KnowledgeBaseArticle() {
  const { id } = useParams()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const user = useAuthStore((s) => s.user)

  const [article, setArticle] = useState(null)
  const [category, setCategory] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    setLoading(true)
    kbApi.article(id)
      .then((a) => {
        setArticle(a)
        if (a.categoryId) {
          kbApi.categories()
            .then((d) => setCategory((d.categories ?? []).find((c) => c.id === a.categoryId) || null))
            .catch(() => {})
        }
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  const canEdit = !!article && user?.userType === 1 && user?.tenantId === article.tenantId

  const handleDelete = async () => {
    if (!confirm(t('knowledge_base.delete_confirm_article'))) return
    try {
      await kbApi.removeArticle(id)
      navigate('/knowledge-base')
    } catch (e) { setError(e.message) }
  }

  if (loading) return <div className="text-center py-5"><CSpinner /></div>
  if (!article) return <CAlert color="danger">{error || t('knowledge_base.not_found')}</CAlert>

  return (
    <>
      <div className="d-flex align-items-center gap-3 mb-4 flex-wrap">
        <CButton color="light" onClick={() => navigate(-1)}>
          <CIcon icon={cilArrowLeft} />
        </CButton>
        <h4 className="mb-0">{article.title}</h4>
        {canEdit && (
          <div className="ms-auto d-flex gap-2">
            <CButton color="light" onClick={() => navigate(`/knowledge-base/article/${id}/edit`)}>
              <CIcon icon={cilPencil} className="me-1" />{t('common.edit')}
            </CButton>
            <CButton color="danger" onClick={handleDelete}>
              <CIcon icon={cilTrash} className="me-1" />{t('common.delete')}
            </CButton>
          </div>
        )}
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <CCard>
        <CCardHeader className="d-flex align-items-center justify-content-between flex-wrap gap-2">
          <div className="d-flex align-items-center gap-2 flex-wrap">
            {category && <CBadge color="secondary">{catName(category, i18n.language)}</CBadge>}
            {(article.tags || []).map((tg) => (
              <CBadge
                key={tg}
                color="info"
                style={{ cursor: 'pointer' }}
                onClick={() => navigate(`/knowledge-base?tag=${encodeURIComponent(tg)}`)}
              >
                #{tg}
              </CBadge>
            ))}
          </div>
          <span className="text-muted small">
            {t('knowledge_base.created_by')}: {article.createdByName || '—'} · {new Date(article.createdAt).toLocaleDateString()}
          </span>
        </CCardHeader>
        <CCardBody>
          {/* article.body is HTML from the rich-text editor (see
              KnowledgeBaseEditor) — already sanitized server-side on save
              (bluemonday), re-sanitized here too as defense-in-depth around
              dangerouslySetInnerHTML. Inline photos live directly in this
              markup; only videos get a separate gallery below, since they
              aren't embedded inline. */}
          <div
            className="kb-article-body"
            dangerouslySetInnerHTML={{ __html: DOMPurify.sanitize(article.body || '') }}
          />
          {(article.media || []).filter((m) => m.type === 'video').length > 0 && (
            <div className="d-flex flex-wrap gap-2 mt-3">
              {article.media.filter((m) => m.type === 'video').map((m) => (
                <video
                  key={m.id}
                  src={kbApi.mediaUrl(id, m.id)}
                  controls
                  style={{ width: 260, borderRadius: 6 }}
                />
              ))}
            </div>
          )}
        </CCardBody>
      </CCard>
    </>
  )
}
