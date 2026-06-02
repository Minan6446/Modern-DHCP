import type { ThreatEvent, TopologyLink, TopologyNode } from '@/types/security';

export const scoreThreat = (event: ThreatEvent) => {
  let score = event.score;
  if (event.type === 'rogue-server') score += 20;
  if (event.type === 'exhaustion') score += 15;
  if (event.type === 'spoofing') score += 10;
  if (event.port?.startsWith('Gi0/1')) score += 5;
  return Math.min(100, score);
};

export const analyzeAttackPattern = (events: ThreatEvent[]) => {
  const buckets = events.reduce(
    (acc, e) => {
      acc[e.type] = (acc[e.type] || 0) + 1;
      return acc;
    },
    {} as Record<string, number>
  );
  const top = Object.entries(buckets)
    .map(([type, count]) => ({ type, count }))
    .sort((a, b) => b.count - a.count);
  return top.slice(0, 3);
};

export const buildTopologyOption = (nodes: TopologyNode[], links: TopologyLink[]) => {
  const colorMap: Record<TopologyNode['status'], string> = {
    normal: '#67C23A',
    warning: '#E6A23C',
    critical: '#F56C6C'
  };
  return {
    tooltip: {},
    legend: { data: ['core', 'distribution', 'access', 'server', 'rogue'] },
    series: [
      {
        type: 'graph',
        layout: 'force',
        roam: true,
        force: { repulsion: 120, edgeLength: 80 },
        data: nodes.map((n) => ({
          ...n,
          category: n.type,
          itemStyle: { color: colorMap[n.status] }
        })),
        links,
        categories: [
          { name: 'core' },
          { name: 'distribution' },
          { name: 'access' },
          { name: 'server' },
          { name: 'rogue' }
        ]
      }
    ]
  };
};
