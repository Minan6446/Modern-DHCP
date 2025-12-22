import { SafetyOutlined } from '@ant-design/icons'
import { Button, Card, Col, Collapse, Empty, Row, Space, Tag, Typography } from 'antd'

export default function PolicyPage() {
  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={12}>
        <Card
          className="glass-panel"
          title="策略草稿"
          extra={<Button type="primary">新建策略</Button>}
        >
          <Empty description="等待导入 RBAC / 配额策略" />
        </Card>
      </Col>
      <Col xs={24} md={12}>
        <Card className="glass-panel" title="安全基线">
          <Space direction="vertical" size="large" style={{ width: '100%' }}>
            <div>
              <Typography.Text strong>DHCP Guard</Typography.Text>
              <Tag icon={<SafetyOutlined />} color="success" style={{ marginLeft: 8 }}>
                启用
              </Tag>
              <Typography.Paragraph type="secondary">
                通过 security.exhaustion.* 阈值限制单端租约数，联动隔离策略。
              </Typography.Paragraph>
            </div>
            <div>
              <Typography.Text strong>审计签名</Typography.Text>
              <Tag color="processing" style={{ marginLeft: 8 }}>
                Rolling
              </Tag>
              <Typography.Paragraph type="secondary">
                所有策略变更写入 auditpayload，支持对接安全体系。
              </Typography.Paragraph>
            </div>
          </Space>
        </Card>
      </Col>
      <Col span={24}>
        <Card className="glass-panel" title="策略工厂">
          <Collapse
            items={[
              {
                key: 'rbac',
                label: '租户 RBAC 模板',
                children: <Typography.Paragraph>结合 tenant service 暴露的 role catalog。</Typography.Paragraph>,
              },
              {
                key: 'quota',
                label: '资源配额',
                children: <Typography.Paragraph>等待 /api/v1/tenants/:id/quotas API 对接。</Typography.Paragraph>,
              },
            ]}
          />
        </Card>
      </Col>
    </Row>
  )
}
