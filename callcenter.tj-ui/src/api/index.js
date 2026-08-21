/**
 * CallCentrix API client
 */

const BASE_URL = import.meta.env.VITE_API_URL || window.location.origin

function getToken() {
  return localStorage.getItem('accessToken')
}

async function request(method, path, body) {
  const headers = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`

  const res = await fetch(`${BASE_URL}${path}`, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })

  if (res.status === 401) {
    localStorage.removeItem('accessToken')
    window.location.hash = '#/login'
    throw new Error('Unauthorized')
  }

  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || err.message || `HTTP ${res.status}`)
  }

  if (res.status === 204) return null
  return res.json()
}

const get   = (path)       => request('GET',    path)
const post  = (path, body) => request('POST',   path, body)
const put   = (path, body) => request('PUT',    path, body)
const del   = (path)       => request('DELETE', path)
const patch = (path, body) => request('PATCH',  path, body)

// ─── Auth ────────────────────────────────────────────────────
export const auth = {
  login:      (username, password) => post('/api/auth/login', { username, password }),
  me:         ()                   => get('/api/auth/me'),
  logout:     ()                   => post('/api/auth/logout'),
  register:   (data)               => post('/api/auth/register', data),
  verifyCode: (username, code)     => post('/api/auth/verify-code', { username, code }),
  changePassword: (currentPassword, newPassword) =>
    patch('/api/auth/password', { currentPassword, newPassword }),
}

// ─── Tenants ─────────────────────────────────────────────────
export const tenants = {
  list:         ()         => get('/api/tenants'),
  get:          (id)       => get(`/api/tenants/${id}`),
  create:       (data)     => post('/api/tenants', data),
  update:       (id, data) => put(`/api/tenants/${id}`, data),
  remove:       (id)       => del(`/api/tenants/${id}`),
  activate:     (id)       => patch(`/api/tenants/${id}/activate`),
  deactivate:   (id)       => patch(`/api/tenants/${id}/deactivate`),
  assignUser:   (id, uid)  => post(`/api/tenants/${id}/users`, { userId: uid }),
  unassignUser: (id, uid)  => del(`/api/tenants/${id}/users/${uid}`),
}

// ─── Users ───────────────────────────────────────────────────
export const users = {
  list:             (params)  => get('/api/users' + toQuery(params)),
  get:              (id)      => get(`/api/users/${id}`),
  create:           (data)    => post('/api/users', data),
  update:           (id, data) => put(`/api/users/${id}`, data),
  remove:           (id)      => del(`/api/users/${id}`),
  activate:         (id)      => patch(`/api/users/${id}/activate`),
  deactivate:       (id)      => patch(`/api/users/${id}/deactivate`),
  resetPwd:         (id, pwd) => patch(`/api/users/${id}/password`, { password: pwd }),
  listUnauthorized: ()        => get('/api/users/unauthorized'),
  authorize:        (id)      => post(`/api/users/${id}/authorize`),
}

// ─── Tickets ─────────────────────────────────────────────────
export const tickets = {
  list:    (params)    => get('/api/tickets' + toQuery(params)),
  get:     (id)        => get(`/api/tickets/${id}`),
  create:  (data)      => post('/api/tickets', data),
  update:  (id, data)  => put(`/api/tickets/${id}`, data),
  remove:  (id)        => del(`/api/tickets/${id}`),
  comment: (id, text)  => post(`/api/tickets/${id}/comments`, { text }),
  comments:(id)        => get(`/api/tickets/${id}/comments`),
  assign:  (id, userId) => patch(`/api/tickets/${id}/assign`, { userId }),
  assignableUsers: ()   => get('/api/tickets/assignable-users'),
}

// ─── Tasks ───────────────────────────────────────────────────
export const tasks = {
  list:            (params)     => get('/api/tasks' + toQuery(params)),
  get:             (id)         => get(`/api/tasks/${id}`),
  create:          (data)       => post('/api/tasks', data),
  update:          (id, data)   => put(`/api/tasks/${id}`, data),
  remove:          (id)         => del(`/api/tasks/${id}`),
  updateStatus:    (id, status) => patch(`/api/tasks/${id}/status`, { status }),
  assignableUsers: (tenantId)   => get('/api/tasks/assignable-users' + toQuery({ tenantId })),
  notifications:            ()   => get('/api/tasks/notifications'),
  markNotificationRead:     (id) => patch(`/api/tasks/notifications/${id}/read`),
  markAllNotificationsRead: ()   => patch('/api/tasks/notifications/read-all'),
}

// ─── Reports ─────────────────────────────────────────────────
export const reports = {
  tickets: (params) => get('/api/reports/tickets' + toQuery(params)),
}

// ─── Blacklist ───────────────────────────────────────────────
export const blacklist = {
  list:   (tenantId, search) => get(`/api/blacklist?tenantId=${tenantId}` + (search ? `&search=${encodeURIComponent(search)}` : '')),
  create: (tenantId, data)   => post(`/api/blacklist?tenantId=${tenantId}`, data),
  update: (tenantId, id, data) => put(`/api/blacklist/${id}?tenantId=${tenantId}`, data),
  remove: (tenantId, id)     => del(`/api/blacklist/${id}?tenantId=${tenantId}`),
  toggle: (tenantId, id)     => patch(`/api/blacklist/${id}/toggle?tenantId=${tenantId}`),
  check:  (tenantId, phone)  => get(`/api/blacklist/check?tenantId=${tenantId}&phone=${encodeURIComponent(phone)}`),
}

// ─── Whitelist ───────────────────────────────────────────────
export const whitelist = {
  list:   (tenantId, search) => get(`/api/whitelist?tenantId=${tenantId}` + (search ? `&search=${encodeURIComponent(search)}` : '')),
  create: (tenantId, data)   => post(`/api/whitelist?tenantId=${tenantId}`, data),
  update: (tenantId, id, data) => put(`/api/whitelist/${id}?tenantId=${tenantId}`, data),
  remove: (tenantId, id)     => del(`/api/whitelist/${id}?tenantId=${tenantId}`),
  toggle: (tenantId, id)     => patch(`/api/whitelist/${id}/toggle?tenantId=${tenantId}`),
  check:  (tenantId, phone)  => get(`/api/whitelist/check?tenantId=${tenantId}&phone=${encodeURIComponent(phone)}`),
}

// ─── Topics ──────────────────────────────────────────────────
export const topics = {
  my:     ()                    => get('/api/topics'),
  list:   (tenantId)            => get(`/api/tenants/${tenantId}/topics`),
  create: (tenantId, data)      => post(`/api/tenants/${tenantId}/topics`, data),
  update: (tenantId, id, data)  => put(`/api/tenants/${tenantId}/topics/${id}`, data),
  remove: (tenantId, id)        => del(`/api/tenants/${tenantId}/topics/${id}`),
}

// ─── Knowledge Base ──────────────────────────────────────────
export const knowledgeBase = {
  categories:     (tenantId)   => get('/api/kb/categories' + toQuery({ tenantId })),
  createCategory: (data)       => post('/api/kb/categories', data),
  updateCategory: (id, data)   => put(`/api/kb/categories/${id}`, data),
  removeCategory: (id)         => del(`/api/kb/categories/${id}`),
  articles:       (params)     => get('/api/kb/articles' + toQuery(params)),
  article:        (id)         => get(`/api/kb/articles/${id}`),
  createArticle:  (data)       => post('/api/kb/articles', data),
  updateArticle:  (id, data)   => put(`/api/kb/articles/${id}`, data),
  removeArticle:  (id)         => del(`/api/kb/articles/${id}`),
  tags:           (tenantId)   => get('/api/kb/tags' + toQuery({ tenantId })),
  mediaUrl:       (articleId, mediaId) => `${BASE_URL}/api/kb/articles/${articleId}/media/${mediaId}?token=${getToken()}`,
  // type: 'photo' | 'video'
  uploadMedia:    (articleId, file, type) => {
    const fd = new FormData()
    fd.append('file', file)
    fd.append('type', type)
    const token = localStorage.getItem('accessToken')
    return fetch(`${BASE_URL}/api/kb/articles/${articleId}/media`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    }).then(async r => {
      if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText) }
      return r.json()
    })
  },
  removeMedia:    (articleId, mediaId) => del(`/api/kb/articles/${articleId}/media/${mediaId}`),
}

// ─── SIP Providers (carrier trunks) ────────────────────────────
export const providers = {
  list:   ()       => get('/api/providers'),
  create: (data)    => post('/api/providers', data),
  update: (id, data) => put(`/api/providers/${id}`, data),
  remove: (id)      => del(`/api/providers/${id}`),
}

// ─── Asterisk Servers (multi-box telephony, single shared DB) ─────────
export const asteriskServers = {
  list:   ()       => get('/api/asterisk-servers'),
  create: (data)    => post('/api/asterisk-servers', data),
  update: (id, data) => put(`/api/asterisk-servers/${id}`, data),
  remove: (id)      => del(`/api/asterisk-servers/${id}`),
}

// ─── KC Numbers (call-center DIDs) ─────────────────────────────
export const kcNumbers = {
  // overview table: numbers + config status for the caller's own tenant
  list:   (tenantId)         => get(`/api/kc-numbers?tenantId=${tenantId}`),
  // SuperAdmin-only management, nested under a specific tenant
  create: (tenantId, number, providerId) => post(`/api/tenants/${tenantId}/kc-numbers`, { number, providerId }),
  remove: (tenantId, numberId) => del(`/api/tenants/${tenantId}/kc-numbers/${numberId}`),
}

// ─── IVR / Queue (scoped to a single KC number) ────────────────
export const ivr = {
  get:            (kcId)        => get(`/api/kc-numbers/${kcId}/ivr`),
  updateConfig:   (kcId, data)  => put(`/api/kc-numbers/${kcId}/ivr`, data),
  sync:           (kcId)        => post(`/api/kc-numbers/${kcId}/ivr/sync`),
  uploadGreeting: (kcId, file)  => {
    const fd = new FormData()
    fd.append('file', file)
    const token = localStorage.getItem('accessToken')
    return fetch(`${BASE_URL}/api/kc-numbers/${kcId}/ivr/greeting`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    }).then(async r => {
      if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText) }
      return r.json()
    })
  },
  uploadClosedGreeting: (kcId, file) => {
    const fd = new FormData()
    fd.append('file', file)
    const token = localStorage.getItem('accessToken')
    return fetch(`${BASE_URL}/api/kc-numbers/${kcId}/ivr/closed-greeting`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    }).then(async r => {
      if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText) }
      return r.json()
    })
  },
  uploadMOH: (kcId, file) => {
    const fd = new FormData()
    fd.append('file', file)
    const token = localStorage.getItem('accessToken')
    return fetch(`${BASE_URL}/api/kc-numbers/${kcId}/ivr/moh`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    }).then(async r => {
      if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText) }
      return r.json()
    })
  },
  listOptions:    (kcId)        => get(`/api/kc-numbers/${kcId}/ivr/options`),
  saveOption:     (kcId, data)  => post(`/api/kc-numbers/${kcId}/ivr/options`, data),
  deleteOption:   (kcId, digit) => del(`/api/kc-numbers/${kcId}/ivr/options/${digit}`),
  listMembers:    (kcId)        => get(`/api/kc-numbers/${kcId}/ivr/members`),
  addMember:      (kcId, username) => post(`/api/kc-numbers/${kcId}/ivr/members`, { username }),
  removeMember:   (kcId, username) => del(`/api/kc-numbers/${kcId}/ivr/members/${username}`),
  availableUsers: (kcId)        => get(`/api/kc-numbers/${kcId}/ivr/available-users`),
}

// ─── CDR ─────────────────────────────────────────────────────
export const cdr = {
  list:  (params) => get('/api/cdr' + toQuery(params)),
  get:   (id)     => get(`/api/cdr/${id}`),
  audio: (id)     => `${BASE_URL}/api/cdr/${id}/audio?token=${getToken()}`,
}

// ─── Monitor (embedded in the Dashboard's live agent/call widget) ─────────
export const monitor = {
  tenantSnapshot: () => get('/api/dashboard/monitor'),
  pause:          (agentId) => post('/api/actions/pause',   { agentId }),
  unpause:        (agentId) => post('/api/actions/unpause', { agentId }),
  hangup:         (channel) => post('/api/actions/hangup',  { channel }),
}

// ─── Version (public, unauthenticated) ────────────────────────
export const version = {
  get: () => get('/api/version'),
}

// ─── Settings / Branding (branding GETs are public, unauthenticated) ──
export const settings = {
  branding:       ()     => get('/api/settings/branding'),
  updateBranding: (data) => put('/api/settings/branding', data),
  logoUrl:        ()     => `${BASE_URL}/api/settings/branding/logo`,
  uploadLogo:     (file) => {
    const fd = new FormData()
    fd.append('file', file)
    const token = localStorage.getItem('accessToken')
    return fetch(`${BASE_URL}/api/settings/branding/logo`, {
      method: 'POST',
      headers: token ? { Authorization: `Bearer ${token}` } : {},
      body: fd,
    }).then(async r => {
      if (!r.ok) { const e = await r.json().catch(() => ({})); throw new Error(e.error || r.statusText) }
      return r.json()
    })
  },
  smpp:       ()     => get('/api/settings/smpp'),
  updateSmpp: (data) => put('/api/settings/smpp', data),
  telegram:       ()     => get('/api/settings/telegram'),
  updateTelegram: (data) => put('/api/settings/telegram', data),
}

// ─── Helpers ─────────────────────────────────────────────────
function toQuery(params) {
  if (!params) return ''
  const q = new URLSearchParams(
    Object.fromEntries(Object.entries(params).filter(([, v]) => v != null))
  ).toString()
  return q ? `?${q}` : ''
}
