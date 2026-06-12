import React, { useEffect, useState } from 'react';
import { Card, Form, Slider, Button, message, Divider, ColorPicker, Select, Space, Input, Switch, Spin, Row, Col } from 'antd';
import type { Color } from 'antd/es/color-picker';
import client from '../api/client';

const Settings: React.FC = () => {
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [form] = Form.useForm();
  const [styleForm] = Form.useForm();
  const [unblockForm] = Form.useForm();

  useEffect(() => {
    client.get('/config/difficulty').then((res) => {
      form.setFieldsValue(res.data);
      setLoading(false);
    });
  }, []);

  const saveDifficulty = async () => {
    const values = await form.validateFields();
    setSaving(true);
    try {
      await client.put('/config/difficulty', values);
      message.success('难度设置已保存');
    } finally {
      setSaving(false);
    }
  };

  const handleUnblock = async () => {
    const values = await unblockForm.validateFields();
    await client.post('/unblock', values);
    message.success('解封成功');
    unblockForm.resetFields();
  };

  if (loading) return <Spin size="large" style={{ display: 'block', margin: '100px auto' }} />;

  return (
    <div>
      <Row gutter={16}>
        <Col span={12}>
          <Card title="难度设置">
            <Form form={form} layout="vertical">
              <Form.Item name="base" label="基础难度">
                <Slider min={1} max={10} marks={{ 1: '1', 5: '5', 10: '10' }} />
              </Form.Item>
              <Form.Item name="min" label="最低难度">
                <Slider min={1} max={10} marks={{ 1: '1', 5: '5', 10: '10' }} />
              </Form.Item>
              <Form.Item name="max" label="最高难度">
                <Slider min={1} max={10} marks={{ 1: '1', 5: '5', 10: '10' }} />
              </Form.Item>
              <Button type="primary" onClick={saveDifficulty} loading={saving}>保存</Button>
            </Form>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="外观自定义">
            <Form form={styleForm} layout="vertical" initialValues={{ primaryColor: '#1890ff', shape: 'slider', borderRadius: 8 }}>
              <Form.Item name="primaryColor" label="主色调">
                <ColorPicker showText />
              </Form.Item>
              <Form.Item name="shape" label="滑块形状">
                <Select>
                  <Select.Option value="slider">滑动验证</Select.Option>
                  <Select.Option value="puzzle">拼图验证</Select.Option>
                  <Select.Option value="click">点击验证</Select.Option>
                  <Select.Option value="rotate">旋转验证</Select.Option>
                </Select>
              </Form.Item>
              <Form.Item name="borderRadius" label="圆角大小">
                <Slider min={0} max={24} />
              </Form.Item>
              <Button type="primary" onClick={() => { message.success('样式已保存'); }}>保存样式</Button>
            </Form>
          </Card>
        </Col>
      </Row>

      <Divider />

      <Row gutter={16}>
        <Col span={12}>
          <Card title="解封操作">
            <Form form={unblockForm} layout="vertical">
              <Form.Item name="ip" label="IP 地址"><Input placeholder="可选" /></Form.Item>
              <Form.Item name="fingerprint" label="指纹"><Input placeholder="可选" /></Form.Item>
              <Button type="primary" danger onClick={handleUnblock}>执行解封</Button>
            </Form>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="功能开关">
            <Form layout="vertical" initialValues={{ enableRateLimit: true, enableFingerprint: true, enableBehavior: true }}>
              <Form.Item name="enableRateLimit" label="频率限制" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="enableFingerprint" label="指纹检测" valuePropName="checked"><Switch /></Form.Item>
              <Form.Item name="enableBehavior" label="行为分析" valuePropName="checked"><Switch /></Form.Item>
            </Form>
          </Card>
        </Col>
      </Row>
    </div>
  );
};

export default Settings;
