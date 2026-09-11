import React from 'react';
import { AuthProvider } from './auth/AuthProvider';
import { WsProvider } from './ws/WsProvider';
import { RegistryProvider } from './registry/RegistryProvider';
import Dashboard from './components/Dashboard';

const App: React.FC = () => (
  <AuthProvider>
    <WsProvider>
      <RegistryProvider>
        <Dashboard />
      </RegistryProvider>
    </WsProvider>
  </AuthProvider>
);

export default App;
