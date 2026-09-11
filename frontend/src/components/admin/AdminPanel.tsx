import React, { useState } from 'react';
import RegistryPanel from './RegistryPanel';
import TelemetryPanel from './TelemetryPanel';
import AccessPanel from './AccessPanel';
import ScenarioPanel from './ScenarioPanel';

const TABS = [
  { id: 'registry', label: 'Реестр КПА' },
  { id: 'telemetry', label: 'Запрос телеметрии' },
  { id: 'access', label: 'Доступ и аудит' },
  { id: 'scenario', label: 'Сценарии стенда' },
] as const;

type TabId = (typeof TABS)[number]['id'];

// AdminPanel: tabbed administration area (TZ §14.7).
const AdminPanel: React.FC = () => {
  const [tab, setTab] = useState<TabId>('registry');

  return (
    <section className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-1">
        {TABS.map((t) => (
          <button
            key={t.id}
            onClick={() => setTab(t.id)}
            className={`rounded px-3 py-1.5 text-[12px] ${
              tab === t.id
                ? 'bg-accent/10 font-semibold text-accent'
                : 'text-mute hover:bg-panel2 hover:text-ink'
            }`}
          >
            {t.label}
          </button>
        ))}
      </div>
      {tab === 'registry' && <RegistryPanel />}
      {tab === 'telemetry' && <TelemetryPanel />}
      {tab === 'access' && <AccessPanel />}
      {tab === 'scenario' && <ScenarioPanel />}
    </section>
  );
};

export default AdminPanel;
