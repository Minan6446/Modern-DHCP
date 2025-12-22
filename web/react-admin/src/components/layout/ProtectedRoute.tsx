import { Spin } from 'antd'
import { Suspense, type ReactNode } from 'react'
import { Navigate } from 'react-router-dom'
import { useSessionStore } from '../../store/session'

type ProtectedRouteProps = {
  children: ReactNode
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const isAuthenticated = useSessionStore((state) => state.isAuthenticated)
  const bootstrapped = useSessionStore((state) => state.bootstrapped)

  if (!bootstrapped) {
    return (
      <div className="auth-viewport">
        <Spin tip="正在拉取会话..." size="large" />
      </div>
    )
  }

  if (!isAuthenticated) {
    return <Navigate to="/auth/login" replace />
  }

  return <Suspense fallback={<Spin />}>{children}</Suspense>
}
