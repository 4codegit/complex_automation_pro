import React from 'react';
import { TelemetryReading } from '../hooks/useWebSocket';
import { STAGE_META, METRIC_LABELS } from './StatusCard';

interface Props {
  alerts: TelemetryReading[];
}

const AlertLog: React.FC<Props> = ({ alerts }) => {
  const recent = alerts.slice(-20).reverse();

  return (
    <div className="rounded-lg border border-line bg-panel p-3.5">
      <h3 className="mb-2 text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
        Журнал тревог
      </h3>
      {recent.length === 0 ? (
        <p className="py-3 text-[12px] text-dim">Тревог пока нет — система работает в норме.</p>
      ) : (
        <div className="max-h-56 space-y-1 overflow-y-auto pr-1">
          {recent.map((a, i) => {
            const meta = STAGE_META[a.stage];
            return (
              <div
                key={i}
                className="flex items-center gap-2.5 rounded border border-alarm/20 bg-alarm/10 px-2.5 py-1.5"
              >
                <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-red-500 animate-blink-soft" />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[11px] font-semibold text-alarm">
                    {meta?.label ?? a.stage}
                    <span className="mx-1.5 text-dim">/</span>
                    <span className="text-alarm">{METRIC_LABELS[a.metric] ?? a.metric.replace(/_/g, ' ')}</span>
                  </p>
                  <p className="num text-[10px] text-mute">
                    значение <span className="font-mono text-ink">{a.value.toFixed(2)} {a.unit}</span>
                  </p>
                </div>
                <span className="num shrink-0 font-mono text-[10px] text-dim">
                  {new Date(a.timestamp).toLocaleTimeString()}
                </span>
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
};

export default AlertLog;
