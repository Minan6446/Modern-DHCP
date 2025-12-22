import { Button, Result } from 'antd'
import { useNavigate } from 'react-router-dom'

export default function NotFoundPage() {
  const navigate = useNavigate()

  return (
    <Result
      status="404"
      title="页面不存在"
      subTitle="请检查链接或返回控制面板"
      extra={
        <Button type="primary" onClick={() => navigate('/app/dashboard')}>
          返回首页
        </Button>
      }
    />
  )
}
