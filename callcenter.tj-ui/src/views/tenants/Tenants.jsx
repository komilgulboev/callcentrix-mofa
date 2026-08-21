import React, { useEffect, useState } from 'react'
import {
  CButton, CCard, CCardBody, CCardHeader, CModal, CModalBody,
  CModalFooter, CModalHeader, CModalTitle, CForm, CFormInput,
  CFormLabel, CFormSelect, CTable, CTableBody, CTableDataCell, CTableHead,
  CTableHeaderCell, CTableRow, CBadge, CAlert, CSpinner,
  CNav, CNavItem, CNavLink, CTabContent, CTabPane,
} from '@coreui/react'
import CIcon from '@coreui/icons-react'
import { cilPlus, cilPencil, cilTrash, cilCheckCircle, cilBan, cilReload } from '@coreui/icons'
import { useTranslation } from 'react-i18next'
import { tenants as tenantsApi, kcNumbers as kcNumbersApi, providers as providersApi, asteriskServers as asteriskServersApi } from 'src/api'
import NumberEditor from 'src/views/ivr/NumberEditor'
import { statusBadge, PROVIDER_STATUS_POLL_MS } from 'src/utils/providerStatus'

const EMPTY = { name: '', domain: '', maxUsers: '', maxOperators: '', outboundProviderId: '', outboundCallerId: '' }

const PROVIDER_EMPTY = {
  name: '', host: '', port: 5060, transport: 'transport-udp',
  codecs: 'ulaw,alaw', username: '', password: '', register: true,
}

const SERVER_EMPTY = {
  name: '', amiHost: '', amiUser: '', amiPass: '',
  wsUri: '', sipHost: '', sipPort: 5060, active: true,
}

function AsteriskServersTab() {
  const { t } = useTranslation()
  const [servers, setServers] = useState([])
  const [loading, setLoading] = useState(true)
  const [error,   setError]   = useState('')
  const [modal,   setModal]   = useState(false)
  const [editing, setEditing] = useState(null)
  const [form,    setForm]    = useState(SERVER_EMPTY)
  const [saving,  setSaving]  = useState(false)

  const load = () => {
    setLoading(true)
    asteriskServersApi.list()
      .then(d => setServers(d.servers || []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])

  const openCreate = () => { setEditing(null); setForm(SERVER_EMPTY); setModal(true) }
  const openEdit   = (s) => { setEditing(s); setForm({ ...s, amiPass: '' }); setModal(true) }

  const handleSave = async () => {
    setSaving(true); setError('')
    try {
      const payload = { ...form, sipPort: parseInt(form.sipPort) || 5060 }
      if (editing) await asteriskServersApi.update(editing.id, payload)
      else         await asteriskServersApi.create(payload)
      setModal(false); load()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDelete = async (id, name) => {
    if (!confirm(t('asterisk_servers.delete_confirm', { name }))) return
    try { await asteriskServersApi.remove(id); load() }
    catch (e) { setError(e.message) }
  }

  return (
    <div>
      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}
      <div className="text-muted small mb-3">{t('asterisk_servers.hint')}</div>

      <div className="d-flex justify-content-end mb-3">
        <CButton color="primary" onClick={openCreate}>
          <CIcon icon={cilPlus} className="me-2" />{t('asterisk_servers.add')}
        </CButton>
      </div>

      {loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : (
        <CTable hover responsive>
          <CTableHead>
            <CTableRow>
              <CTableHeaderCell>{t('asterisk_servers.col_name')}</CTableHeaderCell>
              <CTableHeaderCell>{t('asterisk_servers.col_ami_host')}</CTableHeaderCell>
              <CTableHeaderCell>{t('asterisk_servers.col_sip_host')}</CTableHeaderCell>
              <CTableHeaderCell>{t('common.status')}</CTableHeaderCell>
              <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
            </CTableRow>
          </CTableHead>
          <CTableBody>
            {servers.map(s => (
              <CTableRow key={s.id}>
                <CTableDataCell className="fw-semibold">{s.name}</CTableDataCell>
                <CTableDataCell className="text-muted">{s.amiHost}</CTableDataCell>
                <CTableDataCell className="text-muted">{s.sipHost}:{s.sipPort}</CTableDataCell>
                <CTableDataCell>
                  <CBadge color={s.active ? 'success' : 'secondary'}>
                    {s.active ? t('common.active') : t('common.inactive')}
                  </CBadge>
                </CTableDataCell>
                <CTableDataCell className="text-end">
                  <div className="d-flex gap-1 justify-content-end">
                    <CButton size="sm" color="light" onClick={() => openEdit(s)}><CIcon icon={cilPencil} /></CButton>
                    <CButton size="sm" color="danger" onClick={() => handleDelete(s.id, s.name)}><CIcon icon={cilTrash} /></CButton>
                  </div>
                </CTableDataCell>
              </CTableRow>
            ))}
            {!servers.length && (
              <CTableRow><CTableDataCell colSpan={5} className="text-center text-muted py-4">{t('asterisk_servers.empty')}</CTableDataCell></CTableRow>
            )}
          </CTableBody>
        </CTable>
      )}

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader><CModalTitle>{editing ? t('asterisk_servers.edit') : t('asterisk_servers.add')}</CModalTitle></CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <div><CFormLabel>{t('asterisk_servers.name_label')}</CFormLabel><CFormInput value={form.name} onChange={e => setForm(f => ({ ...f, name: e.target.value }))} placeholder="asterisk-2" /></div>
            <div><CFormLabel>{t('asterisk_servers.ami_host_label')}</CFormLabel><CFormInput value={form.amiHost} onChange={e => setForm(f => ({ ...f, amiHost: e.target.value }))} placeholder="10.0.0.5:5038" /></div>
            <div><CFormLabel>{t('asterisk_servers.ami_user_label')}</CFormLabel><CFormInput value={form.amiUser} onChange={e => setForm(f => ({ ...f, amiUser: e.target.value }))} /></div>
            <div>
              <CFormLabel>{t('asterisk_servers.ami_pass_label')}</CFormLabel>
              <CFormInput type="password" value={form.amiPass} onChange={e => setForm(f => ({ ...f, amiPass: e.target.value }))}
                placeholder={editing ? '••••••' : ''} />
            </div>
            <div><CFormLabel>{t('asterisk_servers.ws_uri_label')}</CFormLabel><CFormInput value={form.wsUri} onChange={e => setForm(f => ({ ...f, wsUri: e.target.value }))} placeholder="wss://10.0.0.5:8089/ws" /></div>
            <div><CFormLabel>{t('asterisk_servers.sip_host_label')}</CFormLabel><CFormInput value={form.sipHost} onChange={e => setForm(f => ({ ...f, sipHost: e.target.value }))} placeholder="10.0.0.5" /></div>
            <div><CFormLabel>{t('asterisk_servers.sip_port_label')}</CFormLabel><CFormInput type="number" value={form.sipPort} onChange={e => setForm(f => ({ ...f, sipPort: e.target.value }))} placeholder="5060" /></div>
            <div className="form-check">
              <input className="form-check-input" type="checkbox" id="serverActive"
                checked={form.active} onChange={e => setForm(f => ({ ...f, active: e.target.checked }))} />
              <label className="form-check-label" htmlFor="serverActive">{t('common.active')}</label>
            </div>
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving || !form.name.trim() || !form.amiHost.trim()}>
            {saving ? <CSpinner size="sm" /> : t('common.save')}
          </CButton>
        </CModalFooter>
      </CModal>
    </div>
  )
}

function ProvidersTab() {
  const { t } = useTranslation()
  const [providers, setProviders] = useState([])
  const [loading,   setLoading]   = useState(true)
  const [refreshing, setRefreshing] = useState(false)
  const [error,     setError]     = useState('')
  const [modal,     setModal]     = useState(false)
  const [editing,   setEditing]   = useState(null)
  const [form,      setForm]      = useState(PROVIDER_EMPTY)
  const [saving,    setSaving]    = useState(false)

  const load = (silent) => {
    if (silent) setRefreshing(true); else setLoading(true)
    providersApi.list()
      .then(d => setProviders(d.providers || []))
      .catch(e => setError(e.message))
      .finally(() => { setLoading(false); setRefreshing(false) })
  }

  useEffect(() => {
    load(false)
    const t = setInterval(() => load(true), PROVIDER_STATUS_POLL_MS)
    return () => clearInterval(t)
  }, [])

  const openCreate = () => { setEditing(null); setForm(PROVIDER_EMPTY); setModal(true) }
  const openEdit   = (p) => { setEditing(p); setForm({ ...p }); setModal(true) }

  const handleSave = async () => {
    setSaving(true); setError('')
    try {
      const payload = { ...form, port: parseInt(form.port) || 5060 }
      if (editing) await providersApi.update(editing.id, payload)
      else         await providersApi.create(payload)
      setModal(false); load()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDelete = async (id, name) => {
    if (!confirm(t('providers.delete_confirm', { name }))) return
    try { await providersApi.remove(id); load() }
    catch (e) { setError(e.message) }
  }

  return (
    <div>
      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <div className="d-flex justify-content-end gap-2 mb-3">
        <CButton color="light" onClick={() => load(true)} disabled={refreshing}>
          {refreshing ? <CSpinner size="sm" /> : <CIcon icon={cilReload} />}
        </CButton>
        <CButton color="primary" onClick={openCreate}>
          <CIcon icon={cilPlus} className="me-2" />{t('providers.add')}
        </CButton>
      </div>

      {loading ? (
        <div className="text-center py-5"><CSpinner /></div>
      ) : (
        <CTable hover responsive>
          <CTableHead>
            <CTableRow>
              <CTableHeaderCell>{t('providers.col_name')}</CTableHeaderCell>
              <CTableHeaderCell>{t('providers.col_host')}</CTableHeaderCell>
              <CTableHeaderCell>{t('providers.col_transport')}</CTableHeaderCell>
              <CTableHeaderCell>{t('providers.col_connection')}</CTableHeaderCell>
              <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
            </CTableRow>
          </CTableHead>
          <CTableBody>
            {providers.map(p => {
              const badge = statusBadge(p.status)
              return (
                <CTableRow key={p.id}>
                  <CTableDataCell className="fw-semibold">{p.name}</CTableDataCell>
                  <CTableDataCell className="text-muted">{p.host}:{p.port}</CTableDataCell>
                  <CTableDataCell>{p.transport}</CTableDataCell>
                  <CTableDataCell>
                    <div className="d-flex align-items-center gap-2">
                      {badge ? <CBadge color={badge.color}>{badge.label}</CBadge> : <span className="text-muted">{t('providers.no_registration')}</span>}
                      {!!p.outboundTenantsCount && (
                        <CBadge color="info">{t('providers.outbound_tenants_count', { count: p.outboundTenantsCount })}</CBadge>
                      )}
                    </div>
                  </CTableDataCell>
                  <CTableDataCell className="text-end">
                    <div className="d-flex gap-1 justify-content-end">
                      <CButton size="sm" color="light" onClick={() => openEdit(p)}><CIcon icon={cilPencil} /></CButton>
                      <CButton size="sm" color="danger" onClick={() => handleDelete(p.id, p.name)}><CIcon icon={cilTrash} /></CButton>
                    </div>
                  </CTableDataCell>
                </CTableRow>
              )
            })}
            {!providers.length && (
              <CTableRow><CTableDataCell colSpan={5} className="text-center text-muted py-4">{t('providers.empty')}</CTableDataCell></CTableRow>
            )}
          </CTableBody>
        </CTable>
      )}

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader><CModalTitle>{editing ? t('providers.edit') : t('providers.new')}</CModalTitle></CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <div>
              <CFormLabel>{t('providers.col_name')}</CFormLabel>
              <CFormInput
                value={form.name}
                onChange={e => setForm(f => ({ ...f, name: e.target.value.replace(/[^A-Za-z0-9_-]/g, '') }))}
                placeholder={t('providers.name_placeholder')}
              />
              <div className="form-text">{t('providers.name_hint')}</div>
            </div>
            <div><CFormLabel>{t('providers.host_label')}</CFormLabel><CFormInput value={form.host} onChange={e => setForm(f => ({ ...f, host: e.target.value }))} placeholder="91.218.160.162" /></div>
            <div><CFormLabel>{t('providers.port_label')}</CFormLabel><CFormInput type="number" value={form.port} onChange={e => setForm(f => ({ ...f, port: e.target.value }))} placeholder="5060" /></div>
            <div>
              <CFormLabel>{t('providers.col_transport')}</CFormLabel>
              <CFormSelect value={form.transport} onChange={e => setForm(f => ({ ...f, transport: e.target.value }))}>
                <option value="transport-udp">UDP</option>
                <option value="transport-tcp">TCP</option>
                <option value="transport-tls">TLS</option>
              </CFormSelect>
            </div>
            <div><CFormLabel>{t('providers.codecs_label')}</CFormLabel><CFormInput value={form.codecs} onChange={e => setForm(f => ({ ...f, codecs: e.target.value }))} placeholder="ulaw,alaw" /></div>
            <div><CFormLabel>{t('providers.username_label')}</CFormLabel><CFormInput value={form.username} onChange={e => setForm(f => ({ ...f, username: e.target.value }))} /></div>
            <div><CFormLabel>{t('common.password')}</CFormLabel><CFormInput type="password" value={form.password} onChange={e => setForm(f => ({ ...f, password: e.target.value }))} /></div>
            <div className="form-check">
              <input className="form-check-input" type="checkbox" id="providerRegister"
                checked={form.register} onChange={e => setForm(f => ({ ...f, register: e.target.checked }))} />
              <label className="form-check-label" htmlFor="providerRegister">
                {t('providers.register_label')}
              </label>
            </div>
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving || !form.name.trim() || !form.host.trim()}>
            {saving ? <CSpinner size="sm" /> : t('common.save')}
          </CButton>
        </CModalFooter>
      </CModal>
    </div>
  )
}

function KCNumbersTab({ tenantsList }) {
  const { t } = useTranslation()
  const [tenantId, setTenantId] = useState('')
  const [numbers,  setNumbers]  = useState([])
  const [providersList, setProvidersList] = useState([])
  const [loading,  setLoading]  = useState(false)
  const [error,    setError]    = useState('')
  const [newNumber, setNewNumber] = useState('')
  const [newProviderId, setNewProviderId] = useState('')
  const [saving,   setSaving]   = useState(false)
  const [selected, setSelected] = useState(null) // kc number object or null

  useEffect(() => {
    if (tenantsList.length > 0 && !tenantId) setTenantId(String(tenantsList[0].id))
  }, [tenantsList])

  useEffect(() => {
    providersApi.list()
      .then(d => {
        const list = d.providers || []
        setProvidersList(list)
        if (list.length > 0 && !newProviderId) setNewProviderId(String(list[0].id))
      })
      .catch(() => {})
  }, [])

  const load = () => {
    if (!tenantId) return
    setLoading(true)
    kcNumbersApi.list(tenantId)
      .then(d => setNumbers(d.numbers || []))
      .catch(e => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [tenantId])

  const handleAdd = async () => {
    if (!newNumber.trim() || !newProviderId) return
    setSaving(true); setError('')
    try {
      const { id } = await kcNumbersApi.create(tenantId, newNumber.trim(), parseInt(newProviderId))
      const provider = providersList.find(p => String(p.id) === newProviderId)
      // Straight into the editor — a freshly created number has no greeting/
      // menu/queue yet, so leaving the admin on the bare list (where they'd
      // have to remember to come back and click "Configure") is how numbers
      // end up going live with just the default "beep" and an empty queue.
      setSelected({ id, number: newNumber.trim(), providerName: provider?.name || '' })
      setNewNumber('')
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDelete = async (numberId, number) => {
    if (!confirm(t('kc_numbers.delete_confirm', { number }))) return
    try { await kcNumbersApi.remove(tenantId, numberId); load() }
    catch (e) { setError(e.message) }
  }

  if (selected) {
    return <NumberEditor kcNumber={selected} onBack={() => { setSelected(null); load() }} />
  }

  return (
    <div>
      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <div className="mb-3" style={{ maxWidth: 340 }}>
        <CFormLabel>{t('common.tenant')}</CFormLabel>
        <CFormSelect value={tenantId} onChange={e => setTenantId(e.target.value)}>
          <option value="">{t('topics.select_tenant')}</option>
          {tenantsList.map(tn => <option key={tn.id} value={tn.id}>{tn.name}</option>)}
        </CFormSelect>
      </div>

      {!tenantId ? (
        <CAlert color="info">{t('common.select_tenant_hint')}</CAlert>
      ) : !providersList.length ? (
        <CAlert color="warning">
          {t('kc_numbers.need_provider_hint')}
        </CAlert>
      ) : (
        <>
          <div className="d-flex gap-2 mb-3 flex-wrap" style={{ maxWidth: 640 }}>
            <CFormInput value={newNumber} onChange={e => setNewNumber(e.target.value)}
              placeholder={t('kc_numbers.number_placeholder')} style={{ maxWidth: 260 }} />
            <CFormSelect value={newProviderId} onChange={e => setNewProviderId(e.target.value)} style={{ maxWidth: 220 }}>
              {providersList.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
            </CFormSelect>
            <CButton color="primary" onClick={handleAdd} disabled={saving || !newNumber.trim() || !newProviderId}>
              {saving ? <CSpinner size="sm" /> : <CIcon icon={cilPlus} />}
            </CButton>
          </div>

          {loading ? (
            <div className="text-center py-5"><CSpinner /></div>
          ) : (
            <CTable hover responsive>
              <CTableHead>
                <CTableRow>
                  <CTableHeaderCell>{t('kc_numbers.col_number')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('kc_numbers.col_provider')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('kc_numbers.col_greeting')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('kc_numbers.col_menu')}</CTableHeaderCell>
                  <CTableHeaderCell>{t('kc_numbers.col_agents')}</CTableHeaderCell>
                  <CTableHeaderCell className="text-end">{t('common.actions')}</CTableHeaderCell>
                </CTableRow>
              </CTableHead>
              <CTableBody>
                {numbers.map(n => (
                  <CTableRow key={n.id}>
                    <CTableDataCell className="fw-semibold">{n.number}</CTableDataCell>
                    <CTableDataCell className="text-muted">{n.providerName || '—'}</CTableDataCell>
                    <CTableDataCell>
                      <CBadge color={n.hasGreeting ? 'success' : 'secondary'}>{n.hasGreeting ? t('kc_numbers.yes') : t('kc_numbers.no')}</CBadge>
                    </CTableDataCell>
                    <CTableDataCell>{n.optionsCount}</CTableDataCell>
                    <CTableDataCell>{n.queueMembers}</CTableDataCell>
                    <CTableDataCell className="text-end">
                      <div className="d-flex gap-1 justify-content-end">
                        <CButton size="sm" color="primary" onClick={() => setSelected(n)}>
                          {t('kc_numbers.configure')}
                        </CButton>
                        <CButton size="sm" color="danger" onClick={() => handleDelete(n.id, n.number)}>
                          <CIcon icon={cilTrash} />
                        </CButton>
                      </div>
                    </CTableDataCell>
                  </CTableRow>
                ))}
                {!numbers.length && (
                  <CTableRow><CTableDataCell colSpan={6} className="text-center text-muted py-4">{t('kc_numbers.empty')}</CTableDataCell></CTableRow>
                )}
              </CTableBody>
            </CTable>
          )}
        </>
      )}
    </div>
  )
}

export default function Tenants() {
  const { t } = useTranslation()
  const [tab,     setTab]     = useState('tenants')
  const [rows,    setRows]    = useState([])
  const [providersList, setProvidersList] = useState([])
  const [loading, setLoading] = useState(true)
  const [error,   setError]   = useState('')
  const [modal,   setModal]   = useState(false)
  const [editing, setEditing] = useState(null)
  const [form,    setForm]    = useState(EMPTY)
  const [saving,  setSaving]  = useState(false)

  const load = () => {
    setLoading(true)
    tenantsApi.list()
      .then((d) => setRows(d.tenants ?? d))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }

  useEffect(load, [])
  useEffect(() => {
    providersApi.list().then(d => setProvidersList(d.providers || [])).catch(() => {})
  }, [])

  const openCreate = () => { setEditing(null); setForm(EMPTY); setModal(true) }
  const openEdit   = (tn)  => { setEditing(tn); setForm({
    name: tn.name, domain: tn.domain, maxUsers: tn.maxUsers ?? '', maxOperators: tn.maxOperators ?? '',
    outboundProviderId: tn.outboundProviderId ?? '', outboundCallerId: tn.outboundCallerId ?? '',
  }); setModal(true) }

  const handleSave = async () => {
    setSaving(true)
    try {
      const payload = {
        ...form,
        maxUsers: parseInt(form.maxUsers) || 50,
        outboundProviderId: form.outboundProviderId ? parseInt(form.outboundProviderId) : null,
      }
      if (editing) await tenantsApi.update(editing.id, payload)
      else         await tenantsApi.create(payload)
      setModal(false); load()
    } catch (e) { setError(e.message) }
    finally { setSaving(false) }
  }

  const handleDelete = async (id) => {
    if (!confirm(t('tenants.delete_confirm'))) return
    try { await tenantsApi.remove(id); load() }
    catch (e) { setError(e.message) }
  }

  const handleToggle = async (tn) => {
    try {
      if (tn.active) await tenantsApi.deactivate(tn.id)
      else           await tenantsApi.activate(tn.id)
      load()
    } catch (e) { setError(e.message) }
  }

  return (
    <>
      <div className="d-flex align-items-center justify-content-between mb-4">
        <h4 className="mb-0">{t('tenants.title')}</h4>
        {tab === 'tenants' && (
          <CButton color="primary" onClick={openCreate}>
            <CIcon icon={cilPlus} className="me-2" />{t('tenants.new_tenant')}
          </CButton>
        )}
      </div>

      {error && <CAlert color="danger" dismissible onClose={() => setError('')}>{error}</CAlert>}

      <CCard>
        <CCardHeader>
          <CNav variant="tabs" className="card-header-tabs">
            <CNavItem>
              <CNavLink active={tab === 'tenants'} onClick={() => setTab('tenants')} style={{ cursor: 'pointer' }}>
                {t('tenants.title')}
              </CNavLink>
            </CNavItem>
            <CNavItem>
              <CNavLink active={tab === 'kc-numbers'} onClick={() => setTab('kc-numbers')} style={{ cursor: 'pointer' }}>
                {t('kc_numbers.tab_title')}
              </CNavLink>
            </CNavItem>
            <CNavItem>
              <CNavLink active={tab === 'providers'} onClick={() => setTab('providers')} style={{ cursor: 'pointer' }}>
                {t('providers.tab_title')}
              </CNavLink>
            </CNavItem>
            <CNavItem>
              <CNavLink active={tab === 'asterisk-servers'} onClick={() => setTab('asterisk-servers')} style={{ cursor: 'pointer' }}>
                {t('asterisk_servers.tab_title')}
              </CNavLink>
            </CNavItem>
          </CNav>
        </CCardHeader>
        <CCardBody className={tab === 'tenants' ? 'p-0' : ''}>
          <CTabContent>
            <CTabPane visible={tab === 'tenants'}>
              {loading ? <div className="text-center py-5"><CSpinner /></div> : (
                <CTable hover responsive>
                  <CTableHead>
                    <CTableRow>
                      <CTableHeaderCell>{t('tenants.col_name')}</CTableHeaderCell>
                      <CTableHeaderCell>{t('tenants.col_domain')}</CTableHeaderCell>
                      <CTableHeaderCell>{t('tenants.col_max_users')}</CTableHeaderCell>
                      <CTableHeaderCell>{t('tenants.col_status')}</CTableHeaderCell>
                      <CTableHeaderCell className="text-end">{t('tenants.col_actions')}</CTableHeaderCell>
                    </CTableRow>
                  </CTableHead>
                  <CTableBody>
                    {rows.map((tn) => (
                      <CTableRow key={tn.id}>
                        <CTableDataCell className="fw-semibold">{tn.name}</CTableDataCell>
                        <CTableDataCell className="text-muted">{tn.domain}</CTableDataCell>
                        <CTableDataCell>{tn.maxUsers ?? '—'}</CTableDataCell>
                        <CTableDataCell>
                          <CBadge color={tn.active ? 'success' : 'secondary'}>{tn.active ? t('tenants.active') : t('tenants.inactive')}</CBadge>
                        </CTableDataCell>
                        <CTableDataCell className="text-end">
                          <div className="d-flex gap-1 justify-content-end">
                            <CButton size="sm" color="light" onClick={() => openEdit(tn)}><CIcon icon={cilPencil} /></CButton>
                            <CButton size="sm" color={tn.active ? 'warning' : 'success'} onClick={() => handleToggle(tn)}>
                              <CIcon icon={tn.active ? cilBan : cilCheckCircle} />
                            </CButton>
                            <CButton size="sm" color="danger" onClick={() => handleDelete(tn.id)}><CIcon icon={cilTrash} /></CButton>
                          </div>
                        </CTableDataCell>
                      </CTableRow>
                    ))}
                    {!rows.length && (
                      <CTableRow><CTableDataCell colSpan={5} className="text-center text-muted py-4">{t('tenants.empty')}</CTableDataCell></CTableRow>
                    )}
                  </CTableBody>
                </CTable>
              )}
            </CTabPane>
            <CTabPane visible={tab === 'kc-numbers'}>
              <KCNumbersTab tenantsList={rows} />
            </CTabPane>
            <CTabPane visible={tab === 'providers'}>
              <ProvidersTab />
            </CTabPane>
            <CTabPane visible={tab === 'asterisk-servers'}>
              <AsteriskServersTab />
            </CTabPane>
          </CTabContent>
        </CCardBody>
      </CCard>

      <CModal visible={modal} onClose={() => setModal(false)}>
        <CModalHeader><CModalTitle>{editing ? t('tenants.edit') : t('tenants.new_tenant')}</CModalTitle></CModalHeader>
        <CModalBody>
          <CForm className="d-flex flex-column gap-3">
            <div><CFormLabel>{t('tenants.name_label')}</CFormLabel><CFormInput value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Acme Corp" /></div>
            <div><CFormLabel>{t('tenants.domain_label')}</CFormLabel><CFormInput value={form.domain} onChange={(e) => setForm({ ...form, domain: e.target.value })} placeholder="acme.example.com" /></div>
            <div><CFormLabel>{t('tenants.max_users_label')}</CFormLabel><CFormInput type="number" value={form.maxUsers} onChange={(e) => setForm({ ...form, maxUsers: e.target.value })} placeholder="50" /></div>
            <div>
              <CFormLabel>{t('tenants.outbound_provider_label')}</CFormLabel>
              <CFormSelect value={form.outboundProviderId} onChange={(e) => setForm({ ...form, outboundProviderId: e.target.value })}>
                <option value="">{t('tenants.outbound_provider_none')}</option>
                {providersList.map(p => <option key={p.id} value={p.id}>{p.name}</option>)}
              </CFormSelect>
            </div>
            <div>
              <CFormLabel>{t('tenants.outbound_caller_id_label')}</CFormLabel>
              <CFormInput value={form.outboundCallerId} onChange={(e) => setForm({ ...form, outboundCallerId: e.target.value })} placeholder="992370000000" />
              <div className="form-text">{t('tenants.outbound_caller_id_hint')}</div>
            </div>
          </CForm>
        </CModalBody>
        <CModalFooter>
          <CButton color="secondary" onClick={() => setModal(false)}>{t('common.cancel')}</CButton>
          <CButton color="primary" onClick={handleSave} disabled={saving}>{saving ? <CSpinner size="sm" /> : t('common.save')}</CButton>
        </CModalFooter>
      </CModal>
    </>
  )
}
