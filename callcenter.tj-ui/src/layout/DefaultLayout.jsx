import React from 'react'
import { AppContent, AppFooter, AppHeader, AppSidebar } from 'src/components'
import PhoneWidget from 'src/components/phone/PhoneWidget'
import { usePhoneInit } from 'src/hooks/usePhoneInit'
import useSessionExpiry from 'src/hooks/useSessionExpiry'

export default function DefaultLayout() {
  // Initialize JsSIP once for the whole app session
  usePhoneInit()

  // Auto-logout as soon as the JWT expires, without waiting for the next
  // navigation or API call to reveal it via a 401.
  useSessionExpiry()

  return (
    <div>
      {/* Single persistent audio sink for the call's remote track — must
          never unmount across route changes (see store/phone.js
          attachRemoteAudio, which attaches the WebRTC track to this element
          by id once, on the 'track' event). Previously PhoneWidget and the
          /webphone page each rendered their own #cx-remote-audio, so
          navigating between them destroyed whichever one the track had
          already attached to and nothing ever re-attached it — no sound
          until hold/unhold forced renegotiation. */}
      <audio id="cx-remote-audio" autoPlay />
      <AppSidebar />
      <div className="wrapper d-flex flex-column min-vh-100">
        <AppHeader />
        <div className="body flex-grow-1">
          <AppContent />
        </div>
        <AppFooter />
      </div>
      <PhoneWidget />
    </div>
  )
}
