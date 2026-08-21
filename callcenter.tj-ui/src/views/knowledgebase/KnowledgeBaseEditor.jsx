import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import {
  CAlert, CButton, CCard, CCardBody, CForm, CFormInput, CFormLabel,
  CFormSelect, CFormCheck, CSpinner,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilArrowLeft } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import ReactQuill from 'react-quill-new'
import 'react-quill-new/dist/quill.snow.css'
import { knowledgeBase as kbApi, tasks as tasksApi } from 'src/api'
import { catName } from './KnowledgeBase'

export default function KnowledgeBaseEditor() {
  const { id } = useParams() // present once the article has been saved at least once
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const { t, i18n } = useTranslation()
  const editing = !!id
  const quillRef = useRef(null)

  const [categories, setCategories] = useState([])
  const [title, setTitle] = useState('')
  const [categoryId, setCategoryId] = useState(searchParams.get('categoryId') || '')
  const [tagsText, setTagsText] = useState('')
  const [body, setBody] = useState('')
  const [videos, setVideos] = useState([])
  const [uploadingVideo, setUploadingVideo] = useState(false)
  const [visibleToAll, setVisibleToAll] = useState(true)
  const [allowedUserIds, setAllowedUserIds] = useState([])
  const [assignableUsers, setAssignableUsers] = useState([])
  const [loading, setLoading] = useState(editing)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    kbApi.categories().then((d) => setCategories(d.categories ?? [])).catch(() => {})
    tasksApi.assignableUsers().then((d) => setAssignableUsers(d.users ?? [])).catch(() => {})
  }, [])

  useEffect(() => {
    if (!editing) return
    kbApi.article(id)
      .then((a) => {
        setTitle(a.title)
        setCategoryId(a.categoryId ? String(a.categoryId) : '')
        setTagsText((a.tags || []).join(', '))
        setBody(a.body || '')
        setVideos((a.media || []).filter((m) => m.type === 'video'))
        setVisibleToAll(a.visibleToAll ?? true)
        setAllowedUserIds(a.allowedUserIds || [])
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [id])

  // Inline photos are uploaded through the same MinIO-backed media endpoint
  // as the video attachments below — the article just has to exist first
  // (an image needs an articleId to attach to), so on a brand-new article
  // this tells the admin to save once before the toolbar's image button can
  // do anything, rather than silently failing.
  const imageHandler = useCallback(() => {
    if (!id) {
      setError(t('knowledge_base.media_hint_save_first'))
      return
    }
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.onchange = async () => {
      const file = input.files[0]
      if (!file) return
      setError('')
      try {
        const res = await kbApi.uploadMedia(id, file, 'photo')
        const url = kbApi.mediaUrl(id, res.id)
        const quill = quillRef.current?.getEditor()
        const range = quill?.getSelection(true)
        const index = range ? range.index : quill.getLength()
        quill.insertEmbed(index, 'image', url, 'user')
        quill.setSelection(index + 1)
      } catch (err) { setError(err.message) }
    }
    input.click()
  }, [id, t])

  const quillModules = useMemo(() => ({
    toolbar: {
      container: [
        [{ header: [1, 2, 3, false] }],
        ['bold', 'italic', 'underline', 'strike'],
        [{ list: 'ordered' }, { list: 'bullet' }],
        ['blockquote', 'link', 'image'],
        ['clean'],
      ],
      handlers: { image: imageHandler },
    },
  }), [imageHandler])

  const handleVideoChange = async (e) => {
    const file = e.target.files[0]
    e.target.value = ''
    if (!file) return
    setUploadingVideo(true)
    setError('')
    try {
      const res = await kbApi.uploadMedia(id, file, 'video')
      setVideos((v) => [...v, { id: res.id, type: 'video' }])
    } catch (err) { setError(err.message) }
    finally { setUploadingVideo(false) }
  }

  const handleVideoDelete = async (mediaId) => {
    try {
      await kbApi.removeMedia(id, mediaId)
      setVideos((v) => v.filter((x) => x.id !== mediaId))
    } catch (err) { setError(err.message) }
  }

  const toggleAllowedUser = (userId) => {
    setAllowedUserIds((ids) => (
      ids.includes(userId) ? ids.filter((x) => x !== userId) : [...ids, userId]
    ))
  }

  const handleSave = async () => {
    setSaving(true)
    setError('')
    try {
      const tags = tagsText.split(/[,\s]+/).map((s) => s.trim()).filter(Boolean)
      const payload = {
        title: title.trim(),
        body,
        categoryId: categoryId ? Number(categoryId) : null,
        tags,
        visibleToAll,
        allowedUserIds: visibleToAll ? [] : allowedUserIds,
      }
      if (editing) {
        await kbApi.updateArticle(id, payload)
        navigate(`/knowledge-base/article/${id}`)
      } else {
        const res = await kbApi.createArticle(payload)
        // Stay in the editor (now in "editing" mode) instead of jumping to
        // the detail page — the article needs to exist before inline images
        // can be inserted, so this lets the admin keep going without an
        // extra click back into Edit.
        navigate(`/knowledge-base/article/${res.id}/edit`, { replace: true })
      }
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  if (loading) return <div className="text-center py-5"><CSpinner /></div>

  return (
    <>
      <div className="d-flex align-items-center gap-3 mb-4">
        <CButton color="light" onClick={() => navigate(-1)}>
          <CIcon icon={cilArrowLeft} />
        </CButton>
        <h4 className="mb-0">{editing ? t('knowledge_base.edit_article') : t('knowledge_base.new_article')}</h4>
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <CCard>
        <CCardBody>
          <CForm className="d-flex flex-column gap-3">
            <div>
              <CFormLabel>{t('knowledge_base.article_title')}</CFormLabel>
              <CFormInput value={title} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div>
              <CFormLabel>{t('knowledge_base.category_label')}</CFormLabel>
              <CFormSelect value={categoryId} onChange={(e) => setCategoryId(e.target.value)}>
                <option value="">{t('knowledge_base.select_category')}</option>
                {categories.map((c) => (
                  <option key={c.id} value={c.id}>{catName(c, i18n.language)}</option>
                ))}
              </CFormSelect>
            </div>
            <div>
              <CFormLabel>{t('knowledge_base.tags_label')}</CFormLabel>
              <CFormInput
                value={tagsText}
                onChange={(e) => setTagsText(e.target.value)}
                placeholder={t('knowledge_base.tags_placeholder')}
              />
            </div>
            <div>
              <CFormLabel>{t('knowledge_base.article_body')}</CFormLabel>
              <ReactQuill
                ref={quillRef}
                theme="snow"
                value={body}
                onChange={setBody}
                modules={quillModules}
                style={{ height: 320, marginBottom: 42 }}
              />
              {!editing && (
                <div className="text-muted small mt-1">{t('knowledge_base.media_hint_save_first')}</div>
              )}
            </div>

            <div>
              <CFormLabel>{t('knowledge_base.visibility_label')}</CFormLabel>
              <div className="d-flex flex-column gap-1">
                <CFormCheck
                  type="radio"
                  name="visibility"
                  label={t('knowledge_base.visibility_all')}
                  checked={visibleToAll}
                  onChange={() => setVisibleToAll(true)}
                />
                <CFormCheck
                  type="radio"
                  name="visibility"
                  label={t('knowledge_base.visibility_selected')}
                  checked={!visibleToAll}
                  onChange={() => setVisibleToAll(false)}
                />
              </div>
              {!visibleToAll && (
                <div className="border rounded p-2 mt-2" style={{ maxHeight: 220, overflowY: 'auto' }}>
                  {assignableUsers.length === 0 ? (
                    <div className="text-muted small">{t('tasks.no_assignable_users')}</div>
                  ) : assignableUsers.map((u) => (
                    <CFormCheck
                      key={u.id}
                      label={[u.firstName, u.lastName].filter(Boolean).join(' ') || u.username}
                      checked={allowedUserIds.includes(u.id)}
                      onChange={() => toggleAllowedUser(u.id)}
                    />
                  ))}
                </div>
              )}
            </div>

            <div>
              <CFormLabel>{t('knowledge_base.video_label')}</CFormLabel>
              {editing ? (
                <>
                  {videos.length > 0 && (
                    <div className="d-flex flex-wrap gap-2 mb-2">
                      {videos.map((v) => (
                        <div key={v.id} className="position-relative">
                          <video src={kbApi.mediaUrl(id, v.id)} style={{ width: 160, borderRadius: 6 }} controls />
                          <CButton
                            size="sm"
                            color="danger"
                            className="position-absolute top-0 end-0 p-0 d-flex align-items-center justify-content-center"
                            style={{ width: 22, height: 22, lineHeight: 1, borderRadius: '50%' }}
                            onClick={() => handleVideoDelete(v.id)}
                          >
                            ×
                          </CButton>
                        </div>
                      ))}
                    </div>
                  )}
                  <div className="d-flex align-items-center gap-2">
                    <input type="file" accept="video/*" onChange={handleVideoChange} disabled={uploadingVideo} />
                    {uploadingVideo && <CSpinner size="sm" />}
                  </div>
                </>
              ) : (
                <div className="text-muted small">{t('knowledge_base.media_hint_save_first')}</div>
              )}
            </div>
          </CForm>
        </CCardBody>
      </CCard>

      <div className="d-flex justify-content-end gap-2 mt-3">
        <CButton color="secondary" onClick={() => navigate(-1)}>{t('common.cancel')}</CButton>
        <CButton color="primary" onClick={handleSave} disabled={saving || !title.trim()}>
          {saving ? <CSpinner size="sm" /> : t('common.save')}
        </CButton>
      </div>
    </>
  )
}
