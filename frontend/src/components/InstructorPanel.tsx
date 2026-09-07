import React from 'react';

interface Props {
  connected: boolean;
  emergencyActive: boolean;
  onTriggerEmergency: () => void;
  onStopEmergency: () => void;
}

const InstructorPanel: React.FC<Props> = ({ connected, emergencyActive, onTriggerEmergency, onStopEmergency }) => {
  return (
    <div className="rounded-xl border border-gray-700 bg-gray-800/70 p-5">
      <h3 className="mb-3 text-sm font-semibold text-gray-400 uppercase tracking-wider">
        🎬 Панель оператора
      </h3>
      <div className="space-y-3">
        <div className="flex items-center gap-2 text-sm">
          <span className={`h-2.5 w-2.5 rounded-full ${connected ? 'bg-green-500' : 'bg-red-500'}`} />
          <span className="text-gray-300">
            WebSocket: {connected ? 'Подключено' : 'Отключено'}
          </span>
        </div>

        <div className="flex gap-3">
          <button
            onClick={onTriggerEmergency}
            disabled={emergencyActive}
            className={`rounded-lg px-4 py-2 text-sm font-semibold transition-all ${
              emergencyActive
                ? 'cursor-not-allowed bg-red-900/50 text-red-300'
                : 'bg-red-600 text-white hover:bg-red-500 shadow-lg shadow-red-600/30'
            }`}
          >
            {emergencyActive ? '🚨 Авария активна' : '⚡ Смоделировать критическую неисправность'}
          </button>

          {emergencyActive && (
            <button
              onClick={onStopEmergency}
              className="rounded-lg border border-yellow-600 bg-yellow-900/30 px-4 py-2 text-sm font-semibold text-yellow-300 hover:bg-yellow-800/40 transition-colors"
            >
              ✅ Завершить аварию
            </button>
          )}
        </div>

        {emergencyActive && (
          <p className="text-xs text-yellow-400 animate-pulse">
            Режим аварии активен — влажность шлама выросла до 15%, pH флотации упал до 5.0
          </p>
        )}
      </div>
    </div>
  );
};

export default InstructorPanel;