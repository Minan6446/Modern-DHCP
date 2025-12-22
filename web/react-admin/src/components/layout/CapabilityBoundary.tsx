import { Result, Spin } from 'antd'
import type { ReactNode } from 'react'
import { useCapabilities } from '../../hooks/useCapabilities'
import { useI18n } from '../../hooks/useI18n'

type Props = {
  capability?: string
  children: ReactNode
}

export default function CapabilityBoundary({ capability, children }: Props) {
  const { data, isLoading } = useCapabilities()
  const { t } = useI18n()

  if (!capability) return <>{children}</>
  if (isLoading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', padding: 64 }}>
        <Spin tip={t('permissionsLoading')} size="large" />
      </div>
    )
  }

  const granted = new Set([...(data?.granted ?? []), ...(data?.temporary ?? [])])
  if (granted.has(capability)) {
    return <>{children}</>
  }

  return (
    <Result
      status="403"
      title={t('capabilityDeniedTitle')}
      subTitle={t('capabilityDeniedSubtitle')}
    />
  )
}
