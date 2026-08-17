import { useCallback, useEffect } from 'react'
import useAuthStore from 'src/store/auth'
import usePhoneStore from 'src/store/phone'

const API_URL = import.meta.env.VITE_API_URL || window.location.origin

function authHeaders() {
  return { Authorization: `Bearer ${localStorage.getItem('accessToken')}` }
}

// Confirms with the backend (AMI-derived) whether the agent still has a live
// channel on Asterisk, and reconciles the store accordingly. Called once
// right after init() — to catch up on a call that was answered before a
// page reload, since the JsSIP session/timer that tracked it doesn't
// survive the reload — and then polled while the store shows a call with no
// live JsSIP session, so a hangup that happens while reattached still
// clears the UI within a few seconds.
function checkActiveCall() {
  return fetch(`${API_URL}/api/phone/active-call`, { headers: authHeaders() })
    .then((r) => (r.ok ? r.json() : { active: false }))
    .then((data) => {
      const { session, status } = usePhoneStore.getState()
      if (data.active) {
        usePhoneStore.getState().rehydrate({
          remoteNumber: data.remoteNumber,
          answeredAt:   Date.now() - data.duration * 1000,
          onHold:       data.onHold,
          channel:      data.channel,
        })
      } else if (!session && (status === 'active' || status === 'on_hold')) {
        // We were showing a reattached call (no live session) that the
        // backend no longer sees as active — it ended while this tab was
        // reloading, or the other party hung up in the meantime.
        usePhoneStore.getState()._resetCall()
      }
    })
    .catch(() => {})
}

// Claims any caller Asterisk is still holding for this agent (see
// ami.Monitor.pendingReconnect on the backend) and redirects them into a
// fresh Dial() at this agent's extension — arrives as an ordinary incoming
// call via the existing JsSIP newRTCSession flow, no special handling here.
function resumeCall() {
  return fetch(`${API_URL}/api/phone/resume-call`, { method: 'POST', headers: authHeaders() }).catch(() => {})
}

// Asterisk only holds a reconnecting caller for a few seconds (see
// writeKCDialplan's post-Queue Wait), and redirecting them back only works
// once this agent's extension has a registered contact to ring — so
// resumeCall() has to fire as soon as registration completes, not on a
// fixed delay after init().
function waitForRegistered(timeoutMs = 4000) {
  return new Promise((resolve) => {
    if (usePhoneStore.getState().status === 'registered') { resolve(); return }
    const timer = setTimeout(() => { unsub(); resolve() }, timeoutMs)
    const unsub = usePhoneStore.subscribe((s) => {
      if (s.status === 'registered') { clearTimeout(timer); unsub(); resolve() }
    })
  })
}

export function usePhoneInit() {
  const user = useAuthStore((s) => s.user)
  const init = usePhoneStore((s) => s.init)
  const destroy = usePhoneStore((s) => s.destroy)

  const load = useCallback(() => {
    if (!user) return

    fetch(`${API_URL}/api/phone/config`, { headers: authHeaders() })
      .then(async (r) => {
        if (!r.ok) {
          const body = await r.json().catch(() => ({}))
          throw new Error(body.error || `phone config request failed (${r.status})`)
        }
        return r.json()
      })
      .then(({ wsUri, sipUri, sipDomain, password, displayName }) => {
        const token = localStorage.getItem('accessToken')
        init({ wsUri: `${wsUri}?token=${token}`, sipUri, sipDomain, password, displayName })
        checkActiveCall()
        waitForRegistered().then(resumeCall)
      })
      .catch((err) => {
        console.error('Phone init failed:', err.message)
        usePhoneStore.setState({ status: 'failed', configError: err.message })
      })
  }, [user, init])

  useEffect(() => {
    usePhoneStore.setState({ retryInit: load })
  }, [load])

  useEffect(() => {
    load()
    return () => destroy()
  }, [user?.id])

  // Reconcile a reattached (session-less) call display with Asterisk every
  // few seconds — the only way to notice it ended, since there's no live
  // JsSIP session to fire an 'ended' event.
  useEffect(() => {
    if (!user) return
    const interval = setInterval(() => {
      const { session, status } = usePhoneStore.getState()
      if (!session && (status === 'active' || status === 'on_hold')) checkActiveCall()
    }, 5000)
    return () => clearInterval(interval)
  }, [user?.id])

  // A page reload/close kills the operator's PJSIP/WebRTC leg outright.
  // resumeCall() above gives a short (~5s) window to redirect the caller
  // back once this tab reconnects, but that window is tight — network/SIP
  // registration delays can easily blow past it — so warning before an
  // accidental reload is still the primary defense, not just a fallback.
  useEffect(() => {
    const handler = (e) => {
      const { status } = usePhoneStore.getState()
      if (['ringing_out', 'active', 'on_hold'].includes(status)) {
        e.preventDefault()
        e.returnValue = ''
      }
    }
    window.addEventListener('beforeunload', handler)
    return () => window.removeEventListener('beforeunload', handler)
  }, [])
}
