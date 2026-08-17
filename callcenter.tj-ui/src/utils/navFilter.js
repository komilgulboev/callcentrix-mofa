// userType: 0=SuperAdmin, 1=TenantAdmin, 2=Supervisor, 3=Operator

export function filterNav(items, userType) {
  return items.filter((item) => {
    if (!item.roles) return true
    return item.roles.includes(userType)
  })
}

export const ROLES = {
  SUPER_ADMIN:  0,
  TENANT_ADMIN: 1,
  SUPERVISOR:   2,
  OPERATOR:     3,
}
