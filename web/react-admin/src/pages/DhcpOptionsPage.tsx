import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Row, Statistic, Tag, Typography } from 'antd'
import { fetchDhcpOptionCatalog } from '../services/features'
import type { DhcpOptionCatalog } from '../types/api'

export default function DhcpOptionsPage() {
  const { data, isLoading } = useQuery<DhcpOptionCatalog>({
    queryKey: ['dhcp-option-catalog'],
    queryFn: fetchDhcpOptionCatalog,
    staleTime: 120_000,
  })

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} lg={12}>
        <Card title="标准选项库" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.standard ?? []}
            renderItem={(item) => (
              <List.Item>
                <List.Item.Meta title={`${item.name}`} description={`Option ${item.code} · ${item.category}`} />
                <Tag color="blue">引用 {item.usageCount ?? 0}</Tag>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col xs={24} lg={12}>
        <Card title="自定义选项" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.custom ?? []}
            renderItem={(item) => (
              <List.Item>
                <List.Item.Meta title={`${item.name}`} description={`Option ${item.code} · ${item.description}`} />
                <Tag color="purple">{item.category}</Tag>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card title="模板系统" loading={isLoading} className="glass-panel">
          <List
            grid={{ gutter: 16, xs: 1, md: 3 }}
            dataSource={data?.templates ?? []}
            renderItem={(template) => (
              <List.Item>
                <Card bordered={false} style={{ background: 'rgba(248,249,255,0.02)' }}>
                  <Typography.Title level={5}>{template.name}</Typography.Title>
                  <Typography.Paragraph type="secondary">{template.description}</Typography.Paragraph>
                  <Statistic title="选项数量" value={template.options} />
                  <Statistic title="引用地址池" value={template.usedBy} />
                </Card>
              </List.Item>
            )}
          />
        </Card>
      </Col>
    </Row>
  )
}
