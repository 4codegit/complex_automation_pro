import React, { useCallback, useEffect, useState } from 'react';
import { api } from '../api/client';
import type { ControlLoop } from '../api/types';
import { useLoopStates, useLive } from '../ws/WsProvider';
interface LoopFaceplateProps {
  loopId: string;
  compact?: boolean;
}

const stateLabel: Record<string, { text: string; cls: string }> = {
  ok: { text: 'норма', cls: 'text-ok' },
  watchdog: { text: 'потеря PV → ручной', cls: 'text-alarm' },
};

// LoopFaceplate is the operator panel for one supervisory loop (TZ §14): PV,
// SP, output bar, mode switching and setpoint writes. Every write requires an
// explicit confirm (server enforces confirm:true) — an ISA-101-style guard.
const LoopFaceplate: React.FC<LoopFaceplateProps> = ({ loopId, compact = false }) => {
  const [loop, setLoop] = useState<ControlLoop | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirmSp, setConfirmSp] = useState(false);
  const [spDraft, setSpDraft] = useState('');
  const wsLoops = useLoopStates();

  const load = useCallback(async () => {
    try {
      const loops = await api.get<ControlLoop[]>('/control/loops');
      const found = loops.find((l) => l.id === loopId);
      if (found) setLoop(found);
    } catch (err) {
      setError((err as Error).message);
    }
  }, [loopId]);

  useEffect(() => {
    void load();
  }, [load]);

  // Live overlay from the WS loop_state events (1 Hz).
  const ws = wsLoops.get(loopId);
  const pvTag = loop?.pv_tag ?? '';
  const pvLive = useLive(pvTag);
  const pv = pvLive ? pvLive.value : loop?.pv;
  const mode = ws?.mode ?? loop?.mode ?? 'manual';
  const state = ws?.state ?? loop?.state ?? 'ok';
  const out = ws?.out ?? loop?.out ?? 0;
  const sp = ws?.sp ?? loop?.sp ?? 0;

  if (!loop) {
    return <div className="rounded border border-line bg-panel p-3 text-[12px] text-dim">Загрузка контура {loopId}…</div>;
  }

  const outPct = Math.round(((out - loop.out_min) / (loop.out_max - loop.out_min)) * 100);
  const pvPct = pv !== undefined ? Math.min(100, Math.max(0, ((pv - loop.sp_min) / (loop.sp_max - loop.sp_min)) * 100)) : 0;

  const write = async (path: string, body: Record<string, unknown>) => {
    setError(null);
    try {
      await api.put<ControlLoop>(`/control/loops/${loopId}/${path}`, { confirm: true, ...body });
      await load();
      return true;
    } catch (err) {
      setError((err as Error).message);
      return false;
    }
  };

  const submitSp = async () => {
    const value = parseFloat(spDraft.replace(',', '.'));
    if (Number.isNaN(value)) {
      setError('Уставка — не число');
      return;
    }
    if (await write('setpoint', { value })) {
      setConfirmSp(false);
      setSpDraft('');
    }
  };

  return (
    <div className={`rounded-lg border border-line bg-panel ${compact ? 'p-3' : 'p-4'}`}>
      <div className="mb-2 flex items-center justify-between gap-2">
        <div>
          <h3 className="text-[12.5px] font-semibold text-ink">{loop.label}</h3>
          <p className="font-mono text-[10px] text-dim">{loop.id} · PV {loop.pv_tag} → MV {loop.mv_tag}</p>
        </div>
        <div className="flex items-center gap-1.5">
          <span className={`text-[11px] font-semibold ${stateLabel[state]?.cls ?? 'text-dim'}`}>
            {stateLabel[state]?.text ?? state}
          </span>
          <button
            onClick={() => void write('mode', { mode: mode === 'auto' ? 'manual' : 'auto' })}
            className={`rounded px-2.5 py-1 text-[11px] font-bold transition-colors ${
              mode === 'auto'
                ? 'bg-ok/15 text-ok ring-1 ring-ok/40'
                : 'bg-panel2 text-mute ring-1 ring-line hover:text-ink'
            }`}
          >
            {mode === 'auto' ? 'АВТО' : 'РУЧН'}
          </button>
        </div>
      </div>

      {/* PV / SP readouts */}
      <div className="mb-2 grid grid-cols-3 gap-2">
        <div className="rounded bg-panel2/60 px-2.5 py-1.5">
          <p className="text-[9.5px] uppercase tracking-wide text-dim">PV</p>
          <p className="num font-mono text-[16px] font-semibold text-ink">
            {pv !== undefined ? pv.toFixed(Math.abs(pv) < 20 ? 2 : 1) : '—'}
          </p>
        </div>
        <div className="rounded bg-panel2/60 px-2.5 py-1.5">
          <p className="text-[9.5px] uppercase tracking-wide text-dim">Уставка SP</p>
          <p className="num font-mono text-[16px] font-semibold text-accent">{sp.toFixed(1)}</p>
        </div>
        <div className="rounded bg-panel2/60 px-2.5 py-1.5">
          <p className="text-[9.5px] uppercase tracking-wide text-dim">Выход MV</p>
          <p className="num font-mono text-[16px] font-semibold text-ink">
            {out.toFixed(1)} <span className="text-[10px] text-dim">({outPct}%)</span>
          </p>
        </div>
      </div>

      {/* Output bar */}
      <div className="mb-2">
        <div className="h-2.5 w-full overflow-hidden rounded-full bg-panel2">
          <div
            className={`h-full rounded-full transition-all ${mode === 'auto' ? 'bg-ok/70' : 'bg-warn/70'}`}
            style={{ width: `${Math.max(0, Math.min(100, outPct))}%` }}
          />
        </div>
        <div className="mt-0.5 flex justify-between text-[9.5px] text-dim">
          <span>{loop.out_min}</span>
          <span>PV: {pvPct.toFixed(0)}% диапазона</span>
          <span>{loop.out_max}</span>
        </div>
      </div>

      {/* Writes */}
      <div className="flex flex-wrap items-center gap-2">
        <input
          value={spDraft}
          onChange={(e) => setSpDraft(e.target.value)}
          placeholder={`SP ${loop.sp_min}…${loop.sp_max}`}
          className="h-7 w-28 rounded border border-line bg-base px-2 font-mono text-[12px] text-ink outline-none focus:border-accent/60"
        />
        {!confirmSp ? (
          <button
            onClick={() => spDraft && setConfirmSp(true)}
            className="h-7 rounded border border-accent/40 bg-accent/10 px-2.5 text-[11px] font-semibold text-accent hover:bg-accent/20"
          >
            Изменить SP
          </button>
        ) : (
          <span className="flex items-center gap-1.5">
            <span className="text-[11px] text-warn">Подтвердить запись SP?</span>
            <button
              onClick={() => void submitSp()}
              className="h-7 rounded bg-accent px-2.5 text-[11px] font-semibold text-white hover:opacity-90"
            >
              Да, записать
            </button>
            <button
              onClick={() => setConfirmSp(false)}
              className="h-7 rounded border border-line px-2 text-[11px] text-mute"
            >
              Отмена
            </button>
          </span>
        )}
        {mode === 'manual' && (
          <button
            onClick={() => void write('output', { value: out })}
            className="h-7 rounded border border-line px-2.5 text-[11px] text-mute hover:text-ink"
            title="Записать текущее значение выхода (FC6)"
          >
            Записать MV в станцию
          </button>
        )}
      </div>

      {error && <p className="mt-2 text-[11px] text-alarm">{error}</p>}
    </div>
  );
};

export default LoopFaceplate;
