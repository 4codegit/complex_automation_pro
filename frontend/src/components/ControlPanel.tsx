import React from 'react';
import LoopFaceplate from './LoopFaceplate';

// ControlPanel: all supervisory loops on one screen (TZ §14.4).
const ControlPanel: React.FC = () => {
  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">
        Контуры управления (супервизорные ПИД)
      </h2>
      <div className="grid gap-3 lg:grid-cols-2 xl:grid-cols-3">
        <LoopFaceplate loopId="lic301" />
        <LoopFaceplate loopId="fic301" />
        <LoopFaceplate loopId="dic401" />
        <LoopFaceplate loopId="tic201" />
      </div>
      <p className="max-w-3xl text-[11px] leading-relaxed text-dim">
        Переключение режима и запись уставок выполняются с подтверждением и фиксируются
        в журнале аудита. При потере актуального PV контур автоматически переходит в
        ручной режим, выходной сигнал замораживается (безопасное состояние по IEC 61511).
        Запись в станцию выполняет шлюз методом FC6 только в режиме АВТО.
        Контур tic201 (температура подшипника мельницы, клапан маслостанции)
        имеет температурный интерлок: при 75 °C и выше клапан масла
        принудительно открывается на 100 % даже из ручного режима, сброс —
        после остывания ниже 70 °C.
      </p>
    </section>
  );
};

export default ControlPanel;
