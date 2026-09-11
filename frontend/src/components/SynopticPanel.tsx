import React, { useMemo, useState } from 'react';
import { useLiveMap } from '../ws/WsProvider';
import { useRegistry } from '../registry/RegistryProvider';
import Faceplate from './Faceplate';

interface NodeSpec {
  asset: string;
  x: number;
  y: number;
  w: number;
  h: number;
  label: string;
  metrics: { tag: string; unit?: boolean }[];
}

// Vertical flowsheet (TZ §5): bin → crusher → mill+cyclone → flotation →
// thickener → filter → shipping, with the tails branch. Values stream live;
// clicking a node opens its faceplate.
const NODES: NodeSpec[] = [
  {
    asset: 'plant.crushing', x: 60, y: 30, w: 240, h: 88, label: 'Дробление',
    metrics: [{ tag: 'plant.crushing.fi101' }, { tag: 'plant.crushing.si101' }],
  },
  {
    asset: 'plant.grinding', x: 60, y: 168, w: 240, h: 112, label: 'Измельчение',
    metrics: [
      { tag: 'plant.grinding.ei201' },
      { tag: 'plant.grinding.xi201' },
      { tag: 'plant.grinding.pi201' },
    ],
  },
  {
    asset: 'plant.flotation', x: 60, y: 330, w: 240, h: 128, label: 'Флотация',
    metrics: [
      { tag: 'plant.flotation.li301' },
      { tag: 'plant.flotation.ai301' },
      { tag: 'plant.flotation.aft301' },
      { tag: 'plant.flotation.qi301' },
    ],
  },
  {
    asset: 'plant.thickening', x: 60, y: 508, w: 240, h: 100, label: 'Сгущение',
    metrics: [{ tag: 'plant.thickening.li401' }, { tag: 'plant.thickening.di401' }],
  },
  {
    asset: 'plant.filtration', x: 60, y: 658, w: 240, h: 100, label: 'Фильтрация',
    metrics: [{ tag: 'plant.filtration.mi501' }, { tag: 'plant.filtration.wi501' }],
  },
];

const SynopticPanel: React.FC = () => {
  const live = useLiveMap();
  const { tagLabel, assets } = useRegistry();
  const [openAsset, setOpenAsset] = useState<string | null>(null);

  const assetAlarm = useMemo(() => {
    // A node shows red while any of its tags carries an alarm-quality feed
    // item; offline detection from live data quality.
    return (assetId: string) => {
      const hasData = NODES.find((n) => n.asset === assetId)?.metrics.some((m) => live.has(m.tag));
      return hasData ? 'ok' : 'offline';
    };
  }, [live]);

  const valueOf = (tagId: string): { text: string; quality: string } => {
    const point = live.get(tagId);
    if (!point) return { text: '—', quality: 'offline' };
    const digits = Math.abs(point.value) < 1 ? 3 : point.value < 20 ? 2 : 1;
    return { text: point.value.toFixed(digits), quality: point.quality };
  };

  const assetName = (id: string) => assets.find((a) => a.id === id)?.name ?? id;

  const renderNode = (node: NodeSpec) => {
    const status = assetAlarm(node.asset);
    const stroke = status === 'offline' ? 'stroke-warn' : 'stroke-line';
    return (
      <g key={node.asset} onClick={() => setOpenAsset(node.asset)} className="cursor-pointer">
        <rect
          x={node.x} y={node.y} width={node.w} height={node.h} rx={8}
          className={`fill-panel ${stroke}`}
          strokeWidth={1.5}
        />
        <title>{assetName(node.asset)}</title>
        <text x={node.x + 12} y={node.y + 20} className="fill-mute" fontSize={11}>
          {node.label}
        </text>
        {status === 'offline' && (
          <text x={node.x + node.w - 12} y={node.y + 20} textAnchor="end" className="fill-warn" fontSize={10}>
            нет связи
          </text>
        )}
        {node.metrics.map((m, i) => {
          const v = valueOf(m.tag);
          const label = tagLabel(m.tag);
          const clipped = label.length > 22 ? label.slice(0, 21) + '…' : label;
          return (
            <g key={m.tag}>
              <text x={node.x + 12} y={node.y + 42 + i * 20} className="fill-dim" fontSize={9.5}>
                {clipped}
              </text>
              <text
                x={node.x + node.w - 12}
                y={node.y + 42 + i * 20}
                textAnchor="end"
                fontSize={12}
                className={`num font-semibold ${
                  v.quality === 'good' ? 'fill-ink' : 'fill-warn'
                }`}
              >
                {v.text}
                {live.get(m.tag)?.unit ? <tspan className="fill-dim" fontSize={9}> {live.get(m.tag)!.unit}</tspan> : null}
              </text>
            </g>
          );
        })}
      </g>
    );
  };

  const pipe = (x: number, y1: number, y2: number) => (
    <g>
      <line x1={x} y1={y1} x2={x} y2={y2} strokeWidth={5} className="stroke-line" strokeLinecap="round" />
      <polygon points={`${x - 5},${y2 - 9} ${x + 5},${y2 - 9} ${x},${y2}`} className="fill-dim" />
    </g>
  );

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-[13px] font-semibold uppercase tracking-[0.07em] text-mute">
        Технологическая схема участка — {assets.length ? 'живые данные' : 'реестр загружается…'}
      </h2>
      <div className="rounded-lg border border-line bg-panel p-2">
        <svg viewBox="0 0 760 800" className="mx-auto block h-auto w-full max-w-[860px]">
          {/* Main process column */}
          {pipe(180, 118, 168)}
          {pipe(180, 280, 330)}
          {pipe(180, 458, 508)}
          {pipe(180, 608, 658)}

          {/* Concentrate branch: flotation → shipping (right) */}
          <g>
            <path d="M 300 390 H 560 V 700 H 310" fill="none" strokeWidth={3} className="stroke-ok/60" strokeDasharray="1 0" />
            <polygon points="300,390 310,385 310,395" className="fill-ok" />
            <text x={430} y={380} textAnchor="middle" className="fill-ok" fontSize={10.5}>
              концентрат
            </text>
            <text x={560} y={730} textAnchor="middle" fontSize={12.5} className="num font-semibold fill-ink">
              {live.has('plant.flotation.wi301') ? live.get('plant.flotation.wi301')!.value.toFixed(2) : '—'} т/ч
            </text>
            <text x={640} y={730} textAnchor="middle" className="fill-dim" fontSize={10}>
              отгрузка
            </text>
          </g>

          {/* Tails branch: flotation → tailings (left) */}
          <g>
            <path d="M 60 420 H 26 V 730" fill="none" strokeWidth={3} className="stroke-warn/60" />
            <text x={30} y={380} className="fill-warn" fontSize={10.5}>
              хвосты
            </text>
            <text x={26} y={752} textAnchor="middle" fontSize={11.5} className="num fill-dim">
              {live.has('plant.flotation.wi302') ? live.get('plant.flotation.wi302')!.value.toFixed(1) : '—'} т/ч
            </text>
          </g>

          {/* Cyclone loop: grinding → cyclone → back to mill */}
          <g>
            <circle cx={430} cy={224} r={30} className="fill-panel2 stroke-line" strokeWidth={1.5} />
            <text x={430} y={228} textAnchor="middle" className="fill-mute" fontSize={10}>
              гидроциклон
            </text>
            <path d="M 300 200 H 398" fill="none" strokeWidth={3} className="stroke-line" />
            <path d="M 430 194 V 150 H 300" fill="none" strokeWidth={3} className="stroke-warn/50" strokeDasharray="6 4" />
            <text x={360} y={142} textAnchor="middle" className="fill-warn" fontSize={9.5}>
              пески (циркуляция)
            </text>
          </g>

          {NODES.map(renderNode)}
        </svg>
      </div>
      <p className="text-[11px] text-dim">
        Клик по узлу — паспорт объекта (faceplate) с параметрами и контурами управления.
      </p>

      {openAsset && <Faceplate assetId={openAsset} onClose={() => setOpenAsset(null)} />}
    </section>
  );
};

export default SynopticPanel;
