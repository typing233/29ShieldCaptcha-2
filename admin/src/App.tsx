import React, { useState, useEffect } from 'react';
import { Routes, Route, Navigate, useNavigate, useLocation } from 'react-router-dom';
import { Layout, Menu, ConfigProvider, theme, Switch } from 'antd';
import {
  DashboardOutlined,
  FileSearchOutlined,
  SafetyOutlined,
  ExperimentOutlined,
  SettingOutlined,
  LogoutOutlined,
  BulbOutlined,
} from '@ant-design/icons';
import zhCN from 'antd/locale/zh_CN';
import Login from './pages/Login';
import Dashboard from './pages/Dashboard';
import Logs from './pages/Logs';
import Rules from './pages/Rules';
import Experiments from './pages/Experiments';
import Settings from './pages/Settings';

const { Sider, Content, Header } = Layout;

const App: React.FC = () => {
  const [darkMode, setDarkMode] = useState(false);
  const [collapsed, setCollapsed] = useState(false);
  const navigate = useNavigate();
  const location = useLocation();
  const token = localStorage.getItem('token');

  const menuItems = [
    { key: '/dashboard', icon: <DashboardOutlined />, label: '仪表盘' },
    { key: '/logs', icon: <FileSearchOutlined />, label: '验证日志' },
    { key: '/rules', icon: <SafetyOutlined />, label: '规则管理' },
    { key: '/experiments', icon: <ExperimentOutlined />, label: '实验管理' },
    { key: '/settings', icon: <SettingOutlined />, label: '系统设置' },
  ];

  const handleLogout = () => {
    localStorage.removeItem('token');
    navigate('/login');
  };

  if (!token && location.pathname !== '/login') {
    return <Navigate to="/login" replace />;
  }

  if (location.pathname === '/login') {
    return (
      <ConfigProvider locale={zhCN} theme={{ algorithm: darkMode ? theme.darkAlgorithm : theme.defaultAlgorithm }}>
        <Login />
      </ConfigProvider>
    );
  }

  return (
    <ConfigProvider locale={zhCN} theme={{ algorithm: darkMode ? theme.darkAlgorithm : theme.defaultAlgorithm }}>
      <Layout style={{ minHeight: '100vh' }}>
        <Sider collapsible collapsed={collapsed} onCollapse={setCollapsed}>
          <div style={{ height: 32, margin: 16, color: '#fff', textAlign: 'center', fontWeight: 'bold', fontSize: 14 }}>
            {collapsed ? 'SC' : 'ShieldCaptcha'}
          </div>
          <Menu
            theme="dark"
            mode="inline"
            selectedKeys={[location.pathname]}
            items={menuItems}
            onClick={({ key }) => navigate(key)}
          />
          <Menu
            theme="dark"
            mode="inline"
            selectable={false}
            items={[{ key: 'logout', icon: <LogoutOutlined />, label: '退出登录' }]}
            onClick={handleLogout}
            style={{ position: 'absolute', bottom: 48, width: '100%' }}
          />
        </Sider>
        <Layout>
          <Header style={{ padding: '0 24px', display: 'flex', justifyContent: 'flex-end', alignItems: 'center', background: darkMode ? '#141414' : '#fff' }}>
            <BulbOutlined style={{ marginRight: 8 }} />
            <Switch checked={darkMode} onChange={setDarkMode} checkedChildren="暗" unCheckedChildren="亮" />
          </Header>
          <Content style={{ margin: 24 }}>
            <Routes>
              <Route path="/dashboard" element={<Dashboard />} />
              <Route path="/logs" element={<Logs />} />
              <Route path="/rules" element={<Rules />} />
              <Route path="/experiments" element={<Experiments />} />
              <Route path="/settings" element={<Settings />} />
              <Route path="*" element={<Navigate to="/dashboard" replace />} />
            </Routes>
          </Content>
        </Layout>
      </Layout>
    </ConfigProvider>
  );
};

export default App;
