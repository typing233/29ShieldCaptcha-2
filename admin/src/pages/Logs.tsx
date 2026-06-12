import React, { useEffect, useState } from 'react';
import { Table, Input, Select, Card, Space, Tag, DatePicker, Button, Modal, Descriptions } from 'antd';
import { SearchOutlined } from '@ant-design/icons';
import dayjs from 'dayjs';
import client from '../api/client';

const { RangePicker } = DatePicker;

interface LogItem {
  id: string;
  ip: string;
  fingerprint: string;
  result: string;
  risk_score: number;
  created_at: string;
}

const Logs: React.FC = () => {
  const [logs, setLogs] = useState<LogItem[]>([]);
  const [total, setTotal] = useState(0);
  const [loading, setLoading] = useState(false);
  const [page, setPage] = useState(1);
  const [filters, setFilters] = useState({ ip: '', fingerprint: '', result: '' });
  const [detail, setDetail] = useState<any>(null);

  const fetchLogs = async (p = page) => {
    setLoading(true);
    try {
      const res = await client.get('/logs', { params: { ...filters, page: p, limit: 20 } });
      setLogs(res.data.logs || []);
      setTotal(res.data.total || 0);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { fetchLogs(); }, [page]);

  const showDetail = async (id: string) => {
    const res = await client.get(`/logs/${id}`);
    setDetail(res.data);
  };

  const columns = [
    { title: 'IP', dataIndex: 'ip', key: 'ip', width: 140 },
    { title: '指纹', dataIndex: 'fingerprint', key: 'fingerprint', width: 180, ellipsis: true },
    {
      title: '结果', dataIndex: 'result', key: 'result', width: 80,
      render: (v: string) => <Tag color={v === 'pass' ? 'green' : 'red'}>{v === 'pass' ? '通过' : '拦截'}</Tag>,
    },
    { title: '风险分', dataIndex: 'risk_score', key: 'risk_score', width: 80 },
    {
      title: '时间', dataIndex: 'created_at', key: 'created_at', width: 180,
      render: (v: string) => dayjs(v).format('YYYY-MM-DD HH:mm:ss'),
    },
    {
      title: '操作', key: 'action', width: 80,
      render: (_: any, record: LogItem) => <a onClick={() => showDetail(record.id)}>详情</a>,
    },
  ];

  return (
    <Card title="验证日志">
      <Space style={{ marginBottom: 16 }} wrap>
        <Input placeholder="IP 地址" value={filters.ip} onChange={(e) => setFilters({ ...filters, ip: e.target.value })} style={{ width: 160 }} />
        <Input placeholder="指纹" value={filters.fingerprint} onChange={(e) => setFilters({ ...filters, fingerprint: e.target.value })} style={{ width: 200 }} />
        <Select placeholder="结果" allowClear style={{ width: 100 }} value={filters.result || undefined} onChange={(v) => setFilters({ ...filters, result: v || '' })}>
          <Select.Option value="pass">通过</Select.Option>
          <Select.Option value="block">拦截</Select.Option>
        </Select>
        <Button type="primary" icon={<SearchOutlined />} onClick={() => { setPage(1); fetchLogs(1); }}>搜索</Button>
      </Space>
      <Table
        rowKey="id"
        columns={columns}
        dataSource={logs}
        loading={loading}
        pagination={{ current: page, total, pageSize: 20, onChange: setPage }}
      />
      <Modal title="日志详情" open={!!detail} onCancel={() => setDetail(null)} footer={null} width={600}>
        {detail && (
          <Descriptions column={1} bordered size="small">
            <Descriptions.Item label="ID">{detail.id}</Descriptions.Item>
            <Descriptions.Item label="IP">{detail.ip}</Descriptions.Item>
            <Descriptions.Item label="指纹">{detail.fingerprint}</Descriptions.Item>
            <Descriptions.Item label="结果">{detail.result}</Descriptions.Item>
            <Descriptions.Item label="风险分">{detail.risk_score}</Descriptions.Item>
            <Descriptions.Item label="行为数据">{JSON.stringify(detail.behavior_data, null, 2)}</Descriptions.Item>
            <Descriptions.Item label="时间">{dayjs(detail.created_at).format('YYYY-MM-DD HH:mm:ss')}</Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </Card>
  );
};

export default Logs;
