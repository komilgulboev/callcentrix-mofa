// Asterisk's own PJSIPShowRegistrationsOutbound status strings → badge look.
// "" means registration is off for that provider (nothing to report).
const STATUS_BADGE = {
  Registered:      { color: 'success', label: 'Подключено' },
  Unregistered:    { color: 'secondary', label: 'Не зарегистрирован' },
  Rejected:        { color: 'danger', label: 'Отклонено' },
  'Auth Rejected': { color: 'danger', label: 'Ошибка авторизации' },
  'No response':   { color: 'danger', label: 'Нет ответа' },
  Timeout:         { color: 'danger', label: 'Таймаут' },
  unknown:         { color: 'warning', label: 'Нет данных' },
}

export function statusBadge(status) {
  if (!status) return null
  return STATUS_BADGE[status] || { color: 'warning', label: status }
}

export const PROVIDER_STATUS_POLL_MS = 15000
