import React, { useEffect, useState } from 'react';
import { Card, Table, Button, Modal, Form, Input, InputNumber, Select, Space, message, Tag, Popconfirm } from 'antd';
import { PlusOutlined } from '@ant-design/icons';
import client from '../api/client';

interface Experiment {
  id: string;
  name: string;
  description: string;
  traffic_pct: number;
  config_a: any;
  config_b: any;
  status: string;
}

const Experiments: React.FC = () => {
  const [experiments, setExperiments] = useState<Experiment[]>([]);
  const [loading, setLoading] = useState(false);
  const [modal, setModal] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form] = Form.useForm();

  const fetchExperiments = async () => {
    setLoading(true);
    const res = await client.get('/experiments');
    setExperiments(res.data || []);
    setLoading(false);
  };

  useEffect(() => { fetchExperiments(); }, []);

  const handleSave = async () => {
    const values = await form.validateFields();
    const payload = {
      ...values,
      config_a: JSON.parse(values.config_a || '{}'),
      config_b: JSON.parse(values.config_b || '{}'),
    };
    if (editId) {
      await client.put(`/experiments/${editId}`, payload);
      message.success('实验已更新');
    } else {
      await client.post('/experiments', payload);
      message.success('实验已创建');
    }
    setModal(false);
    form.resetFields();
    setEditId(null);
    fetchExperiments();
  };

  const statusColors: Record<string, string> = { running: 'green', paused: 'orange', stopped: 'red', draft: 'default' };

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: '流量比例', dataIndex: 'traffic_pct', key: 'traffic_pct', width: 100, render: (v: number) => `${v}%` },
    { title: '状态', dataIndex: 'status', key: 'status', width: 80, render: (v: string) => <Tag color={statusColors[v] || 'default'}>{v}</Tag> },
    {
      title: '操作', key: 'actions', width: 120,
      render: (_: any, record: Experiment) => (
        <Space>
          <a onClick={() => {
            setEditId(record.id);
            form.setFieldsValue({
              ...record,
              config_a: JSON.stringify(record.config_a, null, 2),
              config_b: JSON.stringify(record.config_b, null, 2),
            });
            setModal(true);
          }}>编辑</a>
        </Space>
      ),
    },
  ];

  return (
    <Card title="A/B 实验管理" extra={<Button type="primary" icon={<PlusOutlined />} onClick={() => { setEditId(null); form.resetFields(); setModal(true); }}>新建实验</Button>}>
      <Table rowKey="id" columns={columns} dataSource={experiments} loading={loading} pagination={false} />
      <Modal title={editId ? '编辑实验' : '新建实验'} open={modal} onOk={handleSave} onCancel={() => setModal(false)} width={600} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="实验名称" rules={[{ required: true, message: '请输入实验名称' }]}><Input /></Form.Item>
          <Form.Item name="description" label="描述"><Input.TextArea rows={2} /></Form.Item>
          <Form.Item name="traffic_pct" label="流量比例(%)" rules={[{ required: true }]}><InputNumber min={0} max={100} style={{ width: '100%' }} /></Form.Item>
          {editId && (
            <Form.Item name="status" label="状态">
              <Select>
                <Select.Option value="draft">草稿</Select.Option>
                <Select.Option value="running">运行中</Select.Option>
                <Select.Option value="paused">暂停</Select.Option>
                <Select.Option value="stopped">已停止</Select.Option>
              </Select>
            </Form.Item>
          )}
          <Form.Item name="config_a" label="配置 A (JSON)" rules={[{ required: true, message: '请输入配置A' }]}>
            <Input.TextArea rows={4} placeholder='{"difficulty": 3, "type": "slider"}' />
          </Form.Item>
          <Form.Item name="config_b" label="配置 B (JSON)" rules={[{ required: true, message: '请输入配置B' }]}>
            <Input.TextArea rows={4} placeholder='{"difficulty": 5, "type": "puzzle"}' />
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default Experiments;
