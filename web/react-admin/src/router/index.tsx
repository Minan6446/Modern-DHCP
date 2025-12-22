import { type ReactElement, type ReactNode } from 'react'
import {
  AlertOutlined,
  ApiOutlined,
  AppstoreOutlined,
  BranchesOutlined,
  ClusterOutlined,
  DeploymentUnitOutlined,
  LinkOutlined,
  QuestionCircleOutlined,
  RadarChartOutlined,
  SafetyCertificateOutlined,
  SecurityScanOutlined,
  SettingOutlined,
  TableOutlined,
  ThunderboltOutlined,
  ToolOutlined,
} from '@ant-design/icons'
import { createBrowserRouter, Navigate } from 'react-router-dom'
import AppLayout from '../components/layout/AppLayout'
import CapabilityBoundary from '../components/layout/CapabilityBoundary'
import { ProtectedRoute } from '../components/layout/ProtectedRoute'
import AutomationPage from '../pages/AutomationPage'
import ClusterPage from '../pages/ClusterPage'
import DashboardPage from '../pages/DashboardPage'
import DhcpOptionsPage from '../pages/DhcpOptionsPage'
import HelpCenterPage from '../pages/HelpCenterPage'
import IntegrationsPage from '../pages/IntegrationsPage'
import LeasesPage from '../pages/LeasesPage'
import LoginPage from '../pages/LoginPage'
import MaintenancePage from '../pages/MaintenancePage'
import MonitoringPage from '../pages/MonitoringPage'
import NotFoundPage from '../pages/NotFoundPage'
import PoolsPage from '../pages/PoolsPage'
import PolicyPage from '../pages/PolicyPage'
import SecurityPage from '../pages/SecurityPage'
import SettingsPage from '../pages/SettingsPage'
import StaticBindingsPage from '../pages/StaticBindingsPage'
import SystemManagementPage from '../pages/SystemManagementPage'

export type NavigationItem = {
  key: string
  label: string
  path: string
  icon: ReactNode
  element: ReactElement
  description: string
  capability?: string
}

export const navigationItems: NavigationItem[] = [
  {
    key: 'dashboard',
    label: '全局总览',
    path: 'dashboard',
    icon: <AppstoreOutlined />,
    description: '系统健康、事件时间线与运行指标',
    element: <DashboardPage />,
    capability: 'view:dashboard',
  },
  {
    key: 'system',
    label: '系统管理',
    path: 'system',
    icon: <SafetyCertificateOutlined />,
    description: '认证、租户隔离、RBAC 与 API 密钥',
    element: <SystemManagementPage />,
    capability: 'manage:system',
  },
  {
    key: 'pools',
    label: '地址池',
    path: 'pools',
    icon: <ClusterOutlined />,
    description: '分层地址池利用率、容量与拓扑',
    element: <PoolsPage />,
    capability: 'view:pools',
  },
  {
    key: 'leases',
    label: '租约视图',
    path: 'leases',
    icon: <TableOutlined />,
    description: 'IPv4/IPv6 租约搜索、隔离与冷却处理',
    element: <LeasesPage />,
    capability: 'view:leases',
  },
  {
    key: 'static-bindings',
    label: '静态绑定',
    path: 'static-bindings',
    icon: <LinkOutlined />,
    description: 'MAC/客户端标识绑定与批量导入',
    element: <StaticBindingsPage />,
    capability: 'manage:static-bindings',
  },
  {
    key: 'dhcp-options',
    label: 'DHCP 选项',
    path: 'dhcp-options',
    icon: <ApiOutlined />,
    description: '标准/自定义选项库与模板',
    element: <DhcpOptionsPage />,
    capability: 'manage:dhcp-options',
  },
  {
    key: 'automation',
    label: '自动化',
    path: 'automation',
    icon: <ThunderboltOutlined />,
    description: '计划任务、一次性作业与审批流',
    element: <AutomationPage />,
    capability: 'view:automation',
  },
  {
    key: 'monitoring',
    label: '监控与告警',
    path: 'monitoring',
    icon: <AlertOutlined />,
    description: '统一告警流、抑制规则与处理动作',
    element: <MonitoringPage />,
    capability: 'view:monitoring',
  },
  {
    key: 'policy',
    label: '策略工作台',
    path: 'policy',
    icon: <RadarChartOutlined />,
    description: 'RBAC、租户配额与策略编排',
    element: <PolicyPage />,
    capability: 'manage:policy',
  },
  {
    key: 'cluster',
    label: '服务器集群',
    path: 'cluster',
    icon: <DeploymentUnitOutlined />,
    description: '节点管理、HA 状态与同步健康',
    element: <ClusterPage />,
    capability: 'view:cluster',
  },
  {
    key: 'security',
    label: '安全与防护',
    path: 'security',
    icon: <SecurityScanOutlined />,
    description: 'Snooping、限速、威胁告警概览',
    element: <SecurityPage />,
    capability: 'view:security',
  },
  {
    key: 'integrations',
    label: 'API 与集成',
    path: 'integrations',
    icon: <BranchesOutlined />,
    description: 'Webhook、CMDB、ITSM 与监控集成',
    element: <IntegrationsPage />,
    capability: 'manage:integrations',
  },
  {
    key: 'maintenance',
    label: '系统维护',
    path: 'maintenance',
    icon: <ToolOutlined />,
    description: '备份、升级计划与容量预测',
    element: <MaintenancePage />,
    capability: 'view:maintenance',
  },
  {
    key: 'settings',
    label: '系统设置',
    path: 'settings',
    icon: <SettingOutlined />,
    description: '集群、审计、外部集成配置',
    element: <SettingsPage />,
    capability: 'manage:settings',
  },
  {
    key: 'help',
    label: '帮助与支持',
    path: 'help',
    icon: <QuestionCircleOutlined />,
    description: '文档、版本信息与技术支持',
    element: <HelpCenterPage />,
    capability: 'view:help',
  },
]

export const router = createBrowserRouter([
  { path: '/', element: <Navigate to="/app/dashboard" replace /> },
  { path: '/auth/login', element: <LoginPage /> },
  {
    path: '/app',
    element: (
      <ProtectedRoute>
        <AppLayout />
      </ProtectedRoute>
    ),
    children: navigationItems.map((item) => ({
      path: item.path,
      element: (
        <CapabilityBoundary capability={item.capability}>{item.element}</CapabilityBoundary>
      ),
    })),
  },
  { path: '*', element: <NotFoundPage /> },
])
