import React, { useMemo } from 'react';
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

const Dashboard: React.FC = () => {
  const { readings, emergency, connected, alerts, sendEmergency, stopEmergency } = useWebSocket();
  const { stageOrder, gateways, loading, error, reload } = useRegistry();

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

  return (
    <div className="min-h-screen bg-base text-ink">
      <AlertModal alerts={alerts} />

      {/* Header: brand, link state, gateways */}
      <header className="sticky top-0 z-40 border-b border-line bg-base/85 backdrop-blur">
        <div className="mx-auto flex h-12 max-w-[1440px] items-center gap-4 px-5">
          <div className="flex items-center gap-2.5">
            <span className="flex h-6 w-6 items-center justify-center rounded bg-accent/10">
              <svg width="14" height="14" viewBox="0 0 32 32" fill="none" aria-hidden>
                <path d="M4 22 L11 22 L14 8 L18 26 L21 14 L28 14" stroke="#38bdf8" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round" />
              </svg>
            </span>
            <span className="text-[13px] font-semibold tracking-[0.06em]">CAP</span>
            <span className="hidden text-[11px] text-dim md:inline">мониторинг обогатительной фабрики</span>
          </div>

          <div className="ml-auto flex items-center gap-3 text-[11px]">
            {isEmergencyActive && (
              <span className="flex items-center gap-1.5 rounded border border-red-500/40 bg-red-950/40 px-2 py-0.5 font-semibold text-red-300">
                <span className="h-1.5 w-1.5 rounded-full bg-red-500 animate-blink-soft" />
                АВАРИЙНЫЙ РЕЖИМ
              </span>
            )}

            {/* Gateway pulses */}
            {gateways.map((g) => (
              <span key={g.id} className="flex items-center gap-1.5 text-mute" title={`буфер: ${g.buffer_size}`}>
                <span className={`h-1.5 w-1.5 rounded-full ${g.status === 'online' ? 'bg-emerald-400' : 'bg-red-500 animate-blink-soft'}`} />
                <span className="font-mono">{g.id}</span>
                {g.buffer_size > 0 && <span className="text-amber-400">+{g.buffer_size}</span>}
              </span>
            ))}
            {!loading && !error && gateways.length === 0 && (
              <span className="text-dim">шлюзы не на пульсе</span>
            )}

            {/* Registry sync */}
            {loading ? (
              <span className="text-dim">реестр…</span>
            ) : error ? (
              <button onClick={reload} className="text-red-400 underline decoration-dotted">реестр: ошибка</button>
            ) : (
              <span className="hidden text-dim lg:inline">реестр {stageOrder.length} уч.</span>
            )}

            {/* Link */}
            <span className="flex items-center gap-1.5">
              <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-emerald-400' : 'bg-red-500 animate-blink-soft'}`} />
              <span className={connected ? 'text-mute' : 'text-red-400'}>{connected ? 'онлайн' : 'переподключение'}</span>
            </span>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-[1440px] space-y-3 px-5 py-4">
        {/* Stage cards */}
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

        <section className="grid grid-cols-1 gap-3 lg:grid-cols-2">
          <AlarmPanel />
          <ProfilesPanel />
        </section>

        <section className="grid grid-cols-1 gap-3 lg:grid-cols-3">
          <div className="lg:col-span-2">
            <AlertLog alerts={alerts} />
          </div>
          <InstructorPanel
            connected={connected}
            emergencyActive={isEmergencyActive}
            onTriggerEmergency={sendEmergency}
            onStopEmergency={stopEmergency}
          />
        </section>

        <AdminPanel />
      </main>

      <footer className="border-t border-line py-3 text-center text-[11px] text-dim">
        CAP · Complex Automation Pro — MVP
      </footer>
    </div>
  );
};

export default Dashboard;
