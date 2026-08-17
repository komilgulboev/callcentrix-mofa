import React from 'react'

const Dashboard    = React.lazy(() => import('src/views/dashboard/Dashboard'))
const Tenants      = React.lazy(() => import('src/views/tenants/Tenants'))
const TenantUsers  = React.lazy(() => import('src/views/tenants/TenantUsers'))
const Users        = React.lazy(() => import('src/views/users/Users'))
const Monitor      = React.lazy(() => import('src/views/monitor/Monitor'))
const CDR          = React.lazy(() => import('src/views/cdr/CDR'))
const Tickets      = React.lazy(() => import('src/views/tickets/Tickets'))
const TicketDetail = React.lazy(() => import('src/views/tickets/TicketDetail'))
const Settings     = React.lazy(() => import('src/views/settings/Settings'))

const routes = [
  { path: '/dashboard',        name: 'Dashboard',      element: Dashboard },
  { path: '/tenants',          name: 'Tenants',        element: Tenants,     roles: [0] },
  { path: '/tenant-users',     name: 'Tenant Users',   element: TenantUsers, roles: [0] },
  { path: '/users',            name: 'Users',          element: Users,       roles: [0, 1] },
  { path: '/monitor',          name: 'Monitor',        element: Monitor,     roles: [0, 1, 2] },
  { path: '/cdr',              name: 'CDR',            element: CDR,         roles: [0, 1, 2] },
  { path: '/tickets',          name: 'Tickets',        element: Tickets },
  { path: '/tickets/:id',      name: 'Ticket',         element: TicketDetail },
  { path: '/settings',         name: 'Settings',       element: Settings,    roles: [0, 1] },
]

export default routes
