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

  const stageLabel = STAGE_META[current.stage]?.label ?? current.stage.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase());

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm">
      <div className="mx-4 max-w-lg rounded-2xl border-2 border-red-500 bg-gray-900 p-8 shadow-2xl shadow-red-500/30 animate-pulse-red">
        <div className="mb-4 flex items-center gap-3">
          <span className="text-4xl">🚨</span>
          <h2 className="text-2xl font-bold text-red-400 uppercase tracking-wide">
            Критическая тревога
          </h2>
        </div>
        <p className="mb-2 text-lg text-gray-100">
          <span className="font-bold text-red-300">{stageLabel}</span> — вышло за пределы нормы!
        </p>
        <p className="text-gray-300">
          <span className="font-mono font-bold text-red-400">{METRIC_LABELS[current.metric] ?? current.metric.replace(/_/g, ' ')}</span>{' '}
          = <span className="font-mono font-bold text-white">{current.value.toFixed(2)} {current.unit}</span>
        </p>
        <p className="mt-2 text-xs text-gray-500">
          {new Date(current.timestamp).toLocaleTimeString()}
        </p>
        <button
          onClick={() => { setVisible(false); onDismiss?.(); }}
          className="mt-6 w-full rounded-lg bg-red-600 py-2 text-sm font-semibold text-white hover:bg-red-500 transition-colors"
        >
          Подтвердить
        </button>
      </div>
    </div>
  );
};

export default AlertModal;