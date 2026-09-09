import React, { useState } from 'react';
import { AlarmState, stateLabel, useAlarms } from '../hooks/useAlarms';

const SEVERITY_BAR: Record<string, string> = {
  critical: 'border-l-2 border-l-red-500',
  high: 'border-l-2 border-l-orange-400',
  medium: 'border-l-2 border-l-warn',
};

const SEVERITY_DOT: Record<string, string> = {
  critical: 'bg-red-500',
  high: 'bg-orange-400',
  medium: 'bg-warn',
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
    <section className="flex flex-col rounded-lg border border-line bg-panel p-3.5">
      <div className="mb-2.5 flex items-center justify-between gap-2">
        <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
          Активные аварии и алермы
        </h3>
        <span
          className={`num rounded px-1.5 py-0.5 font-mono text-[11px] font-semibold ${
            activeCount > 0 ? 'bg-alarm/10 text-alarm' : 'bg-panel2 text-dim'
          }`}
        >
          {activeCount}
        </span>
      </div>

      <label className="mb-2.5 block">
        <span className="sr-only">Оператор (ack_by)</span>
        <input
          value={operator}
          onChange={(event) => setOperator(event.target.value)}
          className="h-7 w-full max-w-[220px] rounded border border-line bg-base px-2 font-mono text-[12px] text-ink outline-none placeholder:text-dim focus:border-accent/60"
          title="Оператор для аудита квитирования"
        />
      </label>

      {error && <p className="mb-2 text-[11px] text-alarm">{error}</p>}

      <div className="space-y-1.5">
        {alarms.length === 0 && (
          <p className="py-3 text-[12px] text-dim">
            {loading ? 'Загрузка…' : 'Активных аварий нет — процесс в норме.'}
          </p>
        )}
        {alarms.map((alarm) => (
          <div
            key={alarm.id}
            className={`rounded border border-line bg-panel2/50 px-2.5 py-2 ${SEVERITY_BAR[alarm.severity] ?? ''}`}
          >
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="flex items-center gap-1.5 truncate text-[12px] font-semibold text-ink">
                  <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${SEVERITY_DOT[alarm.severity] ?? 'bg-gray-500'}`} />
                  {alarm.metric.replace(/_/g, ' ')}
                </p>
                <p className="mt-0.5 truncate font-mono text-[10px] text-dim" title={alarm.tag_id}>{alarm.tag_id}</p>
                <p className="mt-1 text-[11px] text-mute">{alarm.message}</p>
                <p className="num mt-0.5 text-[10px] text-dim">
                  {stateLabel(alarm.state)} · с {formatTime(alarm.observed_at)}
                  {alarm.ack_by && <> · подтвердил: <span className="font-mono">{alarm.ack_by}</span></>}
                </p>
              </div>
              {alarm.state === 'active_unacknowledged' && (
                <button
                  onClick={() => void onAck(alarm)}
                  disabled={busy === alarm.id}
                  className="h-7 shrink-0 rounded border border-accent/40 bg-accent/10 px-2.5 text-[11px] font-semibold text-accent transition-colors hover:bg-accent/20 disabled:opacity-50"
                >
                  {busy === alarm.id ? '…' : 'Квитировать'}
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
