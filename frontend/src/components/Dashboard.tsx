import React, { useEffect, useMemo, useState } from 'react';
import { TelemetryReading, useWebSocket } from '../hooks/useWebSocket';
import { useRegistry } from '../hooks/useRegistry';
import StatusCard from '../components/StatusCard';
import LiveChart from '../components/LiveChart';
import AlertModal from '../components/AlertModal';
import InstructorPanel from '../components/InstructorPanel';
import AlertLog from '../components/AlertLog';
import AdminPanel from '../components/AdminPanel';
import AlarmPanel from '../components/AlarmPanel';
import ProfilesPanel from '../components/ProfilesPanel';
import AnalyticsPanel from '../components/AnalyticsPanel';

type Page = 'overview' | 'analytics' | 'alarms' | 'profiles' | 'settings' | 'instructor';

const PAGES: { id: Page; label: string; icon: JSX.Element }[] = [
  {
    id: 'overview', label: 'Обзор',
    icon: <path d="M3 13h8V3H3v10zm10 8h8V11h-8v10zM3 21h8v-6H3v6zm10-12h8V3h-8v6z" />,
  },
  {
    id: 'analytics', label: 'Аналитика',
    icon: <path d="M4 20h16v1.5H4V20zM6 16h2.5v3H6v-3zm4.75-6h2.5v9h-2.5v-9zM15.5 4H18v15h-2.5V4z" />,
  },
  {
    id: 'alarms', label: 'Аварии',
    icon: <path d="M12 2a6 6 0 0 0-6 6v4l-2 4h16l-2-4V8a6 6 0 0 0-6-6zm-2 16a2 2 0 0 0 4 0h-4z" />,
  },
  {
    id: 'profiles', label: 'Профили',
    icon: <path d="M12 2 2 7l10 5 10-5-10-5zm0 9L2 6v11l10 5 10-5V6l-10 5z" />,
  },
  {
    id: 'settings', label: 'Настройки',
    icon: <path d="M12 8a4 4 0 1 0 0 8 4 4 0 0 0 0-8zm9 4a7.6 7.6 0 0 0-.1-1.2l2-1.6-2-3.4-2.4 1a7.7 7.7 0 0 0-2-1.2L16 3h-4l-.5 2.6a7.7 7.7 0 0 0-2 1.2l-2.4-1-2 3.4 2 1.6a7.6 7.6 0 0 0 0 2.4l-2 1.6 2 3.4 2.4-1a7.7 7.7 0 0 0 2 1.2L8 21h4l.5-2.6a7.7 7.7 0 0 0 2-1.2l2.4 1 2-3.4-2-1.6c.06-.4.1-.8.1-1.2z" />,
  },
  {
    id: 'instructor', label: 'Инструктор',
    icon: <path d="M8 5v14l11-7L8 5z" />,
  },
];

const readPage = (): Page => {
  const hash = window.location.hash.replace('#', '') as Page;
  return PAGES.some((p) => p.id === hash) ? hash : 'overview';
};

const Dashboard: React.FC = () => {
  const { readings, emergency, connected, alerts, sendEmergency, stopEmergency } = useWebSocket();
  const { stageOrder, gateways, loading, error, reload } = useRegistry();
  const [page, setPage] = useState<Page>(readPage);

  useEffect(() => {
    const onHash = () => setPage(readPage());
    window.addEventListener('hashchange', onHash);
    return () => window.removeEventListener('hashchange', onHash);
  }, []);

  const go = (p: Page) => {
    window.location.hash = p;
    setPage(p);
  };

  // Group live readings by stage (area), driven by the registry, not by code.
  const stageData = useMemo(() => {
    const map = new Map<string, TelemetryReading[]>();
    for (const { area } of stageOrder) {
      map.set(area, []);
    }
    readings.forEach((r) => {
      const bucket = map.get(r.stage);
      if (bucket) bucket.push(r);
    });
    return map;
  }, [readings, stageOrder]);

  const isEmergencyActive = emergency?.type === 'emergency_start';
  const activeLabel = PAGES.find((p) => p.id === page)?.label ?? 'Обзор';

  return (
    <div className="flex min-h-screen bg-base text-ink">
      <AlertModal alerts={alerts} />

      {/* Sidebar */}
      <aside className="sticky top-0 flex h-screen w-44 shrink-0 flex-col border-r border-line bg-panel/60">
        <button
          onClick={() => go('overview')}
          className="flex items-center gap-2.5 border-b border-line px-4 py-3.5 text-left"
        >
          <span className="flex h-6 w-6 items-center justify-center rounded bg-accent/10">
            <svg width="14" height="14" viewBox="0 0 32 32" fill="none" aria-hidden>
              <path d="M4 22 L11 22 L14 8 L18 26 L21 14 L28 14" stroke="#38bdf8" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
            </svg>
          </span>
          <span className="text-[13px] font-semibold tracking-[0.06em]">CAP</span>
        </button>

        <nav className="flex-1 space-y-0.5 overflow-y-auto p-2">
          {PAGES.map((p) => (
            <button
              key={p.id}
              onClick={() => go(p.id)}
              className={`flex w-full items-center gap-2.5 rounded px-3 py-2 text-[12.5px] transition-colors ${
                page === p.id
                  ? 'bg-accent/10 font-semibold text-accent'
                  : 'text-mute hover:bg-panel2/70 hover:text-ink'
              }`}
            >
              <svg width="13" height="13" viewBox="0 0 24 24" fill="currentColor" aria-hidden>{p.icon}</svg>
              {p.label}
            </button>
          ))}
        </nav>

        <div className="border-t border-line px-4 py-3 text-[10px] text-dim">
          <span className="flex items-center gap-1.5">
            <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-400' : 'bg-red-500 animate-blink-soft'}`} />
            {connected ? 'канал данных активен' : 'переподключение…'}
          </span>
        </div>
      </aside>

      {/* Content */}
      <div className="flex min-w-0 flex-1 flex-col">
        {/* Top strip: page title + live links */}
        <header className="sticky top-0 z-40 flex h-12 items-center gap-4 border-b border-line bg-base/85 px-5 backdrop-blur">
          <h1 className="text-[13px] font-semibold">{activeLabel}</h1>
          {isEmergencyActive && (
            <span className="flex items-center gap-1.5 rounded border border-red-500/40 bg-red-950/40 px-2 py-0.5 text-[10px] font-semibold text-red-300">
              <span className="h-1.5 w-1.5 rounded-full bg-red-500 animate-blink-soft" />
              АВАРИЙНЫЙ РЕЖИМ
            </span>
          )}
          <div className="ml-auto flex items-center gap-3 text-[10.5px]">
            {gateways.map((g) => (
              <span key={g.id} className="flex items-center gap-1.5 text-mute" title={`буфер: ${g.buffer_size}`}>
                <span className={`h-1.5 w-1.5 rounded-full ${g.status === 'online' ? 'bg-emerald-400' : 'bg-red-500 animate-blink-soft'}`} />
                <span className="font-mono">{g.id}</span>
                {g.buffer_size > 0 && <span className="text-amber-400">+{g.buffer_size}</span>}
              </span>
            ))}
            {!loading && !error && gateways.length === 0 && <span className="text-dim">шлюзы не на пульсе</span>}
            {loading ? (
              <span className="text-dim">реестр…</span>
            ) : error ? (
              <button onClick={reload} className="text-red-400 underline decoration-dotted">реестр: ошибка</button>
            ) : (
              <span className="hidden text-dim lg:inline">реестр {stageOrder.length} уч.</span>
            )}
          </div>
        </header>

        <main className="mx-auto w-full max-w-[1280px] flex-1 space-y-3 px-5 py-4">
          {page === 'overview' && (
            <>
              <section className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-4">
                {stageOrder.map(({ area, label }) => {
                  const stageReadings = stageData.get(area) || [];
                  const hasAlert = stageReadings.some((r) => r.alert);
                  const isEmergencyStage =
                    isEmergencyActive &&
                    (area === 'drying_dewatering' || area === 'flotation');
                  return (
                    <StatusCard
                      key={area}
                      stageKey={area}
                      label={label}
                      readings={stageReadings}
                      hasAlert={hasAlert}
                      isEmergency={isEmergencyStage}
                    />
                  );
                })}
              </section>
              <LiveChart readings={readings} />
            </>
          )}

          {page === 'analytics' && <AnalyticsPanel />}

          {page === 'alarms' && (
            <section className="grid grid-cols-1 gap-3 lg:grid-cols-3">
              <div className="lg:col-span-2"><AlarmPanel /></div>
              <AlertLog alerts={alerts} />
            </section>
          )}

          {page === 'profiles' && <ProfilesPanel />}

          {page === 'settings' && <AdminPanel />}

          {page === 'instructor' && (
            <div className="max-w-md">
              <InstructorPanel
                connected={connected}
                emergencyActive={isEmergencyActive}
                onTriggerEmergency={sendEmergency}
                onStopEmergency={stopEmergency}
              />
            </div>
          )}
        </main>

        <footer className="border-t border-line px-5 py-2.5 text-[10px] text-dim">
          CAP · Complex Automation Pro — MVP
        </footer>
      </div>
    </div>
  );
};

export default Dashboard;
