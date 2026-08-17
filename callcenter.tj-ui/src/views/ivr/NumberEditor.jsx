import React, { useEffect, useRef, useState } from 'react'
import {
  CCard, CCardBody, CCardHeader, CNav, CNavItem, CNavLink,
  CTabContent, CTabPane, CButton, CForm, CFormLabel, CFormInput,
  CFormSelect, CRow, CCol, CTable, CTableHead, CTableHeaderCell,
  CTableBody, CTableRow, CTableDataCell, CBadge, CAlert, CSpinner,
  CModal, CModalHeader, CModalTitle, CModalBody, CModalFooter,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import {
  cilPlus, cilTrash, cilCloudUpload, cilArrowLeft,
  cilPeople, cilListNumbered, cilMusicNote, cilSync, cilSettings,
} from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { ivr as ivrApi } from 'src/api'

const ACTIONS = [
  { value: 'queue',     labelKey: 'number_editor.action_queue' },
  { value: 'extension', labelKey: 'number_editor.action_extension' },
  { value: 'playback',  labelKey: 'number_editor.action_playback' },
  { value: 'hangup',    labelKey: 'number_editor.action_hangup' },
]

const STRATEGIES = [
  { value: 'ringall',      labelKey: 'number_editor.strategy_ringall' },
  { value: 'leastrecent',  labelKey: 'number_editor.strategy_leastrecent' },
  { value: 'fewestcalls',  labelKey: 'number_editor.strategy_fewestcalls' },
  { value: 'random',       labelKey: 'number_editor.strategy_random' },
  { value: 'rrmemory',     labelKey: 'number_editor.strategy_rrmemory' },
  { value: 'linear',       labelKey: 'number_editor.strategy_linear' },
]

const DIGITS = ['0','1','2','3','4','5','6','7','8','9','*','#']

// ─── Editor for a single KC number: Приветствие / Меню IVR / Очередь / Операторы ──
export default function NumberEditor({ kcNumber, onBack }) {
  const { t } = useTranslation()
  const kcId = kcNumber.id

  const [tab,     setTab]     = useState('greeting')
  const [loading, setLoading] = useState(true)
  const [saving,  setSaving]  = useState(false)
  const [syncing, setSyncing] = useState(false)
  const [error,   setError]   = useState('')
  const [success, setSuccess] = useState('')

  const [config,   setConfig]   = useState({ strategy: 'ringall', waitTimeout: 5, queueTimeout: 300, maxCallers: 0, mohClass: 'default' })
  const [options,  setOptions]  = useState([])
  const [members,  setMembers]  = useState([])
  const [availUsers, setAvailUsers] = useState([])

  const [optModal, setOptModal] = useState(false)
  const [optForm,  setOptForm]  = useState({ digit: '1', label: '', action: 'queue', actionData: '', sortOrder: 0 })

  const [uploading,    setUploading]    = useState(false)
  const [greetingInfo, setGreetingInfo] = useState('')
  const fileRef = useRef()

  const load = () => {
    setLoading(true)
    ivrApi.get(kcId)
      .then(d => {
        setConfig(d.config)
        setOptions(d.options || [])
        setMembers(d.members || [])
        setGreetingInfo(d.config?.greetingFile || '')
      })
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  const loadAvailUsers = () => {
    ivrApi.availableUsers(kcId).then(d => setAvailUsers(d.users || [])).catch(() => {})
  }

  useEffect(() => { load(); loadAvailUsers() }, [kcId])

  const handleSaveConfig = async () => {
    setSaving(true); setError(''); setSuccess('')
    try {
      await ivrApi.updateConfig(kcId, config)
      setSuccess(t('ivr.save_success'))
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleSync = async () => {
    setSyncing(true); setError(''); setSuccess('')
    try {
      const r = await ivrApi.sync(kcId)
      setSuccess(t('number_editor.applied_to_asterisk', { context: r.context, queue: r.queue }))
    } catch (e) { setError(e.message) }
    finally { setSyncing(false) }
  }

  const handleUploadGreeting = async (e) => {
    const file = e.target.files[0]
    if (!file) return
    setUploading(true); setError('')
    try {
      const r = await ivrApi.uploadGreeting(kcId, file)
      setGreetingInfo(r.asteriskPath)
      setConfig(c => ({ ...c, greetingFile: r.asteriskPath }))
      setSuccess(t('number_editor.file_uploaded', { file: r.file }))
    } catch (e) { setError(e.message) }
    finally { setUploading(false) }
  }

  const openOptModal = (opt = null) => {
    setOptForm(opt
      ? { digit: opt.digit, label: opt.label, action: opt.action, actionData: opt.actionData, sortOrder: opt.sortOrder }
      : { digit: '1', label: '', action: 'queue', actionData: '', sortOrder: options.length }
    )
    setOptModal(true)
  }

  const handleSaveOption = async () => {
    setSaving(true)
    try {
      await ivrApi.saveOption(kcId, optForm)
      setOptModal(false)
      load()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDeleteOption = async (digit) => {
    if (!confirm(t('number_editor.delete_option_confirm', { digit }))) return
    try { await ivrApi.deleteOption(kcId, digit); load() }
    catch (e) { setError(e.message) }
  }

  const handleAddMember = async (username) => {
    try { await ivrApi.addMember(kcId, username); load(); loadAvailUsers() }
    catch (e) { setError(e.message) }
  }

  const handleRemoveMember = async (username) => {
    if (!confirm(t('number_editor.remove_member_confirm', { username }))) return
    try { await ivrApi.removeMember(kcId, username); load(); loadAvailUsers() }
    catch (e) { setError(e.message) }
  }

  const queueName  = `queue_tenant_${kcId}`
  const ivrContext = kcNumber.providerName || '—'

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <div>
          <CButton color="light" size="sm" className="mb-2" onClick={onBack}>
            <CIcon icon={cilArrowLeft} className="me-1" />{t('number_editor.back_to_list')}
          </CButton>
          <h4 className="mb-0">{kcNumber.number}</h4>
          <div className="text-muted small mt-1">
            {t('number_editor.context_label')}: <code>{ivrContext}</code> · {t('number_editor.queue_label')}: <code>{queueName}</code>
          </div>
        </div>
        <CButton color="success" onClick={handleSync} disabled={syncing}>
          {syncing ? <CSpinner size="sm" className="me-2" /> : <CIcon icon={cilSync} className="me-2" />}
          {t('number_editor.apply_to_asterisk')}
        </CButton>
      </div>

      {error   && <CAlert color="danger"  dismissible onClose={() => setError('')}>{error}</CAlert>}
      {success && <CAlert color="success" dismissible onClose={() => setSuccess('')}>{success}</CAlert>}

      {loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : (
        <CCard>
          <CCardHeader>
            <CNav variant="tabs" className="card-header-tabs">
              {[
                { key: 'greeting', icon: cilMusicNote,     label: t('number_editor.tab_greeting') },
                { key: 'menu',     icon: cilListNumbered,  label: t('number_editor.tab_menu') },
                { key: 'queue',    icon: cilSettings,      label: t('number_editor.tab_queue') },
                { key: 'members',  icon: cilPeople,        label: `${t('number_editor.tab_members')} (${members.length})` },
              ].map(tabDef => (
                <CNavItem key={tabDef.key}>
                  <CNavLink active={tab === tabDef.key} onClick={() => setTab(tabDef.key)} style={{ cursor: 'pointer' }}>
                    <CIcon icon={tabDef.icon} className="me-1" />{tabDef.label}
                  </CNavLink>
                </CNavItem>
              ))}
            </CNav>
          </CCardHeader>

          <CCardBody>
            <CTabContent>

              {/* ── Приветствие ── */}
              <CTabPane visible={tab === 'greeting'}>
                <div style={{ maxWidth: 520 }}>
                  <div className="mb-4">
                    <CFormLabel className="fw-semibold">{t('number_editor.greeting_file_label')}</CFormLabel>
                    <div className="text-muted small mb-2">
                      {t('number_editor.greeting_hint')}
                    </div>
                    {greetingInfo && (
                      <CAlert color="info" className="small py-2 mb-3">
                        {t('number_editor.current_asterisk_file_label')} <code>{greetingInfo}</code>
                      </CAlert>
                    )}
                    <div className="d-flex gap-2 align-items-center">
                      <CButton color="primary" onClick={() => fileRef.current?.click()} disabled={uploading}>
                        {uploading
                          ? <CSpinner size="sm" className="me-2" />
                          : <CIcon icon={cilCloudUpload} className="me-2" />}
                        {uploading ? t('number_editor.uploading') : t('number_editor.upload_file')}
                      </CButton>
                      <input ref={fileRef} type="file" accept=".wav,.gsm,.mp3,.ulaw"
                        className="d-none" onChange={handleUploadGreeting} />
                    </div>
                  </div>

                  <hr />

                  <div className="mt-3">
                    <CFormLabel className="fw-semibold">{t('number_editor.moh_label')}</CFormLabel>
                    <div className="text-muted small mb-2">
                      {t('number_editor.moh_hint')}
                    </div>
                    <div className="d-flex gap-2" style={{ maxWidth: 300 }}>
                      <CFormInput value={config.mohClass}
                        onChange={e => setConfig(c => ({ ...c, mohClass: e.target.value }))}
                        placeholder="default" />
                      <CButton color="primary" onClick={handleSaveConfig} disabled={saving}>
                        {t('common.save')}
                      </CButton>
                    </div>
                  </div>
                </div>
              </CTabPane>

              {/* ── Меню IVR ── */}
              <CTabPane visible={tab === 'menu'}>
                <div className="d-flex justify-content-between align-items-center mb-3">
                  <div className="text-muted small">
                    {t('number_editor.menu_hint')}
                  </div>
                  <CButton color="primary" size="sm" onClick={() => openOptModal()}>
                    <CIcon icon={cilPlus} className="me-1" />{t('number_editor.add_option')}
                  </CButton>
                </div>
                {options.length === 0 ? (
                  <div className="text-center text-muted py-5">
                    <div style={{ fontSize: 40 }}>☎️</div>
                    <div className="mt-2">{t('number_editor.menu_empty')}</div>
                    <div className="small mt-1">{t('number_editor.menu_empty_hint')}</div>
                  </div>
                ) : (
                  <CTable hover responsive>
                    <CTableHead>
                      <CTableRow>
                        <CTableHeaderCell>{t('number_editor.key_label')}</CTableHeaderCell>
                        <CTableHeaderCell>{t('number_editor.option_name')}</CTableHeaderCell>
                        <CTableHeaderCell>{t('number_editor.action')}</CTableHeaderCell>
                        <CTableHeaderCell>{t('number_editor.data_label')}</CTableHeaderCell>
                        <CTableHeaderCell className="text-end">{t('common.delete')}</CTableHeaderCell>
                      </CTableRow>
                    </CTableHead>
                    <CTableBody>
                      {options.map(opt => (
                        <CTableRow key={opt.digit}>
                          <CTableDataCell>
                            <CBadge color="primary" className="fs-6 px-3">{opt.digit}</CBadge>
                          </CTableDataCell>
                          <CTableDataCell className="fw-semibold">{opt.label}</CTableDataCell>
                          <CTableDataCell>
                            <CBadge color="secondary">
                              {t(ACTIONS.find(a => a.value === opt.action)?.labelKey) ?? opt.action}
                            </CBadge>
                          </CTableDataCell>
                          <CTableDataCell className="text-muted small">
                            {opt.actionData || '—'}
                          </CTableDataCell>
                          <CTableDataCell className="text-end">
                            <CButton size="sm" color="danger" onClick={() => handleDeleteOption(opt.digit)}>
                              <CIcon icon={cilTrash} />
                            </CButton>
                          </CTableDataCell>
                        </CTableRow>
                      ))}
                    </CTableBody>
                  </CTable>
                )}
              </CTabPane>

              {/* ── Очередь ── */}
              <CTabPane visible={tab === 'queue'}>
                <CForm className="d-flex flex-column gap-3" style={{ maxWidth: 520 }}>
                  <CRow className="g-3">
                    <CCol md={6}>
                      <CFormLabel>{t('number_editor.strategy_label')}</CFormLabel>
                      <CFormSelect value={config.strategy} onChange={e => setConfig(c => ({ ...c, strategy: e.target.value }))}>
                        {STRATEGIES.map(s => <option key={s.value} value={s.value}>{t(s.labelKey)}</option>)}
                      </CFormSelect>
                    </CCol>
                    <CCol md={6}>
                      <CFormLabel>{t('number_editor.max_waiting_label')}</CFormLabel>
                      <CFormInput type="number" min="0" value={config.maxCallers}
                        onChange={e => setConfig(c => ({ ...c, maxCallers: +e.target.value }))} />
                    </CCol>
                    <CCol md={6}>
                      <CFormLabel>{t('number_editor.key_wait_label')}</CFormLabel>
                      <CFormInput type="number" min="1" max="30" value={config.waitTimeout}
                        onChange={e => setConfig(c => ({ ...c, waitTimeout: +e.target.value }))} />
                    </CCol>
                    <CCol md={6}>
                      <CFormLabel>{t('number_editor.max_queue_time_label')}</CFormLabel>
                      <CFormInput type="number" min="30" value={config.queueTimeout}
                        onChange={e => setConfig(c => ({ ...c, queueTimeout: +e.target.value }))} />
                    </CCol>
                  </CRow>
                  <div>
                    <CButton color="primary" onClick={handleSaveConfig} disabled={saving}>
                      {saving ? <CSpinner size="sm" className="me-2" /> : null}
                      {t('number_editor.save_settings')}
                    </CButton>
                  </div>
                </CForm>
              </CTabPane>

              {/* ── Операторы ── */}
              <CTabPane visible={tab === 'members'}>
                <CRow className="g-3">
                  <CCol md={6}>
                    <div className="fw-semibold mb-2">{t('number_editor.queue_label')} <code>{queueName}</code></div>
                    {members.length === 0 ? (
                      <div className="text-muted small py-3">{t('number_editor.no_members')}</div>
                    ) : (
                      <CTable hover responsive className="mb-0">
                        <CTableHead>
                          <CTableRow>
                            <CTableHeaderCell>{t('monitor.col_agent')}</CTableHeaderCell>
                            <CTableHeaderCell>{t('common.status')}</CTableHeaderCell>
                            <CTableHeaderCell></CTableHeaderCell>
                          </CTableRow>
                        </CTableHead>
                        <CTableBody>
                          {members.map(m => (
                            <CTableRow key={m.username}>
                              <CTableDataCell>
                                <div className="fw-semibold">{m.username}</div>
                                <div className="text-muted small">
                                  {[m.firstName, m.lastName].filter(Boolean).join(' ') || ''}
                                </div>
                              </CTableDataCell>
                              <CTableDataCell>
                                <CBadge color={m.paused ? 'warning' : 'success'}>
                                  {m.paused ? t('number_editor.paused') : t('common.active')}
                                </CBadge>
                              </CTableDataCell>
                              <CTableDataCell className="text-end">
                                <CButton size="sm" color="danger" onClick={() => handleRemoveMember(m.username)}>
                                  <CIcon icon={cilTrash} />
                                </CButton>
                              </CTableDataCell>
                            </CTableRow>
                          ))}
                        </CTableBody>
                      </CTable>
                    )}
                  </CCol>

                  <CCol md={6}>
                    <div className="fw-semibold mb-2">{t('number_editor.available_users')}</div>
                    {availUsers.length === 0 ? (
                      <div className="text-muted small py-3">{t('number_editor.all_users_in_queue')}</div>
                    ) : (
                      <CTable hover responsive className="mb-0">
                        <CTableHead>
                          <CTableRow>
                            <CTableHeaderCell>{t('number_editor.col_user')}</CTableHeaderCell>
                            <CTableHeaderCell></CTableHeaderCell>
                          </CTableRow>
                        </CTableHead>
                        <CTableBody>
                          {availUsers.map(u => (
                            <CTableRow key={u.username}>
                              <CTableDataCell>
                                <div className="fw-semibold">{u.username}</div>
                                <div className="text-muted small">
                                  {[u.firstName, u.lastName].filter(Boolean).join(' ') || ''}
                                </div>
                              </CTableDataCell>
                              <CTableDataCell className="text-end">
                                <CButton size="sm" color="success" onClick={() => handleAddMember(u.username)}>
                                  <CIcon icon={cilPlus} />
                                </CButton>
                              </CTableDataCell>
                            </CTableRow>
                          ))}
                        </CTableBody>
                      </CTable>
                    )}
                  </CCol>
                </CRow>
              </CTabPane>

            </CTabContent>
          </CCardBody>
        </CCard>
      )}

      {/* Modal: Add/Edit IVR option */}
      <CModal visible={optModal} onClose={() => setOptModal(false)}>
        <CModalHeader><CModalTitle>{t('number_editor.option_modal_title')}</CModalTitle></CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <CRow className="g-2">
              <CCol md={4}>
                <CFormLabel>{t('number_editor.key_label')}</CFormLabel>
                <CFormSelect value={optForm.digit} onChange={e => setOptForm(f => ({ ...f, digit: e.target.value }))}>
                  {DIGITS.map(d => <option key={d} value={d}>{d}</option>)}
                </CFormSelect>
              </CCol>
              <CCol md={8}>
                <CFormLabel>{t('number_editor.option_name')}</CFormLabel>
                <CFormInput value={optForm.label}
                  onChange={e => setOptForm(f => ({ ...f, label: e.target.value }))}
                  placeholder={t('number_editor.option_name_placeholder')} />
              </CCol>
            </CRow>
            <div>
              <CFormLabel>{t('number_editor.action')}</CFormLabel>
              <CFormSelect value={optForm.action} onChange={e => setOptForm(f => ({ ...f, action: e.target.value }))}>
                {ACTIONS.map(a => <option key={a.value} value={a.value}>{t(a.labelKey)}</option>)}
              </CFormSelect>
            </div>
            {optForm.action !== 'hangup' && (
              <div>
                <CFormLabel>
                  {optForm.action === 'queue'     && t('number_editor.queue_name_empty_hint', { queue: queueName })}
                  {optForm.action === 'extension' && t('number_editor.extension_number_label')}
                  {optForm.action === 'playback'  && t('number_editor.file_path_label')}
                </CFormLabel>
                <CFormInput value={optForm.actionData}
                  onChange={e => setOptForm(f => ({ ...f, actionData: e.target.value }))}
                  placeholder={
                    optForm.action === 'queue'     ? queueName :
                    optForm.action === 'extension' ? '1001' : 'sounds/custom-file'
                  }
                />
              </div>
            )}
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setOptModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSaveOption} disabled={saving || !optForm.digit}>
            {saving ? <CSpinner size="sm" /> : t('common.save')}
          </CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
