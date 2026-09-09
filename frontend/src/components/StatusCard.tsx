import React from 'react';
import { TelemetryReading } from '../hooks/useWebSocket';

// Stage identity: label + accent used for the card's left rail and charts.
export const STAGE_META: Record<string, { label: string; accent: string }> = {
  crushing_grinding: { label: 'Дробление и измельчение', accent: '#818cf8' },
  flotation: { label: 'Флотация', accent: '#38bdf8' },
  drying_dewatering: { label: 'Сушка / Обезвоживание', accent: '#fbbf24' },
  final_concentrate: { label: 'Финальный концентрат', accent: '#34d399' },
};

export const METRIC_LABELS: Record<string, string> = {
  particle_size: 'Размер частиц',
  pulp_density: 'Плотность пульпы',
  ph_level: 'pH',
  reagent_dosage: 'Дозировка реагента',
  cake_moisture: 'Влажность шлама',
  dryer_temperature: 'Температура сушилки',
  tonnage_weight: 'Тоннаж',
  final_moisture: 'Конечная влажность',
};

const QUALITY_DOT: Record<string, { cls: string; label: string }> = {
  good: { cls: 'bg-emerald-400', label: 'хорошее' },
  uncertain: { cls: 'bg-amber-400', label: 'неопределённое' },
  bad: { cls: 'bg-red-500', label: 'негодное' },
  stale: { cls: 'bg-gray-500', label: 'устаревшее' },
  substituted: { cls: 'bg-violet-400', label: 'замещённое' },
  offline: { cls: 'bg-red-500', label: 'нет связи' },
};

interface Props {
  stageKey: string;
  label?: string;
  readings: TelemetryReading[];
  hasAlert: boolean;
  isEmergency: boolean;
}

const StatusCard: React.FC<Props> = ({ stageKey, label, readings, hasAlert, isEmergency }) => {
  const meta = STAGE_META[stageKey];
  const title = label ?? meta?.label ?? stageKey;
  const accent = meta?.accent ?? '#5c6875';
  const alarm = hasAlert || isEmergency;

  return (
    <div
      className={`rounded-lg border bg-panel p-3.5 transition-colors ${
        alarm ? 'border-red-500/60 animate-pulse-red' : 'border-line'
      }`}
      style={alarm ? undefined : { borderLeft: `2px solid ${accent}` }}
    >
      <div className="mb-2 flex items-center justify-between gap-2">
        <h3 className="truncate text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
          {title}
        </h3>
        {hasAlert ? (
          <span className="flex shrink-0 items-center gap-1 rounded bg-red-950/50 px-1.5 py-0.5 text-[10px] font-semibold text-red-300">
            <span className="h-1 w-1 rounded-full bg-red-500 animate-blink-soft" />
            ТРЕВОГА
          </span>
        ) : isEmergency ? (
          <span className="flex shrink-0 items-center gap-1 rounded bg-amber-950/50 px-1.5 py-0.5 text-[10px] font-semibold text-amber-300">
            <span className="h-1 w-1 rounded-full bg-amber-400 animate-blink-soft" />
            АВАРИЯ
          </span>
        ) : null}
      </div>

      <div>
        {readings.map((r) => {
          const q = r.quality ? QUALITY_DOT[r.quality] : undefined;
          return (
            <div
              key={r.metric}
              className="flex items-center justify-between gap-2 border-b border-line/60 py-[7px] last:border-0"
            >
              <span className="flex min-w-0 items-center gap-1.5 text-[12px] text-mute">
                {q && <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${q.cls}`} title={`Качество: ${q.label}`} />}
                <span className="truncate">{METRIC_LABELS[r.metric] ?? r.metric.replace(/_/g, ' ')}</span>
              </span>
              <span
                className={`num shrink-0 font-mono text-[14px] font-semibold ${r.alert ? 'text-red-400' : 'text-ink'}`}
              >
                {r.value.toFixed(2)}
                <span className="ml-1 text-[10px] font-normal text-dim">{r.unit}</span>
              </span>
            </div>
          );
        })}
        {readings.length === 0 && (
          <p className="py-2 text-[12px] text-dim">Ожидание данных…</p>
        )}
      </div>
    </div>
  );
};

export default StatusCard;
