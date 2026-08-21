import React from 'react'

const Dashboard    = React.lazy(() => import('src/views/dashboard/Dashboard'))
const Tenants      = React.lazy(() => import('src/views/tenants/Tenants'))
const TenantUsers  = React.lazy(() => import('src/views/tenants/TenantUsers'))
const Users        = React.lazy(() => import('src/views/users/Users'))
const UnauthorizedUsers = React.lazy(() => import('src/views/users/UnauthorizedUsers'))
const CDR          = React.lazy(() => import('src/views/cdr/CDR'))
const Tickets      = React.lazy(() => import('src/views/tickets/Tickets'))
const TicketDetail = React.lazy(() => import('src/views/tickets/TicketDetail'))
const Topics       = React.lazy(() => import('src/views/topics/Topics'))
const KnowledgeBase         = React.lazy(() => import('src/views/knowledgebase/KnowledgeBase'))
const KnowledgeBaseCategory = React.lazy(() => import('src/views/knowledgebase/KnowledgeBaseCategory'))
const KnowledgeBaseArticle  = React.lazy(() => import('src/views/knowledgebase/KnowledgeBaseArticle'))
const KnowledgeBaseEditor   = React.lazy(() => import('src/views/knowledgebase/KnowledgeBaseEditor'))
const IVR          = React.lazy(() => import('src/views/ivr/IVR'))
const Blacklist    = React.lazy(() => import('src/views/blacklist/Blacklist'))
const Whitelist    = React.lazy(() => import('src/views/whitelist/Whitelist'))
const TicketsReport = React.lazy(() => import('src/views/reports/TicketsReport'))
const Settings     = React.lazy(() => import('src/views/settings/Settings'))
const Phone = React.lazy(() => import('src/views/phone/Phone.jsx'))

const routes = [
  { path: '/dashboard',    name: 'Dashboard',    element: Dashboard },
  { path: '/tenants',      name: 'Tenants',      element: Tenants,     roles: [0] },
  { path: '/tenant-users', name: 'Tenant Users', element: TenantUsers, roles: [0] },
  { path: '/users',        name: 'Users',        element: Users,       roles: [0, 1] },
  { path: '/users/unauthorized', name: 'Unauthorized Users', element: UnauthorizedUsers, roles: [0] },
  { path: '/webphone', name: 'WebPhone', element: Phone }, // без roles — для всех
  { path: '/cdr',          name: 'CDR',          element: CDR,         roles: [0, 1, 2] },
  { path: '/tickets',      name: 'Tickets',      element: Tickets },
  { path: '/tickets/:id',  name: 'Ticket',       element: TicketDetail },
  { path: '/topics',       name: 'Topics',       element: Topics,      roles: [0, 1] },
  { path: '/knowledge-base',                  name: 'KnowledgeBase',         element: KnowledgeBase },
  { path: '/knowledge-base/new',              name: 'KnowledgeBaseNew',      element: KnowledgeBaseEditor,  roles: [1] },
  { path: '/knowledge-base/category/:id',     name: 'KnowledgeBaseCategory', element: KnowledgeBaseCategory },
  { path: '/knowledge-base/article/:id',      name: 'KnowledgeBaseArticle',  element: KnowledgeBaseArticle },
  { path: '/knowledge-base/article/:id/edit', name: 'KnowledgeBaseEdit',     element: KnowledgeBaseEditor,  roles: [1] },
  { path: '/ivr',          name: 'IVR',          element: IVR,         roles: [0, 1, 2] },
  { path: '/blacklist',    name: 'Blacklist',    element: Blacklist,   roles: [0, 1] },
  { path: '/whitelist',    name: 'Whitelist',    element: Whitelist,   roles: [0, 1] },
  { path: '/reports/tickets', name: 'Tickets Report', element: TicketsReport, roles: [0, 1, 2] },
  { path: '/settings',     name: 'Settings',     element: Settings }, // без roles — для всех
]

export default routes
