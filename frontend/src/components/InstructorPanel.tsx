import React from 'react';

interface Props {
  connected: boolean;
  emergencyActive: boolean;
  onTriggerEmergency: () => void;
  onStopEmergency: () => void;
}

const InstructorPanel: React.FC<Props> = ({ connected, emergencyActive, onTriggerEmergency, onStopEmergency }) => {
  return (
    <div className="flex flex-col rounded-lg border border-line bg-panel p-3.5">
      <div className="mb-2.5 flex items-center justify-between gap-2">
        <h3 className="text-[11px] font-semibold uppercase tracking-[0.07em] text-mute">
          Режим инструктора
        </h3>
        <span className="rounded border border-line px-1.5 py-0.5 text-[9px] font-semibold uppercase tracking-wide text-dim">
          dev
        </span>
      </div>

      <div className="space-y-2.5">
        <p className="flex items-center gap-1.5 text-[11px] text-mute">
          <span className={`h-1.5 w-1.5 rounded-full ${connected ? 'bg-ok' : 'bg-red-500'}`} />
          WebSocket: {connected ? 'подключено' : 'отключено'}
        </p>

        <div className="flex flex-wrap gap-2">
          <button
            onClick={onTriggerEmergency}
            disabled={emergencyActive}
            className={`h-8 rounded px-3 text-[11px] font-semibold transition-colors ${
              emergencyActive
                ? 'cursor-not-allowed border border-alarm/30 bg-alarm/10 text-alarm'
                : 'bg-red-600 text-white hover:bg-red-500'
            }`}
          >
            {emergencyActive ? 'Авария активна' : 'Смоделировать неисправность'}
          </button>

          {emergencyActive && (
            <button
              onClick={onStopEmergency}
              className="h-8 rounded border border-warn/40 bg-warn/10 px-3 text-[11px] font-semibold text-warn transition-colors hover:bg-warn/20"
            >
              Завершить
            </button>
          )}
        </div>

        {emergencyActive && (
          <p className="text-[11px] leading-relaxed text-warn/90">
            Режим аварии активен — влажность шлама выросла до 15%, pH флотации упал до 5.0.
          </p>
        )}
      </div>
    </div>
  );
};

export default InstructorPanel;
