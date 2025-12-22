import { useQuery } from '@tanstack/react-query'
import { fetchUiMetadata } from '../services/ui'

export function useCapabilities() {
  return useQuery({
    queryKey: ['ui-metadata'],
    queryFn: fetchUiMetadata,
    staleTime: 5 * 60_000,
  })
}
