import { useQuery } from '@tanstack/react-query'
import { listTenants } from '../services/tenants'
import { useSessionStore } from '../store/session'

export function useTenants() {
  const setTenants = useSessionStore((state) => state.setTenants)

  return useQuery({
    queryKey: ['tenants'],
    queryFn: listTenants,
    staleTime: 5 * 60_000,
    onSuccess: (tenants) => setTenants(tenants),
  })
}
