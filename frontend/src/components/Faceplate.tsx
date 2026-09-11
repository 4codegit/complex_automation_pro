import React, { useEffect } from 'react';
import { useLiveMap, useLoopStates } from '../ws/WsProvider';
import { useRegistry } from '../registry/RegistryProvider';
import LoopFaceplate from './LoopFaceplate';

interface FaceplateProps {
  assetId: string;
  onClose: () => void;
}

const Faceplate: React.FC<FaceplateProps> = ({ assetId, onClose }) => {
  const { assets, tagsByAsset, tagLabel } = useRegistry();
  const live = useLiveMap();
  const loops = useLoopStates();

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onClose]);

  const asset = assets.find((a) => a.id === assetId);
  const tags = tagsByAsset(assetId).filter((t) => t.direction === 'input');
  const assetLoops = [...loops.values()].filter((l) => l.pv_tag.startsWith(assetId + '.'));

  const fmt = (v: number) => (Math.abs(v) < 1 ? v.toFixed(3) : v < 20 ? v.toFixed(2) : v.toFixed(1));

  return (
    <div
      className="fixed inset-0 z-40 flex items-center justify-center bg-black/40 p-6"
      onClick={onClose}
    >
      <div
        className="max-h-[90vh] w-[560px] overflow-y-auto rounded-lg border border-line bg-panel p-4 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-3 flex items-start justify-between">
          <div>
            <h3 className="text-[14px] font-bold text-ink">{asset?.name ?? assetId}</h3>
            <p className="text-[11px] text-dim">
              {asset?.area} · критичность: {asset?.criticality} · {assetId}
            </p>
          </div>
          <button
            onClick={onClose}
            className="rounded border border-line px-2 py-0.5 text-[12px] text-mute hover:text-ink"
          >
            ✕
          </button>
        </div>

        <table className="w-full text-[12px]">
          <thead>
            <tr className="border-b border-line text-left text-[10px] uppercase tracking-wide text-dim">
              <th className="py-1.5">Параметр</th>
              <th className="py-1.5 text-right">Значение</th>
              <th className="py-1.5 text-right">Качество</th>
            </tr>
          </thead>
          <tbody>
            {tags.map((t) => {
              const point = live.get(t.id);
              const quality = point?.quality ?? 'нет данных';
              const qColor =
                quality === 'good' ? 'text-ok' : quality === 'нет данных' ? 'text-dim' : 'text-warn';
              return (
                <tr key={t.id} className="border-b border-line/60">
                  <td className="py-1.5 text-ink">{t.name}</td>
                  <td className="num py-1.5 text-right font-mono font-semibold text-ink">
                    {point ? fmt(point.value) : '—'} <span className="text-[10px] text-dim">{t.unit}</span>
                  </td>
                  <td className={`py-1.5 text-right text-[10.5px] ${qColor}`}>{quality}</td>
                </tr>
              );
            })}
          </tbody>
        </table>

        {assetLoops.length > 0 && (
          <div className="mt-4 space-y-3">
            {assetLoops.map((l) => (
              <LoopFaceplate key={l.loop_id} loopId={l.loop_id} compact />
            ))}
          </div>
        )}

        <p className="mt-3 text-[10px] text-dim">
          Уставки сигнализации — в разделе «Тревоги» (ISA-18.2, рационализированные пределы).
        </p>
      </div>
    </div>
  );
};

export default Faceplate;
