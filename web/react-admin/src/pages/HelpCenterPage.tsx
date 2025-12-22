import { useQuery } from '@tanstack/react-query'
import { Card, Col, List, Row, Timeline, Typography } from 'antd'
import { fetchHelpCenterSnapshot } from '../services/features'
import type { HelpCenterSnapshot } from '../types/api'

function docTypeLabel(type: HelpCenterSnapshot['docs'][number]['type']) {
  switch (type) {
    case 'doc':
      return '文档'
    case 'api':
      return 'API'
    case 'faq':
      return 'FAQ'
    default:
      return '指南'
  }
}

export default function HelpCenterPage() {
  const { data, isLoading } = useQuery<HelpCenterSnapshot>({
    queryKey: ['help-center-snapshot'],
    queryFn: fetchHelpCenterSnapshot,
    staleTime: 300_000,
  })

  return (
    <Row gutter={[24, 24]}>
      <Col xs={24} md={12}>
        <Card title="在线文档" loading={isLoading} className="glass-panel">
          <List
            dataSource={data?.docs ?? []}
            renderItem={(doc) => (
              <List.Item>
                <List.Item.Meta
                  title={doc.title}
                  description={`类型：${docTypeLabel(doc.type)} · 更新：${new Date(doc.updatedAt).toLocaleString()}`}
                />
                <a href={doc.link}>查看</a>
              </List.Item>
            )}
          />
        </Card>
      </Col>
      <Col xs={24} md={12}>
        <Card title="版本信息" loading={isLoading} className="glass-panel">
          <Typography.Title level={3}>{data?.latestVersion ?? '--'}</Typography.Title>
          <Typography.Paragraph>当前开放工单：{data?.openTickets ?? 0}</Typography.Paragraph>
          <Typography.Title level={5}>最近更新</Typography.Title>
          <Timeline
            items={(data?.releaseHighlights ?? []).map((item) => ({
              children: <Typography.Text>{item}</Typography.Text>,
            }))}
          />
        </Card>
      </Col>
      <Col span={24}>
        <Card className="glass-panel">
          <Typography.Text type="secondary">
            帮助中心内容使用 mock 数据渲染，可在 `/app/help` 提供在线文档、支持工单与版本信息，保持导航完整度。
          </Typography.Text>
        </Card>
      </Col>
    </Row>
  )
}
