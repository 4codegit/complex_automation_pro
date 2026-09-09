import React, { useEffect, useState } from 'react';
import { TelemetryReading } from '../hooks/useWebSocket';
import { STAGE_META, METRIC_LABELS } from './StatusCard';

interface Props {
  alerts: TelemetryReading[];
  onDismiss?: () => void;
}

const AlertModal: React.FC<Props> = ({ alerts, onDismiss }) => {
  const [visible, setVisible] = useState(false);
  const [current, setCurrent] = useState<TelemetryReading | null>(null);

  useEffect(() => {
    if (alerts.length > 0) {
      const latest = alerts[alerts.length - 1];
      setCurrent(latest);
      setVisible(true);
      const timer = setTimeout(() => setVisible(false), 5000);
      return () => clearTimeout(timer);
    }
  }, [alerts]);

  if (!visible || !current) return null;

  const stageLabel = STAGE_META[current.stage]?.label ?? current.stage.replace(/_/g, ' ');

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 backdrop-blur-sm">
      <div className="mx-4 w-full max-w-sm rounded-lg border border-alarm/50 bg-panel p-5 shadow-2xl shadow-black/10 animate-pulse-red">
        <div className="mb-3 flex items-center gap-2">
          <span className="h-2 w-2 rounded-full bg-red-500 animate-blink-soft" />
          <h2 className="text-[12px] font-bold uppercase tracking-[0.1em] text-alarm">
            Критическая тревога
          </h2>
        </div>
        <p className="text-[13px] text-ink">
          <span className="font-semibold text-alarm">{stageLabel}</span> — выход за пределы нормы
        </p>
        <p className="num mt-2 font-mono text-[13px] text-ink">
          {METRIC_LABELS[current.metric] ?? current.metric.replace(/_/g, ' ')}{' '}
          = <span className="font-semibold text-alarm">{current.value.toFixed(2)}</span>{' '}
          <span className="text-dim">{current.unit}</span>
        </p>
        <p className="num mt-1.5 font-mono text-[10px] text-dim">
          {new Date(current.timestamp).toLocaleTimeString()}
        </p>
        <button
          onClick={() => { setVisible(false); onDismiss?.(); }}
          className="mt-4 h-8 w-full rounded border border-alarm/40 bg-alarm/15 text-[12px] font-semibold text-alarm transition-colors hover:bg-alarm/25"
        >
          Квитировать
        </button>
      </div>
    </div>
  );
};

export default AlertModal;
