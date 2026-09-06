import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  DndContext, closestCenter, useDraggable, useDroppable,
  PointerSensor, useSensor, useSensors,
} from '@dnd-kit/core'
import { CSS } from '@dnd-kit/utilities'
import {
  CCard, CCardHeader, CCardBody, CButton, CBadge, CSpinner, CAlert,
  CModal, CModalHeader, CModalTitle, CModalBody, CModalFooter,
  CForm, CFormInput, CFormLabel, CFormTextarea, CFormSelect, CFormCheck,
  CInputGroup, CInputGroupText,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilPencil, cilSearch } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { tasks as tasksApi, tenants as tenantsApi } from 'src/api'
import useAuthStore from 'src/store/auth'

const POLL_MS = 15000

const COLUMNS = [
  { key: 'todo',        labelKey: 'tasks.status_todo' },
  { key: 'in_progress', labelKey: 'tasks.status_in_progress' },
  { key: 'waiting',     labelKey: 'tasks.status_waiting' },
  { key: 'resolved',    labelKey: 'tasks.status_resolved' },
]

function TaskCard({ task, canManage, onEdit, onOpen }) {
  const { t } = useTranslation()
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({ id: String(task.id) })
  const style = {
    transform: CSS.Translate.toString(transform),
    opacity: isDragging ? 0.4 : 1,
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...listeners}
      {...attributes}
      className="p-2 mb-2 border rounded bg-body-tertiary"
      role="button"
      onClick={() => onOpen(task)}
    >
      <div className="d-flex justify-content-between align-items-start gap-2">
        <div className="fw-semibold small">{task.title}</div>
        {canManage && (
          <CButton size="sm" color="light" className="p-1 lh-1"
            onClick={(e) => { e.stopPropagation(); onEdit(task) }}>
            <CIcon icon={cilPencil} size="sm" />
          </CButton>
        )}
      </div>
      {task.description && (
        <div
          className="text-muted small mt-1"
          style={{
            overflow: 'hidden', textOverflow: 'ellipsis',
            display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical',
          }}
        >
          {task.description}
        </div>
      )}
      <div className="mt-2 small text-muted">
        {t('tasks.assigned_to')}:{' '}
        <strong>
          {(task.assignees || []).map((a) => (a.isPrimary ? `★ ${a.name}` : a.name)).join(', ') || '—'}
        </strong>
      </div>
    </div>
  )
}

function TaskColumn({ column, columnTasks, canManage, onEdit, onOpen }) {
  const { t } = useTranslation()
  const { setNodeRef, isOver } = useDroppable({ id: column.key })

  return (
    <CCard className="h-100" style={{ backgroundColor: isOver ? 'var(--cui-tertiary-bg)' : undefined }}>
      <CCardHeader className="d-flex justify-content-between align-items-center py-2">
        <span className="fw-semibold small">{t(column.labelKey)}</span>
        <CBadge color="secondary">{columnTasks.length}</CBadge>
      </CCardHeader>
      <CCardBody ref={setNodeRef} style={{ minHeight: 120 }}>
        {columnTasks.map((task) => (
          <TaskCard key={task.id} task={task} canManage={canManage} onEdit={onEdit} onOpen={onOpen} />
        ))}
        {!columnTasks.length && (
          <div className="text-muted small text-center py-3">{t('tasks.no_tasks')}</div>
        )}
      </CCardBody>
    </CCard>
  )
}

// Create/edit modal — assignee pool is Supervisors/Operators (tasksApi.assignableUsers).
// SuperAdmin has no home tenant, so creating a task means picking one first;
// editing an existing task keeps its tenant fixed (the backend doesn't allow
// moving a task between tenants, only reassigning within it).
function TaskModal({ visible, task, onClose, onSaved }) {
  const { t } = useTranslation()
  const isSuperAdmin = useAuthStore((s) => s.isSuperAdmin())

  const [title,        setTitle]        = useState('')
  const [description,  setDescription]  = useState('')
  const [tenantId,     setTenantId]     = useState('')
  const [tenantList,   setTenantList]   = useState([])
  const [assigneeIds,  setAssigneeIds]  = useState([])
  const [primaryId,    setPrimaryId]    = useState('')
  const [assigneeList, setAssigneeList] = useState([])
  const [loadingAssignees, setLoadingAssignees] = useState(false)
  const [saving, setSaving] = useState(false)
  const [error,  setError]  = useState('')

  const loadAssignees = (tid) => {
    setLoadingAssignees(true)
    tasksApi.assignableUsers(tid)
      .then((d) => setAssigneeList(d.users || []))
      .catch(() => setAssigneeList([]))
      .finally(() => setLoadingAssignees(false))
  }

  useEffect(() => {
    if (!visible) return
    setError('')
    setTitle(task?.title || '')
    setDescription(task?.description || '')
    setAssigneeList([])

    if (task) {
      setAssigneeIds((task.assignees || []).map((a) => a.userId))
      const primary = (task.assignees || []).find((a) => a.isPrimary)
      setPrimaryId(primary ? String(primary.userId) : '')
      if (task.tenantId) { setTenantId(String(task.tenantId)); loadAssignees(task.tenantId) }
      return
    }

    setAssigneeIds([])
    setPrimaryId('')
    if (isSuperAdmin) {
      tenantsApi.list().then((d) => setTenantList(d.tenants || [])).catch(() => setTenantList([]))
      setTenantId('')
    } else {
      loadAssignees()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, task])

  const handleTenantChange = (e) => {
    const tid = e.target.value
    setTenantId(tid)
    setAssigneeIds([])
    setPrimaryId('')
    if (tid) loadAssignees(tid)
    else setAssigneeList([])
  }

  const toggleAssignee = (id) => {
    setAssigneeIds((prev) => {
      const next = prev.includes(id) ? prev.filter((x) => x !== id) : [...prev, id]
      if (!next.includes(Number(primaryId))) setPrimaryId('')
      return next
    })
  }

  const handleSave = async () => {
    if (!title.trim() || assigneeIds.length === 0) return
    setSaving(true)
    setError('')
    try {
      const payload = {
        title, description, assigneeIds,
        primaryUserId: primaryId ? Number(primaryId) : null,
      }
      if (task) {
        await tasksApi.update(task.id, payload)
      } else {
        if (isSuperAdmin) payload.tenantId = Number(tenantId)
        await tasksApi.create(payload)
      }
      onSaved()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const showTenantPicker = isSuperAdmin && !task
  const selectedAssignees = assigneeList.filter((u) => assigneeIds.includes(u.id))

  return (
    <CModal visible={visible} onClose={onClose}>
      <CModalHeader>
        <CModalTitle>{task ? t('tasks.edit_task') : t('tasks.new_task')}</CModalTitle>
      </CModalHeader>
      <CModalBody>
        {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}
        <CForm className="d-flex flex-column gap-3">
          <div>
            <CFormLabel>{t('tasks.task_title_label')}</CFormLabel>
            <CFormInput value={title} onChange={(e) => setTitle(e.target.value)} required />
          </div>
          <div>
            <CFormLabel>{t('tasks.description_label')}</CFormLabel>
            <CFormTextarea rows={3} value={description} onChange={(e) => setDescription(e.target.value)} />
          </div>
          {showTenantPicker && (
            <div>
              <CFormLabel>{t('tasks.tenant_label')}</CFormLabel>
              <CFormSelect value={tenantId} onChange={handleTenantChange}>
                <option value="">{t('tasks.select_tenant')}</option>
                {tenantList.map((tn) => (
                  <option key={tn.id} value={tn.id}>{tn.name}</option>
                ))}
              </CFormSelect>
            </div>
          )}
          <div>
            <CFormLabel>{t('tasks.assignee_label')}</CFormLabel>
            {loadingAssignees ? (
              <div className="py-2"><CSpinner size="sm" /></div>
            ) : (
              <div className="border rounded p-2" style={{ maxHeight: 180, overflowY: 'auto' }}>
                {assigneeList.map((u) => (
                  <CFormCheck
                    key={u.id}
                    id={`task-assignee-${u.id}`}
                    label={[u.firstName, u.lastName].filter(Boolean).join(' ') || u.username}
                    checked={assigneeIds.includes(u.id)}
                    onChange={() => toggleAssignee(u.id)}
                  />
                ))}
                {!assigneeList.length && (
                  <div className="text-muted small">{t('tasks.no_assignable_users')}</div>
                )}
              </div>
            )}
          </div>
          {assigneeIds.length > 1 && (
            <div>
              <CFormLabel>{t('tasks.primary_label')}</CFormLabel>
              <CFormSelect value={primaryId} onChange={(e) => setPrimaryId(e.target.value)}>
                <option value="">{t('tasks.no_primary')}</option>
                {selectedAssignees.map((u) => (
                  <option key={u.id} value={u.id}>
                    {[u.firstName, u.lastName].filter(Boolean).join(' ') || u.username}
                  </option>
                ))}
              </CFormSelect>
              <div className="form-text">{t('tasks.primary_hint')}</div>
            </div>
          )}
        </CForm>
      </CModalBody>
      <CModalFooter>
        <CButton color="secondary" onClick={onClose}>{t('common.cancel')}</CButton>
        <CButton color="primary" onClick={handleSave} disabled={saving || !title.trim() || assigneeIds.length === 0}>
          {saving ? <CSpinner size="sm" /> : t('common.save')}
        </CButton>
      </CModalFooter>
    </CModal>
  )
}

// Read-only task detail: progress (who/when created, who/when resolved) plus
// the comment thread — this is what a Supervisor opens after searching for a
// task to check in on work they aren't personally assigned to (see
// canAccessTask on the backend, which now allows exactly this). Anyone who
// can see the task may also add a comment, not just its assignees.
function TaskDetailModal({ task, visible, onClose }) {
  const { t } = useTranslation()
  const [comments, setComments] = useState([])
  const [loadingComments, setLoadingComments] = useState(false)
  const [newComment, setNewComment] = useState('')
  const [posting, setPosting] = useState(false)
  const [error, setError] = useState('')

  const loadComments = useCallback(() => {
    if (!task) return
    setLoadingComments(true)
    tasksApi.comments(task.id)
      .then((d) => setComments(d.comments || []))
      .catch(() => {})
      .finally(() => setLoadingComments(false))
  }, [task])

  useEffect(() => {
    if (visible && task) { setNewComment(''); setError(''); loadComments() }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [visible, task?.id])

  const handlePost = async () => {
    if (!newComment.trim() || !task) return
    setPosting(true)
    setError('')
    try {
      await tasksApi.comment(task.id, newComment.trim())
      setNewComment('')
      loadComments()
    } catch (e) { setError(e.message) }
    finally { setPosting(false) }
  }

  if (!task) return null

  return (
    <CModal visible={visible} onClose={onClose} size="lg">
      <CModalHeader>
        <CModalTitle>{task.title}</CModalTitle>
      </CModalHeader>
      <CModalBody>
        <div className="d-flex flex-wrap gap-3 mb-3 small text-muted">
          <div>{t('tasks.col_status')}: <strong className="text-body">{t(`tasks.status_${task.status}`)}</strong></div>
          <div>{t('tasks.created_by')}: <strong className="text-body">{task.creatorName || '—'}</strong></div>
          <div>{t('tasks.assigned_to')}: <strong className="text-body">
            {(task.assignees || []).map((a) => (a.isPrimary ? `★ ${a.name}` : a.name)).join(', ') || '—'}
          </strong></div>
          <div>{t('tasks.col_created')}: <strong className="text-body">{new Date(task.createdAt).toLocaleString()}</strong></div>
          {task.resolvedAt && (
            <div>{t('tasks.resolved_by')}: <strong className="text-body">
              {task.resolvedByName || '—'} ({new Date(task.resolvedAt).toLocaleString()})
            </strong></div>
          )}
        </div>
        {task.description && <p>{task.description}</p>}

        <hr />
        <div className="fw-semibold small mb-2">{t('tasks.comments_title')}</div>
        {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}
        {loadingComments ? (
          <div className="text-center py-3"><CSpinner size="sm" /></div>
        ) : (
          <div className="d-flex flex-column gap-2 mb-3" style={{ maxHeight: 240, overflowY: 'auto' }}>
            {comments.map((cm) => (
              <div key={cm.id} className="border rounded p-2 small">
                <div className="d-flex justify-content-between gap-2">
                  <strong>{cm.username}</strong>
                  <span className="text-muted">{new Date(cm.createdAt).toLocaleString()}</span>
                </div>
                <div className="mt-1">{cm.text}</div>
              </div>
            ))}
            {!comments.length && <div className="text-muted small">{t('tasks.no_comments')}</div>}
          </div>
        )}
        <CFormTextarea rows={2} placeholder={t('tasks.comment_placeholder')} value={newComment}
          onChange={(e) => setNewComment(e.target.value)} />
      </CModalBody>
      <CModalFooter>
        <CButton color="secondary" onClick={onClose}>{t('common.close')}</CButton>
        <CButton color="primary" onClick={handlePost} disabled={posting || !newComment.trim()}>
          {posting ? <CSpinner size="sm" /> : t('tasks.add_comment')}
        </CButton>
      </CModalFooter>
    </CModal>
  )
}

export default function TasksBoard() {
  const { t } = useTranslation()
  const isSuperAdmin  = useAuthStore((s) => s.isSuperAdmin())
  const isTenantAdmin = useAuthStore((s) => s.isTenantAdmin())
  const canManage = isSuperAdmin || isTenantAdmin

  const [taskList, setTaskList] = useState([])
  const [loading,  setLoading]  = useState(true)
  const [error,    setError]    = useState('')
  const [modalOpen,   setModalOpen]   = useState(false)
  const [editingTask, setEditingTask] = useState(null)
  const [detailTask,  setDetailTask]  = useState(null)
  const [searchInput, setSearchInput] = useState('')
  const [search,      setSearch]      = useState('')
  // Task ids with an in-flight status PATCH — the next poll tick must keep
  // showing their optimistic (dropped-into) column instead of overwriting it
  // with the pre-drag snapshot the server may still return mid-flight.
  const pendingRef = useRef(new Set())

  // Debounce: apply the typed search 400ms after the user stops typing,
  // rather than a request per keystroke.
  useEffect(() => {
    const timer = setTimeout(() => setSearch(searchInput.trim()), 400)
    return () => clearTimeout(timer)
  }, [searchInput])

  const load = useCallback(() => {
    tasksApi.list({ search: search || undefined })
      .then((d) => {
        const fresh = d.tasks || []
        setTaskList((prev) => {
          if (pendingRef.current.size === 0) return fresh
          const prevById = Object.fromEntries(prev.map((tk) => [tk.id, tk]))
          return fresh.map((tk) => (pendingRef.current.has(tk.id) && prevById[tk.id] ? prevById[tk.id] : tk))
        })
      })
      .catch(() => setError(t('tasks.load_failed')))
      .finally(() => setLoading(false))
  }, [t, search])

  useEffect(() => {
    load()
    const timer = setInterval(load, POLL_MS)
    return () => clearInterval(timer)
  }, [load])

  const sensors = useSensors(useSensor(PointerSensor, { activationConstraint: { distance: 8 } }))

  const handleDragEnd = ({ active, over }) => {
    if (!over) return
    const taskId = Number(active.id)
    const newStatus = over.id
    const task = taskList.find((tk) => tk.id === taskId)
    if (!task || task.status === newStatus) return

    const prevStatus = task.status
    pendingRef.current.add(taskId)
    setTaskList((ts) => ts.map((tk) => (tk.id === taskId ? { ...tk, status: newStatus } : tk)))

    tasksApi.updateStatus(taskId, newStatus)
      .catch(() => {
        setTaskList((ts) => ts.map((tk) => (tk.id === taskId ? { ...tk, status: prevStatus } : tk)))
        setError(t('tasks.status_update_failed'))
      })
      .finally(() => { pendingRef.current.delete(taskId) })
  }

  const openCreate = () => { setEditingTask(null); setModalOpen(true) }
  const openEdit   = (task) => { setEditingTask(task); setModalOpen(true) }
  const handleSaved = () => { setModalOpen(false); load() }
  const openDetail  = (task) => setDetailTask(task)

  return (
    <CCard>
      <CCardHeader className="d-flex justify-content-between align-items-center flex-wrap gap-2">
        <span>{t('tasks.title')}</span>
        <div className="d-flex align-items-center gap-2">
          <CInputGroup style={{ width: 220 }}>
            <CInputGroupText><CIcon icon={cilSearch} /></CInputGroupText>
            <CFormInput size="sm" placeholder={t('tasks.search_placeholder')}
              value={searchInput} onChange={(e) => setSearchInput(e.target.value)} />
          </CInputGroup>
          {canManage && (
            <CButton size="sm" color="primary" onClick={openCreate}>
              <CIcon icon={cilPlus} className="me-1" />{t('tasks.new_task')}
            </CButton>
          )}
        </div>
      </CCardHeader>
      <CCardBody>
        {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}
        {loading ? (
          <div className="text-center py-4"><CSpinner size="sm" /></div>
        ) : (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragEnd={handleDragEnd}>
            <div className="row g-3">
              {COLUMNS.map((column) => (
                <div className="col-md-6 col-xl-3" key={column.key}>
                  <TaskColumn
                    column={column}
                    columnTasks={taskList.filter((tk) => tk.status === column.key)}
                    canManage={canManage}
                    onEdit={openEdit}
                    onOpen={openDetail}
                  />
                </div>
              ))}
            </div>
          </DndContext>
        )}
      </CCardBody>

      {modalOpen && (
        <TaskModal visible={modalOpen} task={editingTask} onClose={() => setModalOpen(false)} onSaved={handleSaved} />
      )}
      <TaskDetailModal task={detailTask} visible={!!detailTask} onClose={() => setDetailTask(null)} />
    </CCard>
  )
}
