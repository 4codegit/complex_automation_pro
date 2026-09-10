import React, { useCallback, useEffect, useMemo, useState } from 'react';
import { TelemetryReading, useWebSocket } from '../hooks/useWebSocket';
import { useRegistry } from '../hooks/useRegistry';
import { useAlarms } from '../hooks/useAlarms';
import { METRIC_LABELS } from './StatusCard';

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

// Mimic layout: one flowsheet row, pipeline connectors, live values on the
// nodes. This is the SCADA-style synoptic view of the demo flowsheet.
const NODES: {
  stage: string;
  title: string;
  metrics: string[];
  x: number;
  accent: string;
}[] = [
  { stage: 'crushing_grinding', title: 'Дробление · измельчение', metrics: ['particle_size', 'pulp_density'], x: 15, accent: '#818cf8' },
  { stage: 'flotation', title: 'Флотация', metrics: ['ph_level', 'reagent_dosage'], x: 335, accent: '#38bdf8' },
  { stage: 'drying_dewatering', title: 'Сгущение · сушка', metrics: ['cake_moisture', 'dryer_temperature'], x: 655, accent: '#fbbf24' },
  { stage: 'final_concentrate', title: 'Концентрат · отгрузка', metrics: ['tonnage_weight', 'final_moisture'], x: 975, accent: '#34d399' },
];

const VIEW_W = 1240, VIEW_H = 360, NODE_W = 250, NODE_H = 150;

type ControlInfo = {
  enabled: boolean;
  status?: string;
  setpoint?: { value: number };
  pv?: { value: number };
  output?: number;
};

const SynopticPanel: React.FC = () => {
  const { readings } = useWebSocket();
  const { stageOrder, loading } = useRegistry();
  const { alarms } = useAlarms();
  const [ctrl, setCtrl] = useState<ControlInfo | null>(null);

  const loadCtrl = useCallback(async () => {
    try {
      const r = await fetch(`${API_URL}/control/status`, { headers: { Accept: 'application/json' } });
      if (r.ok) setCtrl((await r.json()) as ControlInfo);
    } catch {
      // mimic stays functional without the control service
    }
  }, []);

  useEffect(() => {
    void loadCtrl();
    const t = setInterval(() => void loadCtrl(), 3000);
    return () => clearInterval(t);
  }, [loadCtrl]);

  const byMetric = useMemo(() => {
    const m = new Map<string, TelemetryReading>();
    readings.forEach((r) => m.set(r.metric, r));
    return m;
  }, [readings]);

  const unack = alarms.filter((a) => a.state === 'active_unacknowledged').length;

  const nodeStatus = (stage: string, metrics: string[]): 'ok' | 'alarm' | 'offline' => {
    const rs = metrics.map((m) => byMetric.get(m)).filter(Boolean) as TelemetryReading[];
    if (rs.length === 0) return 'offline';
    if (rs.some((r) => r.alert)) return 'alarm';
    if (rs.every((r) => r.quality === 'offline' || r.quality === 'stale')) return 'offline';
    return 'ok';
  };

  const stageTitle = (stage: string, fallback: string) =>
    stageOrder.find((s) => s.area === stage)?.label ?? fallback;

  return (
    <div className="space-y-3">
      {/* Alarm banner — classic SCADA annunciation strip */}
      <div
        className={`flex items-center gap-3 rounded-lg border px-3.5 py-2 text-[12px] ${
          unack > 0
            ? 'border-alarm/40 bg-alarm/10 font-semibold text-alarm'
            : 'border-line bg-panel text-dim'
        }`}
      >
        <span className={`h-2 w-2 rounded-full ${unack > 0 ? 'bg-red-500 animate-blink-soft' : 'bg-emerald-400'}`} />
        {unack > 0 ? (
          <>
            <span>НЕКВИТИРОВАННЫХ АВАРИЙ: {unack}</span>
            <a href="#alarms" className="ml-auto underline decoration-dotted">перейти в Аварии →</a>
          </>
        ) : (
          <span>Активных аварий нет — технологический процесс в норме</span>
        )}
      </div>

      {/* Mimic canvas */}
      <div className="rounded-lg border border-line bg-panel p-3.5">
        <div className="mb-2 flex items-center justify-between">
          <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
            Мнемосхема — технологическая цепочка
          </h3>
          <span className="text-[10px] text-dim">значения обновляются в реальном времени</span>
        </div>

        <svg viewBox={`0 0 ${VIEW_W} ${VIEW_H}`} className="w-full" role="img" aria-label="Мнемосхема фабрики">
          {/* pipeline */}
          <defs>
            <marker id="arrow" markerWidth="9" markerHeight="9" refX="7" refY="3.2" orient="auto">
              <path d="M0,0 L7,3.2 L0,6.4" fill="none" stroke="rgb(var(--c-dim))" strokeWidth="1.4" />
            </marker>
          </defs>
          <path d="M 15 60 H 1225" stroke="rgb(var(--c-line))" strokeWidth="10" fill="none" strokeLinecap="round" />
          <path d="M 15 60 H 1210" stroke="rgb(var(--c-dim))" strokeWidth="1.4" fill="none" markerEnd="url(#arrow)" opacity="0.7" />
          {[275, 595, 915].map((x) => (
            <circle key={x} cx={x} cy={60} r="4" fill="rgb(var(--c-dim))" />
          ))}
          <text x={15} y={28} fontSize="12" fill="rgb(var(--c-dim))">руда →</text>
          <text x={1140} y={28} fontSize="12" fill="rgb(var(--c-dim))">концентрат → отгрузка</text>

          {/* flotation control bubble: doser -> node */}
          {ctrl?.enabled && ctrl.status === 'auto' && (
            <>
              <path d="M 460 60 V 118" stroke="rgb(var(--c-accent))" strokeWidth="1.6" fill="none" markerEnd="url(#arrow)" />
              <text x={468} y={112} fontSize="11" fill="rgb(var(--c-accent))" fontFamily="monospace">
                дозатор {Math.round(ctrl.output ?? 0)}%
              </text>
            </>
          )}

          {NODES.map((n) => {
            const status = nodeStatus(n.stage, n.metrics);
            const border =
              status === 'alarm' ? '#ef4444' : status === 'offline' ? 'rgb(var(--c-dim))' : n.accent;
            const strokeW = status === 'alarm' ? 2.5 : 1.5;
            return (
              <g key={n.stage}>
                <rect
                  x={n.x} y={95} width={NODE_W} height={NODE_H} rx={10}
                  fill="rgb(var(--c-panel))" stroke={border} strokeWidth={strokeW}
                />
                <rect x={n.x} y={95} width={6} height={NODE_H} rx={3} fill={n.accent} />
                <text x={n.x + 18} y={122} fontSize="13" fontWeight="700" fill="rgb(var(--c-ink))" fontFamily="Arial">
                  {stageTitle(n.stage, n.title).toUpperCase()}
                </text>
                {n.metrics.map((metric, i) => {
                  const r = byMetric.get(metric);
                  const dead = r && (r.quality === 'offline' || r.quality === 'stale');
                  return (
                    <g key={metric} transform={`translate(${n.x + 18} ${155 + i * 40})`}>
                      <text fontSize="11.5" fill="rgb(var(--c-mute))" fontFamily="Arial">
                        {METRIC_LABELS[metric] ?? metric}
                      </text>
                      <text x={NODE_W - 36} y={0} textAnchor="end" fontSize="16" fontWeight="600" fontFamily="monospace"
                        fill={r?.alert ? '#ef4444' : dead ? 'rgb(var(--c-dim))' : 'rgb(var(--c-ink))'}>
                        {dead ? '—' : r ? r.value.toFixed(2) : '…'}
                      </text>
                      <text x={NODE_W - 36} y={14} textAnchor="end" fontSize="9.5" fill="rgb(var(--c-dim))" fontFamily="Arial">
                        {r ? r.unit : ''}
                      </text>
                    </g>
                  );
                })}
                {status === 'alarm' && (
                  <circle cx={n.x + NODE_W - 18} cy={115} r="5" fill="#ef4444" className="animate-blink-soft" />
                )}
                {status === 'offline' && (
                  <text x={n.x + 18} y={NODE_H + 112} fontSize="10" fill="rgb(var(--c-dim))" fontFamily="Arial">нет связи</text>
                )}
              </g>
            );
          })}

          {/* control loop bubble on flotation */}
          {ctrl?.enabled && ctrl.setpoint && (
            <g transform={`translate(335 ${95 + NODE_H + 14})`}>
              <rect width={NODE_W} height={44} rx={8} fill="rgb(var(--c-accent))" opacity={ctrl.status === 'auto' ? 0.12 : 0.05} />
              <text x={12} y={19} fontSize="10.5" fill="rgb(var(--c-accent))" fontFamily="Arial">
                КОНТУР pH {ctrl.status === 'auto' ? '· АВТО' : ctrl.status === 'paused_watchdog' ? '· ПАУЗА' : '· ВЫКЛ'}
              </text>
              <text x={12} y={35} fontSize="11.5" fontFamily="monospace" fill="rgb(var(--c-ink))">
                SP {ctrl.setpoint.value.toFixed(1)} · PV {(ctrl.pv?.value ?? 0).toFixed(2)} · вых {Math.round(ctrl.output ?? 0)}%
              </text>
            </g>
          )}
        </svg>

        <div className="mt-1 flex flex-wrap gap-x-4 gap-y-1 text-[10px] text-dim">
          <span className="flex items-center gap-1"><span className="h-1.5 w-1.5 rounded-full bg-emerald-400" /> норма</span>
          <span className="flex items-center gap-1"><span className="h-1.5 w-1.5 rounded-full bg-red-500 animate-blink-soft" /> тревога / нет связи</span>
          <span>клик по метрике на «Обзоре» — детали; уставка и ручной режим — в «Управлении»</span>
        </div>
      </div>

      {loading && <p className="text-[11px] text-dim">реестр загружается…</p>}
    </div>
  );
};

export default SynopticPanel;
