import React from 'react';
import { TelemetryReading } from '../hooks/useWebSocket';
import { STAGE_META, METRIC_LABELS } from './StatusCard';

interface Props {
  alerts: TelemetryReading[];
}

const STAGE_KEYS = ['crushing_grinding', 'flotation', 'drying_dewatering', 'final_concentrate'] as const;

const METRICS_BY_STAGE: Record<string, string[]> = {
  crushing_grinding: ['particle_size', 'pulp_density'],
  flotation: ['ph_level', 'reagent_dosage'],
  drying_dewatering: ['cake_moisture', 'dryer_temperature'],
  final_concentrate: ['tonnage_weight', 'final_moisture'],
};

function groupAlertsByStage(alerts: TelemetryReading[]): Map<string, TelemetryReading[]> {
  const map = new Map<string, TelemetryReading[]>();
  for (const a of alerts) {
    const stage = a.stage;
    if (!map.has(stage)) map.set(stage, []);
    map.get(stage)!.push(a);
  }
  return map;
}

const AlertLog: React.FC<Props> = ({ alerts }) => {
  const grouped = groupAlertsByStage(alerts);
  const recent = alerts.slice(-20).reverse();

  return (
    <div className="rounded-xl border border-gray-700 bg-gray-800/70 p-5">
      <h3 className="mb-3 text-sm font-semibold text-gray-400 uppercase tracking-wider">
        📋 Журнал тревог
      </h3>
      {recent.length === 0 ? (
        <p className="text-sm text-gray-500 italic">Тревог пока нет — система работает в норме.</p>
      ) : (
        <div className="max-h-64 overflow-y-auto space-y-2">
          {recent.map((a, i) => {
            const meta = STAGE_META[a.stage] || { label: a.stage, icon: '❓' };
            return (
              <div
                key={i}
                className="flex items-center gap-3 rounded-lg bg-red-950/40 border border-red-800/50 px-3 py-2"
              >
                <span className="text-lg">{meta.icon}</span>
                <div className="flex-1">
                  <p className="text-xs text-red-300 font-semibold">
                    {meta.label} — {METRIC_LABELS[a.metric] ?? a.metric.replace(/_/g, ' ')}
                  </p>
                  <p className="text-xs text-gray-400">
                    Значение: <span className="font-mono text-white">{a.value.toFixed(2)} {a.unit}</span>
                  </p>
                </div>
                <span className="text-[10px] text-gray-500">
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