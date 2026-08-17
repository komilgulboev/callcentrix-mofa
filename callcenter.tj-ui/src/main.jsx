import React from 'react'
import { createRoot } from 'react-dom/client'
import App from './App'
import './i18n'

import '@coreui/coreui/dist/css/coreui.min.css'
import 'simplebar-react/dist/simplebar.min.css'
import './scss/style.scss'

createRoot(document.getElementById('root')).render(<App />)
