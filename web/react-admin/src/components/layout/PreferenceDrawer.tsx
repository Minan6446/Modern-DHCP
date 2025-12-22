import {
  Divider,
  Drawer,
  Radio,
  Select,
  Slider,
  Space,
  Switch,
  Typography,
  Button,
} from 'antd'
import type { DrawerProps } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { usePreferencesStore } from '../../store/preferences'

const localeOptions = [
  { label: '简体中文', value: 'zh-CN' },
  { label: 'English', value: 'en-US' },
  { label: '日本語', value: 'ja-JP' },
]

const themeOptions = [
  { label: '暗色', value: 'dark' },
  { label: '亮色', value: 'light' },
  { label: '跟随系统', value: 'system' },
]

type PreferenceDrawerProps = {
  open: boolean
  onClose: DrawerProps['onClose']
}

export default function PreferenceDrawer({ open, onClose }: PreferenceDrawerProps) {
  const {
    themeMode,
    locale,
    fontScale,
    highContrast,
    reduceMotion,
    setThemeMode,
    setLocale,
    setFontScale,
    setHighContrast,
    setReduceMotion,
    resetPreferences,
  } = usePreferencesStore()

  return (
    <Drawer
      title="外观与辅助功能"
      width={420}
      open={open}
      onClose={onClose}
      destroyOnClose
      extra={
        <Button type="text" icon={<ReloadOutlined />} onClick={resetPreferences}>
          恢复默认
        </Button>
      }
    >
      <Space direction="vertical" size={28} style={{ width: '100%' }}>
        <section>
          <Typography.Title level={5} style={{ marginBottom: 12 }}>
            主题模式
          </Typography.Title>
          <Radio.Group
            optionType="button"
            buttonStyle="solid"
            value={themeMode}
            options={themeOptions}
            onChange={(event) => setThemeMode(event.target.value)}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
            切换暗色/亮色模式，或让界面自动跟随系统设置。
          </Typography.Paragraph>
        </section>

        <Divider />

        <section>
          <Typography.Title level={5} style={{ marginBottom: 12 }}>
            语言
          </Typography.Title>
          <Select
            value={locale}
            options={localeOptions}
            onChange={setLocale}
            style={{ width: '100%' }}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
            立即切换界面语言，无需刷新浏览器。
          </Typography.Paragraph>
        </section>

        <Divider />

        <section>
          <Typography.Title level={5} style={{ marginBottom: 12 }}>
            字体缩放
          </Typography.Title>
          <Slider
            min={0.9}
            max={1.3}
            step={0.05}
            value={fontScale}
            marks={{ 0.9: '90%', 1: '100%', 1.3: '130%' }}
            tooltip={{ formatter: (value) => `${Math.round((value ?? 1) * 100)}%` }}
            onChange={(value) => {
              const numericValue = Array.isArray(value) ? value[0] : value
              setFontScale(numericValue)
            }}
          />
          <Typography.Paragraph type="secondary" style={{ marginTop: 12 }}>
            调整基础字号，兼顾监控大屏与高分辨率笔记本。
          </Typography.Paragraph>
        </section>

        <Divider />

        <section>
          <Typography.Title level={5} style={{ marginBottom: 12 }}>
            辅助功能
          </Typography.Title>
          <Space direction="vertical" size={16} style={{ width: '100%' }}>
            <Space align="center" style={{ justifyContent: 'space-between', width: '100%' }}>
              <div>
                <Typography.Text strong>高对比度</Typography.Text>
                <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
                  增强前景/背景对比度，突出关键控制项。
                </Typography.Paragraph>
              </div>
              <Switch checked={highContrast} onChange={setHighContrast} />
            </Space>
            <Space align="center" style={{ justifyContent: 'space-between', width: '100%' }}>
              <div>
                <Typography.Text strong>减少动画</Typography.Text>
                <Typography.Paragraph type="secondary" style={{ margin: 0 }}>
                  禁用大部分过渡与动效，更适合远程桌面或演示环境。
                </Typography.Paragraph>
              </div>
              <Switch checked={reduceMotion} onChange={setReduceMotion} />
            </Space>
          </Space>
        </section>
      </Space>
    </Drawer>
  )
}
