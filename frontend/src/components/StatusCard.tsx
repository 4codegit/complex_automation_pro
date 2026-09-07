import React from 'react';
import { TelemetryReading } from '../hooks/useWebSocket';

export const STAGE_META: Record<string, { label: string; icon: string; color: string }> = {
  crushing_grinding: { label: 'Дробление и измельчение', icon: '⛏️', color: 'slate' },
  flotation: { label: 'Флотация', icon: '🧪', color: 'blue' },
  drying_dewatering: { label: 'Сушка / Обезвоживание', icon: '🔥', color: 'amber' },
  final_concentrate: { label: 'Финальный концентрат', icon: '💰', color: 'emerald' },
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

export const QUALITY_BADGE: Record<string, { label: string; cls: string }> = {
  good:   { label: '✅', cls: 'bg-emerald-900/60 text-emerald-300' },
  uncertain: { label: '?' , cls: 'bg-amber-900/60 text-amber-300' },
  bad:    { label: '❌', cls: 'bg-red-900/60 text-red-300' },
  stale:  { label: '⏳',  cls: 'bg-gray-700/60 text-gray-400' },
  substituted: { label: '↩', cls: 'bg-purple-900/60 text-purple-300' },
  offline: { label: '🔌', cls: 'bg-slate-900/60 text-slate-400' },
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
  const alertClass = hasAlert || isEmergency
    ? 'bg-red-900/60 border-red-500 animate-pulse-red ring-2 ring-red-500'
    : 'bg-gray-800/70 border-gray-700';

  return (
    <div className={`rounded-xl border p-5 transition-all duration-300 ${alertClass}`}>
      <div className="flex items-center gap-3 mb-4">
        <span className="text-2xl">{meta?.icon ?? '⚙️'}</span>
        <h3 className="text-lg font-semibold text-gray-100">{title}</h3>
        {hasAlert && (
          <span className="ml-auto inline-flex items-center rounded-full bg-red-600 px-2.5 py-0.5 text-xs font-bold text-white">
            ТРЕВОГА
          </span>
        )}
        {isEmergency && !hasAlert && (
          <span className="ml-auto inline-flex items-center rounded-full bg-yellow-600 px-2.5 py-0.5 text-xs font-bold text-white">
            АВАРИЯ
          </span>
        )}
      </div>
      <div className="grid grid-cols-1 gap-3">
        {readings.map((r) => (
          <div key={r.metric} className="flex items-center justify-between rounded-lg bg-gray-900/50 px-3 py-2">
            <span className="text-sm text-gray-400 capitalize">{METRIC_LABELS[r.metric] ?? r.metric.replace(/_/g, ' ')}</span>
            <span className="flex items-center gap-2">
              {r.quality && QUALITY_BADGE[r.quality] && (
                <span className={`text-[10px] px-1 py-0.5 rounded ${QUALITY_BADGE[r.quality].cls}`}
                  title={`Качество: ${r.quality}`}>{QUALITY_BADGE[r.quality].label}</span>
              )}
              <span className={`text-sm font-mono font-bold ${r.alert ? 'text-red-400' : 'text-gray-100'}`}>
                {r.value.toFixed(2)} <span className="text-xs text-gray-500">{r.unit}</span>
              </span>
            </span>
          </div>
        ))}
        {readings.length === 0 && (
          <p className="text-sm text-gray-500 italic">Ожидание данных…</p>
        )}
      </div>
    </div>
  );
};

export default StatusCard;