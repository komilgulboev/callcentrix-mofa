import React, { useEffect, useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import {
  CButton, CCard, CCardBody, CModal, CModalBody, CModalFooter,
  CModalHeader, CModalTitle, CForm, CFormInput, CFormLabel,
  CInputGroup, CInputGroupText, CAlert, CSpinner, CBadge, CRow, CCol,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilPencil, cilTrash, cilSearch, cilFolder } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { knowledgeBase as kbApi } from 'src/api'
import useAuthStore from 'src/store/auth'

const LANGS = ['ru', 'tj', 'en']
const EMPTY_NAMES = { ru: '', tj: '', en: '' }

export function catName(cat, lang) {
  if (!cat?.names) return '—'
  return cat.names[lang] || cat.names.ru || cat.names.tj || cat.names.en || '—'
}

export default function KnowledgeBase() {
  const { isSuperAdmin, isTenantAdmin } = useAuthStore()
  const { t, i18n } = useTranslation()
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const superAdmin = isSuperAdmin()
  const tenantAdmin = isTenantAdmin()
  const lang = i18n.language

  const [categories, setCategories] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [search, setSearch] = useState('')
  const [tagFilter, setTagFilter] = useState(searchParams.get('tag') || '')
  const [articles, setArticles] = useState([])
  const [articlesLoading, setArticlesLoading] = useState(false)

  const [modal, setModal] = useState(false)
  const [editing, setEditing] = useState(null)
  const [form, setForm] = useState({ names: { ...EMPTY_NAMES } })
  const [saving, setSaving] = useState(false)

  const loadCategories = () => {
    setLoading(true)
    kbApi.categories()
      .then((d) => setCategories(d.categories ?? []))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }
  useEffect(loadCategories, [])

  useEffect(() => {
    const tag = searchParams.get('tag') || ''
    setTagFilter(tag)
    if (tag) setSearch('')
  }, [searchParams])

  const isFiltering = !!(search.trim() || tagFilter)

  useEffect(() => {
    if (!isFiltering) { setArticles([]); return }
    setArticlesLoading(true)
    kbApi.articles({ search: search.trim() || undefined, tag: tagFilter || undefined })
      .then((d) => setArticles(d.articles ?? []))
      .catch((e) => setError(e.message))
      .finally(() => setArticlesLoading(false))
  }, [search, tagFilter])

  const clearTagFilter = () => {
    setTagFilter('')
    setSearchParams({})
  }

  const openCreate = () => {
    setEditing(null)
    setForm({ names: { ...EMPTY_NAMES } })
    setModal(true)
  }
  const openEdit = (cat) => {
    setEditing(cat)
    setForm({ names: { ...EMPTY_NAMES, ...cat.names } })
    setModal(true)
  }
  const setName = (l, val) => setForm((f) => ({ ...f, names: { ...f.names, [l]: val } }))

  const handleSave = async () => {
    setSaving(true)
    try {
      if (editing) await kbApi.updateCategory(editing.id, { names: form.names })
      else await kbApi.createCategory({ names: form.names })
      setModal(false)
      loadCategories()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('knowledge_base.delete_confirm_category'))) return
    try { await kbApi.removeCategory(id); loadCategories() }
    catch (e) { setError(e.message) }
  }

  const hasName = LANGS.some((l) => form.names[l].trim())

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4 flex-wrap gap-2">
        <h4 className="mb-0">{t('knowledge_base.title')}</h4>
        <div className="d-flex gap-2">
          {tenantAdmin && (
            <CButton color="primary" onClick={() => navigate('/knowledge-base/new')}>
              <CIcon icon={cilPlus} className="me-2" />{t('knowledge_base.new_article')}
            </CButton>
          )}
          {superAdmin && (
            <CButton color="secondary" onClick={openCreate}>
              <CIcon icon={cilPlus} className="me-2" />{t('knowledge_base.new_category')}
            </CButton>
          )}
        </div>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <CInputGroup className="mb-4" style={{ maxWidth: 420 }}>
        <CInputGroupText><CIcon icon={cilSearch} /></CInputGroupText>
        <CFormInput
          placeholder={t('knowledge_base.search_placeholder')}
          value={search}
          onChange={(e) => { setSearch(e.target.value); setTagFilter('') }}
        />
      </CInputGroup>

      {tagFilter && (
        <div className="mb-3 d-flex align-items-center gap-2">
          <CBadge color="info">#{tagFilter}</CBadge>
          <span className="text-muted small">{t('knowledge_base.filtered_by_tag')}</span>
          <CButton size="sm" color="link" onClick={clearTagFilter}>{t('knowledge_base.clear_filter')}</CButton>
        </div>
      )}

      {isFiltering ? (
        articlesLoading ? (
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
                    <div className="d-flex flex-wrap gap-1">
                      {(a.tags || []).map((tg) => (
                        <CBadge key={tg} color="light" className="text-dark border">#{tg}</CBadge>
                      ))}
                    </div>
                  </CCardBody>
                </CCard>
              </CCol>
            ))}
          </CRow>
        )
      ) : loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : categories.length === 0 ? (
        <div className="text-center text-muted py-5">{t('knowledge_base.no_categories')}</div>
      ) : (
        <CRow className="g-3">
          {categories.map((cat) => (
            <CCol md={6} lg={4} key={cat.id}>
              <CCard className="h-100">
                <CCardBody className="d-flex flex-column">
                  <div className="d-flex align-items-start justify-content-between">
                    <div
                      className="d-flex align-items-center gap-2"
                      style={{ cursor: 'pointer' }}
                      onClick={() => navigate(`/knowledge-base/category/${cat.id}`)}
                    >
                      <CIcon icon={cilFolder} className="text-primary" />
                      <span className="fw-semibold">{catName(cat, lang)}</span>
                    </div>
                    {superAdmin && (
                      <div className="d-flex gap-1">
                        <CButton size="sm" color="light" onClick={() => openEdit(cat)}>
                          <CIcon icon={cilPencil} />
                        </CButton>
                        <CButton size="sm" color="danger" onClick={() => handleDelete(cat.id)}>
                          <CIcon icon={cilTrash} />
                        </CButton>
                      </div>
                    )}
                  </div>
                  <div className="text-muted small mt-2">
                    {t('knowledge_base.articles_count', { count: cat.articleCount })}
                  </div>
                </CCardBody>
              </CCard>
            </CCol>
          ))}
        </CRow>
      )}

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader>
          <CModalTitle>{editing ? t('knowledge_base.edit_category') : t('knowledge_base.new_category')}</CModalTitle>
        </CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            {LANGS.map((l) => (
              <div key={l}>
                <CFormLabel>{t('topics.name_in_language', { lang: t(`header.lang_${l}`) })}</CFormLabel>
                <CFormInput value={form.names[l]} onChange={(e) => setName(l, e.target.value)} />
              </div>
            ))}
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving || !hasName}>
            {saving ? <CSpinner size="sm" /> : t('common.save')}
          </CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
