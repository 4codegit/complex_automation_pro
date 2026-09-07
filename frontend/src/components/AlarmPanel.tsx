import React, { useState } from 'react';
import { AlarmState, stateLabel, useAlarms } from '../hooks/useAlarms';

const SEVERITY_STYLES: Record<string, string> = {
  critical: 'bg-red-950/60 border-red-600 text-red-300',
  high: 'bg-orange-950/50 border-orange-600 text-orange-300',
  medium: 'bg-amber-950/40 border-amber-500 text-amber-200',
};

function formatTime(value: string | null): string {
  if (!value) return '—';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('ru-RU', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' });
}

const AlarmPanel: React.FC = () => {
  const { alarms, loading, acknowledge } = useAlarms();
  const [operator, setOperator] = useState('operator-01');
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const onAck = async (alarm: AlarmState) => {
    setBusy(alarm.id);
    setError(null);
    try {
      await acknowledge(alarm.id, operator, 'Подтверждено с панели оператора');
    } catch {
      setError(`Не удалось подтвердить ${alarm.metric}`);
    } finally {
      setBusy(null);
    }
  };

  const activeCount = alarms.filter((a) => a.state.startsWith('active')).length;

  return (
    <section className="rounded-xl border border-gray-700 bg-gray-800/70 p-5">
      <div className="mb-4 flex items-center justify-between gap-3">
        <h3 className="text-sm font-semibold uppercase tracking-wider text-gray-300">
          🔔 Активные аварии и алермы
        </h3>
        <span className="rounded-full bg-red-600/20 px-2.5 py-0.5 text-xs font-bold text-red-300">
          {activeCount}
        </span>
      </div>

      <label className="mb-3 block">
        <span className="mb-1 block text-xs text-gray-500">Оператор (ack_by)</span>
        <input
          value={operator}
          onChange={(event) => setOperator(event.target.value)}
          className="h-8 w-full max-w-xs rounded border border-gray-700 bg-gray-950 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500"
        />
      </label>

      {error && <p className="mb-3 text-xs text-red-400">{error}</p>}

      <div className="space-y-2">
        {alarms.length === 0 && (
          <p className="py-4 text-sm text-gray-500">
            {loading ? 'Загрузка…' : 'Активных аварий нет — процесс в норме.'}
          </p>
        )}
        {alarms.map((alarm) => (
          <div
            key={alarm.id}
            className={`rounded-lg border px-3 py-2.5 ${SEVERITY_STYLES[alarm.severity] ?? 'border-gray-600 bg-gray-900/60 text-gray-300'}`}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="truncate text-sm font-semibold">{alarm.metric.replace(/_/g, ' ')}</p>
                <p className="mt-0.5 truncate font-mono text-[11px] text-gray-500" title={alarm.tag_id}>{alarm.tag_id}</p>
                <p className="mt-1 text-xs text-gray-400">{alarm.message}</p>
                <p className="mt-1 text-[11px] text-gray-500">
                  {stateLabel(alarm.state)} · с {formatTime(alarm.observed_at)}
                  {alarm.ack_by && <> · подтвердил: {alarm.ack_by}</>}
                </p>
              </div>
              {alarm.state === 'active_unacknowledged' && (
                <button
                  onClick={() => void onAck(alarm)}
                  disabled={busy === alarm.id}
                  className="shrink-0 rounded-lg bg-cyan-700 px-3 py-1.5 text-xs font-semibold text-white transition-colors hover:bg-cyan-600 disabled:opacity-50"
                >
                  {busy === alarm.id ? '…' : 'Подтвердить'}
                </button>
              )}
            </div>
          </div>
        ))}
      </div>
    </section>
  );
};

export default AlarmPanel;
