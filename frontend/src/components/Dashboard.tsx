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
  const offlineGateways = gateways.filter((g) => g.status !== 'online');

  return (
    <div className="min-h-screen bg-gray-950 text-gray-100">
      {/* Alert Modal */}
      <AlertModal alerts={alerts} />

      {/* Header */}
      <header className="border-b border-gray-800 bg-gray-900/80 backdrop-blur-sm sticky top-0 z-40">
        <div className="mx-auto max-w-7xl px-4 py-4 flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className="text-3xl">⚙️</span>
            <div>
              <h1 className="text-xl font-bold tracking-tight">CAP</h1>
              <p className="text-xs text-gray-500">Панель мониторинга обогатительной фабрики</p>
            </div>
          </div>
          <div className="flex items-center gap-4">
            {isEmergencyActive && (
              <span className="rounded-full bg-red-600 px-3 py-1 text-xs font-bold text-white animate-pulse">
                🚨 АВАРИЯ
              </span>
            )}
            <div className="flex items-center gap-2 text-sm">
              <span className={`h-2.5 w-2.5 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500 animate-pulse'}`} />
              <span className="text-gray-400">{connected ? 'Онлайн' : 'Переподключение…'}</span>
            </div>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-7xl px-4 py-6 space-y-6">
        {/* Registry sync strip */}
        <div className="flex flex-wrap items-center justify-between gap-2 text-xs">
          <span className="text-gray-500">
            Реестр: {loading ? 'загрузка…' : error ? (
              <button onClick={reload} className="text-red-400 underline">ошибка — обновить</button>
            ) : (
              <span className="text-emerald-400">синхронизирован ({stageOrder.length} участков)</span>
            )}
          </span>
          <span className="flex flex-wrap gap-3">
            {gateways.map((g) => (
              <span key={g.id} className={`inline-flex items-center gap-1.5 ${g.status === 'online' ? 'text-emerald-400' : 'text-red-400'}`}>
                <span className={`h-2 w-2 rounded-full ${g.status === 'online' ? 'bg-emerald-500' : 'bg-red-500 animate-pulse'}`} />
                {g.id}
                {g.buffer_size > 0 && <span className="text-amber-400">буфер: {g.buffer_size}</span>}
              </span>
            ))}
            {!loading && !error && gateways.length === 0 && (
              <span className="text-gray-600">шлюзы ещё не прислали пульс</span>
            )}
          </span>
        </div>

        {/* Status Cards (from registry) */}
        <section className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-4 gap-4">
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

        {/* Live Chart */}
        <section>
          <LiveChart readings={readings} />
        </section>

        {/* Alarms + Profiles */}
        <section className="grid grid-cols-1 lg:grid-cols-2 gap-4">
          <AlarmPanel />
          <ProfilesPanel />
        </section>

        {/* Bottom Row: Alert Log + Instructor Panel */}
        <section className="grid grid-cols-1 lg:grid-cols-3 gap-4">
          <div className="lg:col-span-2">
            <AlertLog alerts={alerts} />
          </div>
          <div>
            <InstructorPanel
              connected={connected}
              emergencyActive={isEmergencyActive}
              onTriggerEmergency={sendEmergency}
              onStopEmergency={stopEmergency}
            />
          </div>
        </section>

        <AdminPanel />
      </main>

      {/* Footer */}
      <footer className="border-b border-gray-800 mt-8 py-4 text-center text-xs text-gray-600">
        CAP MVP — Система мониторинга обогатительной фабрики
        {offlineGateways.length > 0 && (
          <span className="ml-2 text-red-500">· шлюзы офлайн: {offlineGateways.map((g) => g.id).join(', ')}</span>
        )}
      </footer>
    </div>
  );
};

export default Dashboard;
