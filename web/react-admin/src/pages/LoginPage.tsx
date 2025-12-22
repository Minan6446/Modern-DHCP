import { LockOutlined, MailOutlined } from '@ant-design/icons'
import { useMutation } from '@tanstack/react-query'
import { Button, Form, Input, Typography, message } from 'antd'
import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { login } from '../services/auth'
import { useSessionStore } from '../store/session'
import type { LoginResponse } from '../types/api'

type LoginForm = {
  username: string
  password: string
}

export default function LoginPage() {
  const navigate = useNavigate()
  const isAuthenticated = useSessionStore((state) => state.isAuthenticated)
  const setSession = useSessionStore((state) => state.setSession)

  useEffect(() => {
    if (isAuthenticated) {
      navigate('/app/dashboard', { replace: true })
    }
  }, [isAuthenticated, navigate])

  const { mutateAsync, isPending } = useMutation({
    mutationFn: (values: LoginForm) => login(values),
    onSuccess: (payload: LoginResponse) => {
      setSession(payload)
      message.success('欢迎回来')
      navigate('/app/dashboard')
    },
  })

  const handleFinish = async (values: LoginForm) => {
    try {
      await mutateAsync(values)
    } catch {
      message.error('登录失败，请检查凭证')
    }
  }

  return (
    <div className="auth-viewport">
      <div className="auth-panel">
        <Typography.Title level={2}>Modern DHCP</Typography.Title>
        <Typography.Paragraph>以自动化方式管理地址池、租约和全局策略。</Typography.Paragraph>
        <Form layout="vertical" onFinish={handleFinish} requiredMark={false}>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请输入用户名' }]}>
            <Input prefix={<MailOutlined />} placeholder="super.admin" size="large" />
          </Form.Item>
          <Form.Item name="password" label="密码" rules={[{ required: true, message: '请输入密码' }]}>
            <Input.Password prefix={<LockOutlined />} placeholder="••••••" size="large" />
          </Form.Item>
          <Button type="primary" htmlType="submit" loading={isPending} block size="large">
            进入控制平面
          </Button>
        </Form>
      </div>
    </div>
  )
}
