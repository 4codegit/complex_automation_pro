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
  interlock: { text: 'ПЕРЕГРЕВ → масло 100%', cls: 'text-alarm animate-pulse' },
};

// LoopFaceplate is the operator panel for one supervisory loop (TZ §14): PV,
// SP, output bar, mode switching and setpoint writes. Every write requires an
// explicit confirm (server enforces confirm:true) — an ISA-101-style guard.
const LoopFaceplate: React.FC<LoopFaceplateProps> = ({ loopId, compact = false }) => {
  const [loop, setLoop] = useState<ControlLoop | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [confirmSp, setConfirmSp] = useState(false);
  const [spDraft, setSpDraft] = useState('');
  const [showAdaptive, setShowAdaptive] = useState(false);
  const [adaptiveDraft, setAdaptiveDraft] = useState({
    indicator_tag: '',
    gain_low: 50,
    gain_high: 200,
  });
  const [showPH, setShowPH] = useState(false);
  const [phDraft, setPHDraft] = useState({
    warning: 0.2,
    critical: 0.5,
    kpTemp: 0.02,
    kiFlow: 0.01,
    tempTag: '',
    flowTag: '',
  });
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
  const adaptiveEnabled = ws?.adaptive_enabled ?? loop?.adaptive_enabled ?? false;
  const gainFactor = ws?.gain_factor ?? loop?.current_factor;
  const isPH = loop?.loop_type === 'ph';
  const phWarning = ws?.ph_deadband_warning ?? loop?.ph_deadband_warning ?? 0.2;
  const phCritical = ws?.ph_deadband_critical ?? loop?.ph_deadband_critical ?? 0.5;
  const selfTuning = ws?.self_tuning_enabled ?? loop?.self_tuning_enabled ?? false;

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
          {adaptiveEnabled && (
            <span className="rounded bg-accent/15 px-1.5 py-0.5 text-[9.5px] font-semibold text-accent ring-1 ring-accent/40" title="Адаптивная настройка Kp/Ki включена">
              ADAPT{gainFactor != null ? ` ×${gainFactor.toFixed(2)}` : ''}
            </span>
          )}
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

      {/* Adaptive gain scheduling toggle */}
      {!compact && (
        <div className="mt-2 border-t border-line pt-2">
          <button
            onClick={() => setShowAdaptive(!showAdaptive)}
            className="text-[10px] text-dim hover:text-mute transition-colors"
          >
            {showAdaptive ? '▼ Скрыть' : '▶'} Адаптивная настройка (gain scheduling)
          </button>
          {showAdaptive && (
            <div className="mt-2 space-y-2 rounded bg-panel2/50 p-2.5">
              <label className="flex items-center gap-2 text-[11px]">
                <input
                  type="checkbox"
                  checked={adaptiveEnabled}
                  onChange={(e) => void write('adaptive', {
                    adaptive_enabled: e.target.checked,
                    gain_indicator_tag: loop?.gain_indicator_tag || adaptiveDraft.indicator_tag,
                    gain_low: loop?.gain_low ?? adaptiveDraft.gain_low,
                    gain_high: loop?.gain_high ?? adaptiveDraft.gain_high,
                  })}
                  className="h-3.5 w-3.5 rounded accent-accent"
                />
                <span className="text-ink">Включить адаптивную настройку</span>
              </label>
              {adaptiveEnabled && gainFactor != null && (
                <div className="flex items-center gap-3 text-[10.5px]">
                  <span className="text-dim">Текущий множитель:</span>
                  <span className="font-mono font-semibold text-accent">{gainFactor.toFixed(3)}</span>
                  <span className="text-dim">Kp×{gainFactor.toFixed(2)} Ki×{gainFactor.toFixed(2)}</span>
                </div>
              )}
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="text-[9.5px] text-dim">Тег индикатора</label>
                  <input
                    value={loop?.gain_indicator_tag ?? adaptiveDraft.indicator_tag}
                    onChange={(e) => setAdaptiveDraft({ ...adaptiveDraft, indicator_tag: e.target.value })}
                    placeholder="plant.crushing.fi101"
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
                <div>
                  <label className="text-[9.5px] text-dim">Gain Low</label>
                  <input
                    type="number"
                    value={loop?.gain_low ?? adaptiveDraft.gain_low}
                    onChange={(e) => setAdaptiveDraft({ ...adaptiveDraft, gain_low: parseFloat(e.target.value) || 0 })}
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
                <div>
                  <label className="text-[9.5px] text-dim">Gain High</label>
                  <input
                    type="number"
                    value={loop?.gain_high ?? adaptiveDraft.gain_high}
                    onChange={(e) => setAdaptiveDraft({ ...adaptiveDraft, gain_high: parseFloat(e.target.value) || 100 })}
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
              </div>
              <p className="text-[9px] leading-relaxed text-dim">
                Диапазон [Gain Low, Gain High] нормализует показание индикатора в множитель ×0.5..×1.5.
                При выходе за диапазон — clamp ×0.25..×4.0. Коэффициенты Kp/Ki пересчитываются каждый тик без переинициализации петли.
              </p>
              {adaptiveEnabled && (
                <button
                  onClick={() => void write('adaptive', {
                    adaptive_enabled: false,
                    gain_indicator_tag: loop?.gain_indicator_tag || '',
                    gain_low: loop?.gain_low ?? 50,
                    gain_high: loop?.gain_high ?? 200,
                  })}
                  className="h-6 rounded border border-alarm/40 bg-alarm/10 px-2 text-[10px] font-semibold text-alarm hover:bg-alarm/20"
                >
                  Отключить adaptive
                </button>
              )}
            </div>
          )}
        </div>
      )}

      {/* pH regulation panel (patent claim 5) */}
      {!compact && isPH && (
        <div className="mt-2 border-t border-line pt-2">
          <button
            onClick={() => setShowPH(!showPH)}
            className="text-[10px] text-dim hover:text-mute transition-colors"
          >
            {showPH ? '▼ Скрыть' : '▶'} pH-регуляция (Claim 5)
          </button>
          {showPH && (
            <div className="mt-2 space-y-2 rounded bg-panel2/50 p-2.5">
              {/* Dead-band indicators */}
              <div className="flex items-center gap-3 text-[10.5px]">
                <span className="text-dim">Dead-bands:</span>
                <span className="rounded bg-warn/20 px-1.5 py-0.5 text-warn font-mono text-[9px]">
                  ±{phWarning.toFixed(1)} предупреждение
                </span>
                <span className="rounded bg-alarm/20 px-1.5 py-0.5 text-alarm font-mono text-[9px]">
                  ±{phCritical.toFixed(1)} критическое
                </span>
              </div>
              {pv !== undefined && (
                <div className="flex items-center gap-3 text-[10.5px]">
                  <span className="text-dim">Отклонение от SP:</span>
                  <span className={`font-mono font-semibold ${
                    Math.abs(pv - sp) > phCritical ? 'text-alarm' :
                    Math.abs(pv - sp) > phWarning ? 'text-warn' : 'text-ok'
                  }`}>
                    {Math.abs(pv - sp).toFixed(2)} pH
                  </span>
                </div>
              )}

              {/* Self-tuning */}
              <label className="flex items-center gap-2 text-[11px]">
                <input
                  type="checkbox"
                  checked={selfTuning}
                  onChange={(e) => void write('ph', {
                    deadband_warning: phWarning,
                    deadband_critical: phCritical,
                    self_tuning_enabled: e.target.checked,
                    temperature_tag: loop?.temperature_tag || '',
                    flow_tag: loop?.flow_tag || '',
                    kp_temp_factor: loop?.kp_temp_factor ?? 0.02,
                    ki_flow_factor: loop?.ki_flow_factor ?? 0.01,
                  })}
                  className="h-3.5 w-3.5 rounded accent-accent"
                />
                <span className="text-ink">Self-tuning Kp/Ki (температура + расход)</span>
              </label>
              {selfTuning && (
                <div className="grid grid-cols-2 gap-2 text-[9.5px] text-dim">
                  <span>Тег температуры: <code className="font-mono">{loop?.temperature_tag || '—'}</code></span>
                  <span>Тег расхода: <code className="font-mono">{loop?.flow_tag || '—'}</code></span>
                  <span>Kp/°C: <code className="font-mono">{loop?.kp_temp_factor ?? 0.02}</code></span>
                  <span>Ki/flow: <code className="font-mono">{loop?.ki_flow_factor ?? 0.01}</code></span>
                </div>
              )}

              {/* Config form */}
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="text-[9.5px] text-dim">Warning ±pH</label>
                  <input
                    type="number" step="0.1"
                    value={phWarning}
                    onChange={(e) => setPHDraft({ ...phDraft, warning: parseFloat(e.target.value) || 0.2 })}
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
                <div>
                  <label className="text-[9.5px] text-dim">Critical ±pH</label>
                  <input
                    type="number" step="0.1"
                    value={phCritical}
                    onChange={(e) => setPHDraft({ ...phDraft, critical: parseFloat(e.target.value) || 0.5 })}
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
                <div>
                  <label className="text-[9.5px] text-dim">Kp/°C</label>
                  <input
                    type="number" step="0.001"
                    value={loop?.kp_temp_factor ?? 0.02}
                    onChange={(e) => setPHDraft({ ...phDraft, kpTemp: parseFloat(e.target.value) || 0.02 })}
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
              </div>
              <div className="grid grid-cols-3 gap-2">
                <div>
                  <label className="text-[9.5px] text-dim">Ki/flow</label>
                  <input
                    type="number" step="0.001"
                    value={loop?.ki_flow_factor ?? 0.01}
                    onChange={(e) => setPHDraft({ ...phDraft, kiFlow: parseFloat(e.target.value) || 0.01 })}
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
                <div>
                  <label className="text-[9.5px] text-dim">Тег температуры</label>
                  <input
                    value={loop?.temperature_tag || ''}
                    onChange={(e) => setPHDraft({ ...phDraft, tempTag: e.target.value })}
                    placeholder="plant.flotation.ti301"
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
                <div>
                  <label className="text-[9.5px] text-dim">Тег расхода</label>
                  <input
                    value={loop?.flow_tag || ''}
                    onChange={(e) => setPHDraft({ ...phDraft, flowTag: e.target.value })}
                    placeholder="plant.flotation.fi301"
                    className="mt-0.5 h-6 w-full rounded border border-line bg-base px-1.5 font-mono text-[10.5px] text-ink outline-none focus:border-accent/60"
                  />
                </div>
              </div>
              <p className="text-[9px] leading-relaxed text-dim">
                Warning ±0.2 pH → alarm. Critical ±0.5 pH → force manual + freeze output.
                Self-tuning: Kp масштабируется с температурой, Ki — с расходом пульпы.
              </p>
              <button
                onClick={() => void write('ph', {
                  deadband_warning: phDraft.warning,
                  deadband_critical: phDraft.critical,
                  self_tuning_enabled: selfTuning,
                  temperature_tag: phDraft.tempTag,
                  flow_tag: phDraft.flowTag,
                  kp_temp_factor: phDraft.kpTemp,
                  ki_flow_factor: phDraft.kiFlow,
                })}
                className="h-6 rounded bg-accent px-2 text-[10px] font-semibold text-white hover:opacity-90"
              >
                Сохранить конфигурацию pH
              </button>
            </div>
          )}
        </div>
      )}
    </div>
  );
};

export default LoopFaceplate;
