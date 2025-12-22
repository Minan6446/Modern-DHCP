import { ApiOutlined, CloudSyncOutlined } from '@ant-design/icons'
import { Card, Col, Form, Input, Row, Switch, Typography } from 'antd'
import { useSessionStore } from '../store/session'

export default function SettingsPage() {
  const activeTenantId = useSessionStore((state) => state.activeTenantId) ?? 'tenant-default'
  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={12}>
        <Card className="glass-panel" title="API 网关">
          <Form layout="vertical" initialValues={{ baseUrl: import.meta.env.VITE_API_BASE_URL ?? '/api/v1' }}>
            <Form.Item label="Base URL" name="baseUrl">
              <Input prefix={<ApiOutlined />} placeholder="https://api.example.com" />
            </Form.Item>
            <Form.Item label="租户 ID">
              <Input value={activeTenantId} disabled />
            </Form.Item>
          </Form>
        </Card>
      </Col>
      <Col xs={24} md={12}>
        <Card className="glass-panel" title="集群偏好">
          <Form layout="vertical">
            <Form.Item label="HA 控制">
              <Switch checkedChildren="启用" unCheckedChildren="关闭" defaultChecked />
            </Form.Item>
            <Form.Item label="审计流">
              <Switch checkedChildren="推送" unCheckedChildren="关闭" />
            </Form.Item>
          </Form>
        </Card>
      </Col>
      <Col span={24}>
        <Card className="glass-panel" title="外部集成">
          <Typography.Paragraph>
            <CloudSyncOutlined /> 支持 webhook、syslog、Prometheus exporters。配置文件位于 configs/config.yaml。
          </Typography.Paragraph>
        </Card>
      </Col>
    </Row>
  )
}
