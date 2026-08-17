import {
  cilSpeedometer,
  cilPeople,
  cilBuilding,
  cilPhone,
  cilHeadphones,
  cilDescription,
  cilList,
  cilSettings,
  cilUserFollow,
} from '@coreui/icons'

const navigation = [
  { component: 'CNavItem',  name: 'Dashboard',    to: '/dashboard',    icon: cilSpeedometer },
  { component: 'CNavTitle', name: 'Management',   roles: [0, 1] },
  { component: 'CNavItem',  name: 'Tenants',      to: '/tenants',      icon: cilBuilding,    roles: [0] },
  { component: 'CNavItem',  name: 'Tenant Users', to: '/tenant-users', icon: cilUserFollow,  roles: [0] },
  { component: 'CNavItem',  name: 'Users',        to: '/users',        icon: cilPeople,      roles: [0, 1] },
  { component: 'CNavTitle', name: 'Call Center' },
  { component: 'CNavItem',  name: 'WebPhone',     to: '/webphone',     icon: cilPhone },
  { component: 'CNavItem',  name: 'Monitor',      to: '/monitor',      icon: cilHeadphones,  roles: [0, 1, 2] },
  { component: 'CNavItem',  name: 'CDR',          to: '/cdr',          icon: cilList,        roles: [0, 1, 2] },
  { component: 'CNavItem',  name: 'Tickets',      to: '/tickets',      icon: cilDescription },
  { component: 'CNavTitle', name: 'Settings',     roles: [0, 1] },
  { component: 'CNavItem',  name: 'Settings',     to: '/settings',     icon: cilSettings,    roles: [0, 1] },
]

export default navigation