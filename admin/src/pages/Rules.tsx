import React, { useEffect, useState } from 'react';
import { Card, Table, Button, Modal, Form, Input, Select, InputNumber, Space, message, Popconfirm, Tag } from 'antd';
import { PlusOutlined, ReloadOutlined } from '@ant-design/icons';
import client from '../api/client';

interface Rule {
  id: string;
  name: string;
  description: string;
  condition_json: any;
  action: string;
  weight: number;
}

interface ConditionNode {
  type: 'AND' | 'OR' | 'NOT' | 'LEAF';
  field?: string;
  operator?: string;
  value?: string;
  children?: ConditionNode[];
}

const ConditionBuilder: React.FC<{ value?: ConditionNode; onChange?: (v: ConditionNode) => void }> = ({ value, onChange }) => {
  const node = value || { type: 'LEAF', field: '', operator: 'eq', value: '' };

  if (node.type === 'LEAF') {
    return (
      <Space>
        <Input placeholder="字段" value={node.field} onChange={(e) => onChange?.({ ...node, field: e.target.value })} style={{ width: 120 }} />
        <Select value={node.operator} onChange={(v) => onChange?.({ ...node, operator: v })} style={{ width: 100 }}>
          <Select.Option value="eq">等于</Select.Option>
          <Select.Option value="gt">大于</Select.Option>
          <Select.Option value="lt">小于</Select.Option>
          <Select.Option value="contains">包含</Select.Option>
          <Select.Option value="regex">正则</Select.Option>
        </Select>
        <Input placeholder="值" value={node.value} onChange={(e) => onChange?.({ ...node, value: e.target.value })} style={{ width: 140 }} />
        <Select value={node.type} onChange={(v) => onChange?.({ type: v as any, children: v === 'NOT' ? [node] : [node, { type: 'LEAF', field: '', operator: 'eq', value: '' }] })} style={{ width: 90 }}>
          <Select.Option value="LEAF">条件</Select.Option>
          <Select.Option value="AND">AND</Select.Option>
          <Select.Option value="OR">OR</Select.Option>
          <Select.Option value="NOT">NOT</Select.Option>
        </Select>
      </Space>
    );
  }

  return (
    <Card size="small" style={{ marginBottom: 8 }}>
      <Tag color="blue">{node.type}</Tag>
      <Select value={node.type} onChange={(v) => onChange?.({ ...node, type: v as any })} size="small" style={{ width: 80, marginBottom: 8 }}>
        <Select.Option value="AND">AND</Select.Option>
        <Select.Option value="OR">OR</Select.Option>
        <Select.Option value="NOT">NOT</Select.Option>
      </Select>
      {(node.children || []).map((child, i) => (
        <div key={i} style={{ marginLeft: 16, marginBottom: 4 }}>
          <ConditionBuilder value={child} onChange={(v) => {
            const children = [...(node.children || [])];
            children[i] = v;
            onChange?.({ ...node, children });
          }} />
        </div>
      ))}
      {node.type !== 'NOT' && (
        <Button size="small" type="dashed" onClick={() => onChange?.({ ...node, children: [...(node.children || []), { type: 'LEAF', field: '', operator: 'eq', value: '' }] })}>
          + 添加条件
        </Button>
      )}
    </Card>
  );
};

const Rules: React.FC = () => {
  const [rules, setRules] = useState<Rule[]>([]);
  const [loading, setLoading] = useState(false);
  const [modal, setModal] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);
  const [form] = Form.useForm();

  const fetchRules = async () => {
    setLoading(true);
    const res = await client.get('/rules');
    setRules(res.data || []);
    setLoading(false);
  };

  useEffect(() => { fetchRules(); }, []);

  const handleSave = async () => {
    const values = await form.validateFields();
    if (editId) {
      await client.put(`/rules/${editId}`, values);
      message.success('规则已更新');
    } else {
      await client.post('/rules', values);
      message.success('规则已创建');
    }
    setModal(false);
    form.resetFields();
    setEditId(null);
    fetchRules();
  };

  const handleDelete = async (id: string) => {
    await client.delete(`/rules/${id}`);
    message.success('规则已删除');
    fetchRules();
  };

  const handleReload = async () => {
    await client.post('/rules/reload');
    message.success('规则已重载');
  };

  const columns = [
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '描述', dataIndex: 'description', key: 'description', ellipsis: true },
    { title: '动作', dataIndex: 'action', key: 'action', width: 80 },
    { title: '权重', dataIndex: 'weight', key: 'weight', width: 80 },
    {
      title: '操作', key: 'actions', width: 150,
      render: (_: any, record: Rule) => (
        <Space>
          <a onClick={() => { setEditId(record.id); form.setFieldsValue({ ...record, condition_json: record.condition_json }); setModal(true); }}>编辑</a>
          <Popconfirm title="确认删除?" onConfirm={() => handleDelete(record.id)}><a style={{ color: 'red' }}>删除</a></Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <Card title="规则管理" extra={<Space><Button icon={<ReloadOutlined />} onClick={handleReload}>重载规则</Button><Button type="primary" icon={<PlusOutlined />} onClick={() => { setEditId(null); form.resetFields(); setModal(true); }}>新建规则</Button></Space>}>
      <Table rowKey="id" columns={columns} dataSource={rules} loading={loading} pagination={false} />
      <Modal title={editId ? '编辑规则' : '新建规则'} open={modal} onOk={handleSave} onCancel={() => setModal(false)} width={700} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '请输入规则名称' }]}><Input /></Form.Item>
          <Form.Item name="description" label="描述"><Input.TextArea rows={2} /></Form.Item>
          <Form.Item name="condition_json" label="条件"><ConditionBuilder /></Form.Item>
          <Form.Item name="action" label="动作" rules={[{ required: true }]}>
            <Select><Select.Option value="block">拦截</Select.Option><Select.Option value="challenge">挑战</Select.Option><Select.Option value="log">仅记录</Select.Option></Select>
          </Form.Item>
          <Form.Item name="weight" label="权重"><InputNumber min={0} max={100} /></Form.Item>
        </Form>
      </Modal>
    </Card>
  );
};

export default Rules;
