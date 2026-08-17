import { create } from 'zustand'
import { persist, createJSONStorage } from 'zustand/middleware'
import JsSIP from 'jssip'

// Temporarily enabled for diagnosing the hosted-deployment call issue — revert once resolved.
JsSIP.debug.enable('JsSIP:*')

const IN_CALL_STATUSES = ['ringing_in', 'ringing_out', 'active', 'on_hold']
const isInCall = (status) => IN_CALL_STATUSES.includes(status)

// Without a STUN server the browser only offers host candidates, which are
// useless whenever Asterisk sits behind Docker/NAT on a different private
// subnet than the client (see hosted-deployment ICE failures).
const PC_CONFIG = { iceServers: [{ urls: 'stun:stun.l.google.com:19302' }] }

// Synthesized ringtone (no bundled audio asset) — two short beeps repeating
// every 2s while a call is in the 'ringing_in' state.
let ringCtx = null
let ringInterval = null

function ringtoneBeep() {
  if (!ringCtx) return
  if (ringCtx.state === 'suspended') ringCtx.resume().catch(() => {})
  const now = ringCtx.currentTime
  ;[0, 0.35].forEach((offset) => {
    const osc  = ringCtx.createOscillator()
    const gain = ringCtx.createGain()
    osc.type = 'sine'
    osc.frequency.value = 880
    gain.gain.setValueAtTime(0.0001, now + offset)
    gain.gain.exponentialRampToValueAtTime(0.25, now + offset + 0.03)
    gain.gain.exponentialRampToValueAtTime(0.0001, now + offset + 0.3)
    osc.connect(gain).connect(ringCtx.destination)
    osc.start(now + offset)
    osc.stop(now + offset + 0.3)
  })
}

function startRingtone() {
  if (ringInterval) return
  try {
    ringCtx = ringCtx || new (window.AudioContext || window.webkitAudioContext)()
    ringtoneBeep()
    ringInterval = setInterval(ringtoneBeep, 2000)
  } catch {}
}

function stopRingtone() {
  if (ringInterval) { clearInterval(ringInterval); ringInterval = null }
}

// Attach the remote track to the shared <audio> element as soon as it arrives.
function attachRemoteAudio(peerconnection) {
  peerconnection.addEventListener('track', (e) => {
    const audio = document.getElementById('cx-remote-audio')
    if (!audio || !e.streams?.[0]) return
    audio.srcObject = e.streams[0]
    // autoplay isn't reliable once srcObject is assigned programmatically
    // (esp. Firefox) — force playback and surface autoplay-policy blocks.
    audio.play().catch((err) => console.error('[Phone] remote audio play() blocked:', err))
  })
}

function startDurationTimer(answeredAt, set) {
  return setInterval(
    () => set({ callDuration: Math.max(0, Math.floor((Date.now() - answeredAt) / 1000)) }),
    1000,
  )
}

const usePhoneStore = create(
  persist(
    (set, get) => ({
      ua: null,
      session: null,
      status: 'idle',       // idle | connecting | registered | ringing_in | ringing_out | active | on_hold | failed
      remoteNumber: '',
      callDuration: 0,
      // Wall-clock time the call was answered. Duration is always derived
      // from this (not incremented) so it survives a page reload intact and
      // doesn't drift on long calls the way a naive setInterval counter does.
      answeredAt: null,
      // Asterisk channel name, known only once the backend has confirmed a
      // reattached call (see rehydrate()) — lets hangup() reach AMI directly
      // when there's no live JsSIP session to terminate() locally.
      channel: null,
      isMuted: false,
      _timer: null,

      // Set by init()/usePhoneInit so the UI can offer a fix when the socket can't connect
      wsUri: '',
      sipDomain: 'localhost',
      configError: '',
      retryInit: null,

      // Recent calls during this session
      callHistory: [],

      init({ wsUri, sipUri, sipDomain, password, displayName }) {
        get()._teardownUA()
        set({ wsUri, sipDomain: sipDomain || 'localhost', configError: '' })

        const socket = new JsSIP.WebSocketInterface(wsUri)
        const ua = new JsSIP.UA({
          sockets:      [socket],
          uri:          sipUri,
          password,
          display_name: displayName,
          register:     true,
        })

        ua.on('connecting',   () => { if (!isInCall(get().status)) set({ status: 'connecting' }) })
        ua.on('registered',   () => { if (!isInCall(get().status)) set({ status: 'registered' }) })
        ua.on('unregistered', () => { if (!isInCall(get().status)) set({ status: 'idle' }) })
        ua.on('registrationFailed', () => { if (!isInCall(get().status)) set({ status: 'failed' }) })
        ua.on('disconnected', () => {
          if (!isInCall(get().status)) set({ status: 'failed' })
        })

        ua.on('newRTCSession', ({ session, originator }) => {
          if (originator !== 'remote') return
          const { status } = get()
          const busy = ['ringing_out', 'active', 'on_hold'].includes(status)
          if (busy) {
            // Forked leg of a call we're already on (or an unrelated second
            // call while busy) — reject it explicitly instead of clobbering
            // the in-progress session in the store.
            try { session.terminate() } catch {}
            return
          }
          get()._bindSession(session, 'ringing_in')
        })

        ua.start()
        set({ ua })
      },

      // Tears down the live SIP transport only — call state fields
      // (status/remoteNumber/answeredAt/channel) are left untouched so a
      // call rehydrated from sessionStorage (or still being confirmed by
      // the backend) survives re-running init() on every mount.
      _teardownUA() {
        const { ua, _timer } = get()
        if (_timer) clearInterval(_timer)
        stopRingtone()
        if (ua) try { ua.stop() } catch {}
        set({ ua: null, session: null, _timer: null })
      },

      // Full reset for real teardown (logout / user switch) — unlike
      // _teardownUA(), this also clears the call display.
      destroy() {
        get()._teardownUA()
        set({ status: 'idle', remoteNumber: '', callDuration: 0, answeredAt: null, channel: null })
      },

      call(number) {
        const { ua, sipDomain } = get()
        if (!ua) return
        const session = ua.call(`sip:${number}@${sipDomain}`, {
          mediaConstraints: { audio: true, video: false },
          pcConfig: PC_CONFIG,
          // Outgoing calls create the RTCPeerConnection (and emit 'peerconnection')
          // synchronously inside connect(), before ua.call() even returns — a plain
          // session.on('peerconnection', ...) attached afterwards in _bindSession
          // misses it. eventHandlers is bound before the connection is created.
          eventHandlers: {
            peerconnection: ({ peerconnection }) => attachRemoteAudio(peerconnection),
          },
        })
        get()._bindSession(session, 'ringing_out', number)
      },

      answer() {
        const { session } = get()
        if (!session) return
        stopRingtone()
        session.answer({
          mediaConstraints: { audio: true, video: false },
          sessionTimersExpires: 120,
          pcConfig: PC_CONFIG,
        })
      },

      hangup() {
        const { session, channel, status } = get()
        stopRingtone()
        if (session) {
          if (status === 'active' || status === 'on_hold') {
            // Best-effort signal that this call end is deliberate, so the
            // backend doesn't hold the caller for a possible reconnect (see
            // ami.Monitor.MarkDeliberateHangup) once our SIP BYE lands.
            const API_URL = import.meta.env.VITE_API_URL || window.location.origin
            const token = localStorage.getItem('accessToken')
            fetch(`${API_URL}/api/phone/hangup-intent`, {
              method: 'POST',
              headers: { Authorization: `Bearer ${token}` },
            }).catch(() => {})
          }
          try { session.terminate() } catch {}
          get()._resetCall()
          return
        }
        // No live session — this is a call reattached after a page reload.
        // Ask the backend to hang it up on Asterisk directly via AMI.
        if (channel) {
          const API_URL = import.meta.env.VITE_API_URL || window.location.origin
          const token = localStorage.getItem('accessToken')
          fetch(`${API_URL}/api/phone/hangup`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
            body: JSON.stringify({ channel }),
          }).catch((err) => console.error('[Phone] AMI hangup failed:', err))
        }
        get()._resetCall()
      },

      toggleMute() {
        const { session, isMuted } = get()
        if (!session) return
        isMuted ? session.unmute() : session.mute()
        set({ isMuted: !isMuted })
      },

      toggleHold() {
        const { session, status } = get()
        if (!session) return
        // JsSIP's signature is hold(options, done)/unhold(options, done) —
        // the callback is the *second* argument. Passing it as the first
        // (as this used to) makes it silently become `options`, so `done`
        // is never invoked and the store's status never flips back,
        // leaving the UI stuck showing "On Hold" after a real unhold.
        if (status === 'on_hold') {
          session.unhold({}, () => set({ status: 'active' }))
        } else {
          session.hold({}, () => set({ status: 'on_hold' }))
        }
      },

      sendDtmf(tone) {
        const { session } = get()
        if (session) session.sendDTMF(tone)
      },

      // Restore call info/timer after a page reload, using the backend's
      // AMI-derived view of the agent's still-live channel as source of
      // truth. There is no live JsSIP session to reattach to — WebRTC media
      // doesn't survive a reload — so this only restores the display and
      // AMI-backed hangup control, not audio.
      rehydrate({ remoteNumber, answeredAt, onHold, channel }) {
        const { _timer } = get()
        if (_timer) clearInterval(_timer)
        set({
          status: onHold ? 'on_hold' : 'active',
          remoteNumber,
          answeredAt,
          channel: channel ?? null,
          callDuration: Math.max(0, Math.floor((Date.now() - answeredAt) / 1000)),
          _timer: startDurationTimer(answeredAt, set),
        })
      },

      _bindSession(session, initialStatus, number) {
        const remote = number || session.remote_identity?.uri?.user || '?'
        const startedAt = Date.now()
        set({ session, status: initialStatus, remoteNumber: remote, channel: null })
        if (initialStatus === 'ringing_in') startRingtone()

        // Attach remote audio as soon as track arrives (before confirmed).
        // Covers incoming calls: their peerconnection is created later, on answer(),
        // well after this listener is attached. Outgoing calls attach via
        // eventHandlers in call() instead — see the comment there.
        session.on('peerconnection', ({ peerconnection }) => attachRemoteAudio(peerconnection))

        session.on('confirmed', () => {
          stopRingtone()
          const answeredAt = Date.now()
          set({ status: 'active', answeredAt, callDuration: 0, _timer: startDurationTimer(answeredAt, set) })
        })

        const addHistory = (result) => {
          const { remoteNumber, callDuration } = get()
          set((s) => ({
            callHistory: [
              {
                id:       Date.now(),
                number:   remoteNumber,
                duration: callDuration,
                result,
                direction: initialStatus === 'ringing_in' ? 'in' : 'out',
                time:     new Date(startedAt).toLocaleTimeString(),
              },
              ...s.callHistory.slice(0, 49),
            ],
          }))
        }

        session.on('ended',  (e) => {
          stopRingtone()
          addHistory('ended')
          if (get().session === session) get()._resetCall()
        })
        session.on('failed', (e) => {
          stopRingtone()
          console.error('[JsSIP] call failed:', e.cause, e.message)
          addHistory('missed')
          if (get().session === session) get()._resetCall()
        })
      },

      _resetCall() {
        const { _timer } = get()
        if (_timer) clearInterval(_timer)
        set({
          session: null, status: 'registered',
          remoteNumber: '', callDuration: 0,
          isMuted: false, _timer: null,
          answeredAt: null, channel: null,
        })
      },
    }),
    {
      name: 'cx-active-call',
      storage: createJSONStorage(() => sessionStorage),
      // Only persist enough to redraw an already-answered call after reload —
      // ringing calls can't be resumed (no live SDP), so they're excluded.
      partialize: (s) => (
        s.status === 'active' || s.status === 'on_hold'
          ? { status: s.status, remoteNumber: s.remoteNumber, answeredAt: s.answeredAt, channel: s.channel }
          : {}
      ),
    },
  ),
)

export default usePhoneStore
