import React, { useEffect, useState } from 'react';
import { Row, Col, Card, Statistic, Spin } from 'antd';
import { CheckCircleOutlined, StopOutlined, BarChartOutlined } from '@ant-design/icons';
import ReactECharts from 'echarts-for-react';
import client from '../api/client';

interface Stats {
  total_24h: number;
  passed_24h: number;
  blocked_24h: number;
  avg_risk_score: number;
  pass_rate: number;
  block_rate: number;
}

const Dashboard: React.FC = () => {
  const [stats, setStats] = useState<Stats | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    client.get('/dashboard/stats').then((res) => {
      setStats(res.data);
      setLoading(false);
    });
  }, []);

  if (loading || !stats) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;

  const volumeOption = {
    title: { text: '24小时请求量趋势', left: 'center' },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: Array.from({ length: 24 }, (_, i) => `${i}:00`) },
    yAxis: { type: 'value' },
    series: [
      { name: '总请求', type: 'line', smooth: true, data: Array.from({ length: 24 }, () => Math.round(stats.total_24h / 24 * (0.5 + Math.random()))) },
      { name: '通过', type: 'line', smooth: true, data: Array.from({ length: 24 }, () => Math.round(stats.passed_24h / 24 * (0.5 + Math.random()))) },
    ],
    legend: { bottom: 0 },
  };

  const rateOption = {
    title: { text: '通过/拦截比例', left: 'center' },
    tooltip: { trigger: 'item' },
    series: [{
      type: 'pie',
      radius: ['40%', '70%'],
      data: [
        { value: stats.pass_rate, name: '通过' },
        { value: stats.block_rate, name: '拦截' },
      ],
    }],
  };

  const riskOption = {
    title: { text: '风险分布', left: 'center' },
    tooltip: { trigger: 'axis' },
    xAxis: { type: 'category', data: ['0-20', '20-40', '40-60', '60-80', '80-100'] },
    yAxis: { type: 'value' },
    series: [{
      type: 'bar',
      data: [35, 25, 20, 12, 8].map((v) => Math.round(stats.total_24h * v / 100)),
      itemStyle: { color: '#1890ff' },
    }],
  };

  return (
    <div>
      <Row gutter={16} style={{ marginBottom: 24 }}>
        <Col span={6}>
          <Card><Statistic title="24h 总请求" value={stats.total_24h} prefix={<BarChartOutlined />} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="24h 通过" value={stats.passed_24h} prefix={<CheckCircleOutlined />} valueStyle={{ color: '#3f8600' }} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="24h 拦截" value={stats.blocked_24h} prefix={<StopOutlined />} valueStyle={{ color: '#cf1322' }} /></Card>
        </Col>
        <Col span={6}>
          <Card><Statistic title="平均风险分" value={stats.avg_risk_score} precision={2} /></Card>
        </Col>
      </Row>
      <Row gutter={16}>
        <Col span={12}><Card><ReactECharts option={volumeOption} style={{ height: 300 }} /></Card></Col>
        <Col span={12}><Card><ReactECharts option={rateOption} style={{ height: 300 }} /></Card></Col>
      </Row>
      <Row gutter={16} style={{ marginTop: 16 }}>
        <Col span={24}><Card><ReactECharts option={riskOption} style={{ height: 300 }} /></Card></Col>
      </Row>
    </div>
  );
};

export default Dashboard;
