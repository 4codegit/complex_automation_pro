import React, { useCallback, useEffect, useMemo, useState } from 'react';

type Role = {
  role: string;
  label: string;
  permissions: string[];
};

type Asset = {
  id: string;
  name: string;
  area: string;
  criticality: string;
  active: boolean;
};

type Tag = {
  id: string;
  asset_id: string;
  name: string;
  unit: string;
  data_type?: string;
  sampling_interval_seconds?: number;
  active: boolean;
};

type TelemetryRecord = {
  message_id: string;
  gateway_id: string;
  source_sequence: number | null;
  observed_at: string;
  received_at: string;
  asset_id: string;
  tag_id: string;
  value: number;
  unit: string;
  quality: string;
  profile_id: string | null;
};

type AdminTab = 'registry' | 'sensors' | 'telemetry' | 'access';
type RegistryState = 'loading' | 'ready' | 'error';
type TelemetryState = 'idle' | 'loading' | 'ready' | 'error';

const API_URL = import.meta.env.VITE_API_URL ?? `http://${window.location.hostname}:8000/api/v1`;

// Sensor type presets: picking one prefills the unit (and documents the
// typical instrument family) so an operator registers a "temperature sensor",
// not a bare tag id.
const SENSOR_TYPES: { id: string; label: string; unit: string; hint: string }[] = [
  { id: 'temperature', label: 'Температура', unit: 'C', hint: 'термопара, RTD' },
  { id: 'ph', label: 'pH (кислотность)', unit: 'pH', hint: 'pH-электрод' },
  { id: 'density', label: 'Плотность', unit: 'g/cm3', hint: 'плотномер' },
  { id: 'level', label: 'Уровень', unit: '%', hint: 'радарный / ультразвук' },
  { id: 'moisture', label: 'Влажность', unit: '%', hint: 'NIR / микроволновый' },
  { id: 'massflow', label: 'Массовый расход', unit: 't/h', hint: 'конвейерные весы' },
  { id: 'volumeflow', label: 'Объёмный расход', unit: 'm3/h', hint: 'электромагнитный' },
  { id: 'pressure', label: 'Давление', unit: 'bar', hint: 'тензодатчик' },
  { id: 'vibration', label: 'Вибрация', unit: 'mm/s', hint: 'акселерометр' },
  { id: 'current', label: 'Ток', unit: 'A', hint: 'трансформатор тока' },
  { id: 'custom', label: 'Свой тип…', unit: '', hint: '' },
];

const QUALITY_LABELS: Record<string, string> = {
  good: 'Годные',
  uncertain: 'Неопределённые',
  bad: 'Негодные',
  stale: 'Устаревшие',
  substituted: 'Замещённые',
  offline: 'Нет связи',
};

const QUALITY_STYLES: Record<string, string> = {
  good: 'text-ok',
  uncertain: 'text-warn',
  bad: 'text-alarm',
  stale: 'text-warn',
  substituted: 'text-accent',
  offline: 'text-alarm',
};

function formatNumber(value: number): string {
  return new Intl.NumberFormat('ru-RU', { maximumFractionDigits: 3 }).format(value);
}

function formatTimestamp(value: string): string {
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString('ru-RU', {
    day: '2-digit',
    month: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  });
}

async function getJson<T>(path: string): Promise<T> {
  const response = await fetch(`${API_URL}${path}`, {
    headers: { Accept: 'application/json' },
  });

  if (!response.ok) {
    throw new Error(`Request failed with status ${response.status}`);
  }

  return response.json() as Promise<T>;
}

async function sendJson(path: string, method: 'POST' | 'PUT' | 'DELETE', body: unknown, actor: string): Promise<void> {
  const response = await fetch(`${API_URL}${path}`, {
    method,
    headers: { 'Content-Type': 'application/json', Accept: 'application/json', 'X-User': actor },
    body: method === 'DELETE' && body === undefined ? undefined : JSON.stringify(body),
  });

  if (!response.ok) {
    let detail = `status ${response.status}`;
    try {
      const problem = (await response.json()) as { message?: string };
      if (problem.message) detail = problem.message;
    } catch {
      // keep the status-only detail
    }
    throw new Error(detail);
  }
}

const AdminPanel: React.FC = () => {
  const [roles, setRoles] = useState<Role[]>([]);
  const [assets, setAssets] = useState<Asset[]>([]);
  const [tags, setTags] = useState<Tag[]>([]);
  const [registryState, setRegistryState] = useState<RegistryState>('loading');
  const [lastUpdated, setLastUpdated] = useState<string | null>(null);
  const [activeTab, setActiveTab] = useState<AdminTab>('registry');
  const [assetSearch, setAssetSearch] = useState('');
  const [tagSearch, setTagSearch] = useState('');
  const [selectedAssetId, setSelectedAssetId] = useState('');
  const [selectedTagId, setSelectedTagId] = useState('');
  const [selectedRoleId, setSelectedRoleId] = useState('');
  const [historyHours, setHistoryHours] = useState('8');
  const [telemetry, setTelemetry] = useState<TelemetryRecord[]>([]);
  const [telemetryState, setTelemetryState] = useState<TelemetryState>('idle');
  // Sensors tab: actor for the audit trail + add-form state.
  const [actor, setActor] = useState('admin');
  const [newAsset, setNewAsset] = useState({ id: '', name: '', area: '', criticality: 'M' });
  const [newTag, setNewTag] = useState({ assetId: '', id: '', name: '', unit: '', sensorType: 'temperature' });
  const [mutationState, setMutationState] = useState<'idle' | 'busy'>('idle');
  const [mutationNote, setMutationNote] = useState<{ ok: boolean; text: string } | null>(null);

  const loadRegistry = useCallback(async () => {
    setRegistryState('loading');

    try {
      const [nextRoles, nextAssets, nextTags] = await Promise.all([
        getJson<Role[]>('/access/roles'),
        getJson<Asset[]>('/assets'),
        getJson<Tag[]>('/tags'),
      ]);

      setRoles(nextRoles);
      setAssets(nextAssets);
      setTags(nextTags);
      setSelectedRoleId((current) => (
        nextRoles.some((role) => role.role === current) ? current : (nextRoles[0]?.role ?? '')
      ));
      setSelectedAssetId((current) => (
        nextAssets.some((asset) => asset.id === current) ? current : ''
      ));
      setSelectedTagId((current) => (
        nextTags.some((tag) => tag.id === current) ? current : ''
      ));
      setLastUpdated(new Date().toISOString());
      setRegistryState('ready');
    } catch {
      setRegistryState('error');
    }
  }, []);

  useEffect(() => {
    void loadRegistry();
  }, [loadRegistry]);

  const createAsset = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMutationState('busy');
    setMutationNote(null);
    try {
      await sendJson('/assets', 'POST', {
        id: newAsset.id.trim(),
        name: newAsset.name.trim(),
        area: newAsset.area.trim(),
        criticality: newAsset.criticality,
        active: true,
      }, actor.trim());
      setMutationNote({ ok: true, text: `Участок «${newAsset.id.trim()}» добавлен в реестр` });
      setNewAsset({ id: '', name: '', area: '', criticality: 'M' });
      await loadRegistry();
    } catch (err) {
      setMutationNote({ ok: false, text: err instanceof Error ? err.message : 'Ошибка запроса' });
    } finally {
      setMutationState('idle');
    }
  };

  const createTag = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setMutationState('busy');
    setMutationNote(null);
    try {
      const tagID = newTag.id.trim();
      const assetID = newTag.assetId;
      if (!tagID.startsWith(`${assetID}.`)) {
        throw new Error(`Тег должен начинаться с префикса участка: ${assetID}.<метрика>`);
      }
      await sendJson('/tags', 'POST', {
        id: tagID,
        asset_id: assetID,
        name: newTag.name.trim() || tagID,
        unit: newTag.unit.trim(),
        data_type: 'number',
        active: true,
      }, actor.trim());
      setMutationNote({ ok: true, text: `Сигнал «${tagID}» зарегистрирован` });
      setNewTag((current) => ({ ...current, id: '', name: '', unit: '' }));
      await loadRegistry();
    } catch (err) {
      setMutationNote({ ok: false, text: err instanceof Error ? err.message : 'Ошибка запроса' });
    } finally {
      setMutationState('idle');
    }
  };

  const assetById = useMemo(
    () => new Map(assets.map((asset) => [asset.id, asset])),
    [assets],
  );

  const visibleAssets = useMemo(() => {
    const query = assetSearch.trim().toLocaleLowerCase();
    if (!query) return assets;
    return assets.filter((asset) => (
      `${asset.name} ${asset.area} ${asset.id}`.toLocaleLowerCase().includes(query)
    ));
  }, [assetSearch, assets]);

  const visibleTags = useMemo(() => {
    const query = tagSearch.trim().toLocaleLowerCase();
    return tags.filter((tag) => {
      const matchesAsset = !selectedAssetId || tag.asset_id === selectedAssetId;
      const matchesQuery = !query || `${tag.name} ${tag.id} ${tag.unit}`.toLocaleLowerCase().includes(query);
      return matchesAsset && matchesQuery;
    });
  }, [selectedAssetId, tagSearch, tags]);

  const selectedRole = roles.find((role) => role.role === selectedRoleId) ?? null;
  const selectedAsset = assetById.get(selectedAssetId) ?? null;
  const selectedTag = tags.find((tag) => tag.id === selectedTagId) ?? null;
  const latestReading = telemetry[0] ?? null;

  const qualityCounts = useMemo(() => telemetry.reduce<Record<string, number>>((counts, reading) => {
    counts[reading.quality] = (counts[reading.quality] ?? 0) + 1;
    return counts;
  }, {}), [telemetry]);

  const selectAsset = (assetId: string) => {
    setSelectedAssetId(assetId);
    setSelectedTagId((currentTagId) => {
      const candidateTags = tags.filter((tag) => tag.asset_id === assetId);
      return candidateTags.some((tag) => tag.id === currentTagId) ? currentTagId : (candidateTags[0]?.id ?? '');
    });
  };

  const selectTag = (tagId: string) => {
    const tag = tags.find((candidate) => candidate.id === tagId);
    setSelectedTagId(tagId);
    if (tag) setSelectedAssetId(tag.asset_id);
  };

  const loadTelemetry = async (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setTelemetryState('loading');

    const params = new URLSearchParams({ limit: '120' });
    if (selectedAssetId) params.set('asset_id', selectedAssetId);
    if (selectedTagId) params.set('tag_id', selectedTagId);
    params.set('from_time', new Date(Date.now() - Number(historyHours) * 60 * 60 * 1000).toISOString());

    try {
      const records = await getJson<TelemetryRecord[]>(`/telemetry?${params.toString()}`);
      setTelemetry(records);
      setTelemetryState('ready');
    } catch {
      setTelemetryState('error');
    }
  };

  const registryStatus = registryState === 'ready'
    ? 'Реестр синхронизирован'
    : registryState === 'loading'
      ? 'Синхронизация реестра'
      : 'Реестр недоступен';

  const registryStatusClass = registryState === 'ready'
    ? 'text-ok'
    : registryState === 'loading'
      ? 'text-warn'
      : 'text-alarm';

  return (
    <section id="administration" className="rounded-lg border border-line bg-panel2 p-5">
      <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold text-white">Администрирование платформы</h2>
          <p className="mt-1 text-xs text-dim">Реестр фабрики, доступы и проверка принятых измерений</p>
        </div>
        <div className="flex items-center gap-3">
          <span className={`text-xs ${registryStatusClass}`} role="status">{registryStatus}</span>
          <button
            type="button"
            onClick={() => void loadRegistry()}
            className="grid h-8 w-8 place-items-center rounded border border-line text-base text-ink transition-colors hover:border-accent/30 hover:text-accent disabled:cursor-wait disabled:opacity-50"
            aria-label="Обновить реестр"
            title="Обновить реестр"
            disabled={registryState === 'loading'}
          >
            ↻
          </button>
        </div>
      </div>

      <dl className="mb-5 grid grid-cols-2 border-y border-line py-3 sm:grid-cols-4">
        <div className="border-r border-line px-3 first:pl-0">
          <dt className="text-[11px] uppercase tracking-wide text-dim">Участки</dt>
          <dd className="mt-1 text-lg font-semibold text-ink">{assets.length}</dd>
        </div>
        <div className="border-r border-line px-3">
          <dt className="text-[11px] uppercase tracking-wide text-dim">Сигналы</dt>
          <dd className="mt-1 text-lg font-semibold text-ink">{tags.length}</dd>
        </div>
        <div className="border-r border-line px-3">
          <dt className="text-[11px] uppercase tracking-wide text-dim">Активные</dt>
          <dd className="mt-1 text-lg font-semibold text-ok">{tags.filter((tag) => tag.active).length}</dd>
        </div>
        <div className="px-3 last:pr-0">
          <dt className="text-[11px] uppercase tracking-wide text-dim">Роли</dt>
          <dd className="mt-1 text-lg font-semibold text-ink">{roles.length}</dd>
        </div>
      </dl>

      <div className="mb-5 flex gap-1 border-b border-line" role="tablist" aria-label="Администрирование">
        {([
          ['registry', 'Реестр'],
          ['sensors', 'Датчики'],
          ['telemetry', 'Данные'],
          ['access', 'Уровни доступа'],
        ] as const).map(([tab, label]) => (
          <button
            key={tab}
            type="button"
            role="tab"
            aria-selected={activeTab === tab}
            onClick={() => setActiveTab(tab)}
            className={`border-b-2 px-3 py-2 text-sm transition-colors ${
              activeTab === tab
                ? 'border-cyan-400 text-accent'
                : 'border-transparent text-dim hover:text-ink'
            }`}
          >
            {label}
          </button>
        ))}
      </div>

      {activeTab === 'registry' && (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.7fr)]">
          <div>
            <div className="mb-3 flex items-center justify-between gap-3">
              <h3 className="text-xs font-semibold uppercase tracking-wide text-mute">Оборудование</h3>
              <span className="text-xs text-dim">{visibleAssets.length}</span>
            </div>
            <input
              value={assetSearch}
              onChange={(event) => setAssetSearch(event.target.value)}
              className="mb-3 h-9 w-full rounded border border-line bg-base px-3 text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
              placeholder="Поиск участка или оборудования"
              aria-label="Поиск оборудования"
            />
            <div className="max-h-72 divide-y divide-line overflow-y-auto border-y border-line">
              {visibleAssets.map((asset) => {
                const isSelected = selectedAssetId === asset.id;
                return (
                  <button
                    key={asset.id}
                    type="button"
                    onClick={() => selectAsset(asset.id)}
                    className={`flex w-full items-center justify-between gap-3 px-2 py-3 text-left transition-colors ${
                      isSelected ? 'bg-accent/10' : 'hover:bg-panel2/60'
                    }`}
                  >
                    <span className="min-w-0">
                      <span className="block truncate text-sm text-ink">{asset.name}</span>
                      <span className="block truncate text-xs text-dim">{asset.area} · {asset.id}</span>
                    </span>
                    <span className={`shrink-0 text-[11px] ${asset.active ? 'text-ok' : 'text-dim'}`}>
                      {asset.active ? asset.criticality : 'inactive'}
                    </span>
                  </button>
                );
              })}
              {!visibleAssets.length && (
                <p className="px-2 py-4 text-sm text-dim">Совпадений нет</p>
              )}
            </div>
          </div>

          <div>
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-xs font-semibold uppercase tracking-wide text-mute">Сигнальный реестр</h3>
                <p className="mt-1 text-xs text-dim">{selectedAsset ? selectedAsset.name : 'Все участки'}</p>
              </div>
              {selectedAssetId && (
                <button
                  type="button"
                  onClick={() => selectAsset('')}
                  className="text-xs text-accent transition-colors hover:text-ink"
                >
                  Все участки
                </button>
              )}
            </div>
            <input
              value={tagSearch}
              onChange={(event) => setTagSearch(event.target.value)}
              className="mb-3 h-9 w-full rounded border border-line bg-base px-3 text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
              placeholder="Поиск тега или единицы"
              aria-label="Поиск сигнала"
            />
            <div className="max-h-72 overflow-auto border-y border-line">
              <table className="min-w-full text-left text-sm">
                <thead className="sticky top-0 bg-panel2 text-[11px] uppercase tracking-wide text-dim">
                  <tr>
                    <th className="px-2 py-2 font-medium">Сигнал</th>
                    <th className="px-2 py-2 font-medium">Ед.</th>
                    <th className="px-2 py-2 font-medium">Интервал</th>
                    <th className="px-2 py-2 font-medium">Статус</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-line">
                  {visibleTags.map((tag) => (
                    <tr
                      key={tag.id}
                      onClick={() => selectTag(tag.id)}
                      className={`cursor-pointer transition-colors hover:bg-panel2/60 ${selectedTagId === tag.id ? 'bg-accent/10' : ''}`}
                    >
                      <td className="max-w-[14rem] px-2 py-2.5">
                        <span className="block truncate text-ink" title={tag.name}>{tag.name}</span>
                        <span className="block truncate font-mono text-[11px] text-dim" title={tag.id}>{tag.id}</span>
                      </td>
                      <td className="whitespace-nowrap px-2 py-2.5 text-mute">{tag.unit}</td>
                      <td className="whitespace-nowrap px-2 py-2.5 text-mute">
                        {tag.sampling_interval_seconds ?? '—'}{tag.sampling_interval_seconds !== undefined ? ' s' : ''}
                      </td>
                      <td className={`whitespace-nowrap px-2 py-2.5 text-xs ${tag.active ? 'text-ok' : 'text-dim'}`}>
                        {tag.active ? 'Активен' : 'Неактивен'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {!visibleTags.length && (
                <p className="px-2 py-4 text-sm text-dim">Зарегистрированных сигналов нет</p>
              )}
            </div>
          </div>
        </div>
      )}

      {activeTab === 'sensors' && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={createAsset} className="rounded border border-line bg-panel2/40 p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-mute">Новый участок (asset)</h3>
            <div className="mt-3 grid gap-3">
              <label className="text-sm text-ink">
                Идентификатор
                <input
                  required
                  value={newAsset.id}
                  onChange={(event) => setNewAsset({ ...newAsset, id: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-line bg-base px-3 font-mono text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
                  placeholder="plant-a.roasting"
                />
              </label>
              <label className="text-sm text-ink">
                Название
                <input
                  required
                  value={newAsset.name}
                  onChange={(event) => setNewAsset({ ...newAsset, name: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-line bg-base px-3 text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
                  placeholder="Обжиг и кальцинация"
                />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label className="text-sm text-ink">
                  Участок (area)
                  <input
                    required
                    value={newAsset.area}
                    onChange={(event) => setNewAsset({ ...newAsset, area: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-line bg-base px-3 text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
                    placeholder="Roasting"
                  />
                </label>
                <label className="text-sm text-ink">
                  Критичность
                  <select
                    value={newAsset.criticality}
                    onChange={(event) => setNewAsset({ ...newAsset, criticality: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-line bg-base px-2 text-sm text-ink outline-none focus:border-accent/60"
                  >
                    <option value="H">H — высокая</option>
                    <option value="M">M — средняя</option>
                    <option value="L">L — низкая</option>
                  </select>
                </label>
              </div>
            </div>
            <button
              type="submit"
              disabled={mutationState === 'busy'}
              className="mt-4 h-9 w-full rounded bg-cyan-600 px-4 text-sm font-medium text-white transition-colors hover:bg-sky-300 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Добавить участок
            </button>
          </form>

          <form onSubmit={createTag} className="rounded border border-line bg-panel2/40 p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-mute">Новый сигнал (тег)</h3>
            <div className="mt-3 grid gap-3">
              <label className="text-sm text-ink">
                Участок
                <select
                  required
                  value={newTag.assetId}
                  onChange={(event) => setNewTag({ ...newTag, assetId: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-line bg-base px-2 text-sm text-ink outline-none focus:border-accent/60"
                >
                  <option value="">— выберите —</option>
                  {assets.filter((asset) => asset.active).map((asset) => (
                    <option key={asset.id} value={asset.id}>{asset.name} ({asset.id})</option>
                  ))}
                </select>
              </label>
              <label className="text-sm text-ink">
                Тип датчика
                <select
                  value={newTag.sensorType}
                  onChange={(event) => {
                    const preset = SENSOR_TYPES.find((t) => t.id === event.target.value);
                    setNewTag({
                      ...newTag,
                      sensorType: event.target.value,
                      unit: preset && preset.unit ? preset.unit : newTag.unit,
                      name: newTag.name || (preset && preset.id !== 'custom' ? preset.label : newTag.name),
                    });
                  }}
                  className="mt-1 h-9 w-full rounded border border-line bg-base px-2 text-sm text-ink outline-none focus:border-accent/60"
                >
                  {SENSOR_TYPES.map((t) => (
                    <option key={t.id} value={t.id}>{t.label}{t.unit ? ` (${t.unit})` : ''}</option>
                  ))}
                </select>
                {(() => {
                  const preset = SENSOR_TYPES.find((t) => t.id === newTag.sensorType);
                  return preset && preset.hint ? (
                    <span className="mt-1 block text-[10px] text-dim">{preset.hint}</span>
                  ) : null;
                })()}
              </label>
              <label className="text-sm text-ink">
                Идентификатор тега
                <input
                  required
                  value={newTag.id}
                  onChange={(event) => setNewTag({ ...newTag, id: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-line bg-base px-3 font-mono text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
                  placeholder={newTag.assetId ? `${newTag.assetId}.roaster_temp` : '<участок>.<метрика>'}
                />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label className="text-sm text-ink">
                  Название
                  <input
                    value={newTag.name}
                    onChange={(event) => setNewTag({ ...newTag, name: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-line bg-base px-3 text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
                    placeholder="Температура обжига"
                  />
                </label>
                <label className="text-sm text-ink">
                  Единица
                  <input
                    required
                    value={newTag.unit}
                    onChange={(event) => setNewTag({ ...newTag, unit: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-line bg-base px-3 text-sm text-ink outline-none placeholder:text-dim focus:border-accent/60"
                    placeholder="C"
                  />
                </label>
              </div>
            </div>
            <button
              type="submit"
              disabled={mutationState === 'busy' || !newTag.assetId}
              className="mt-4 h-9 w-full rounded bg-cyan-600 px-4 text-sm font-medium text-white transition-colors hover:bg-sky-300 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Зарегистрировать сигнал
            </button>
          </form>

          <div className="lg:col-span-2 flex flex-wrap items-center gap-3">
            <label className="text-sm text-mute">
              Оператор (аудит)
              <input
                value={actor}
                onChange={(event) => setActor(event.target.value)}
                className="ml-2 h-9 w-40 rounded border border-line bg-base px-3 font-mono text-sm text-ink outline-none focus:border-accent/60"
                aria-label="Субъект для аудита"
              />
            </label>
            {mutationNote && (
              <span className={`text-sm ${mutationNote.ok ? 'text-ok' : 'text-alarm'}`} role="status">
                {mutationNote.text}
              </span>
            )}
            <p className="w-full text-xs text-dim">
              Регистрация в реестре делает тег доступным приёму. Для реального источника укажите его адрес
              в шлюзе, например спецификация Modbus в TAGS:
              <span className="ml-1 font-mono text-mute">&lt;тег&gt;:&lt;ед.&gt;|reg=hr:8:u16:0.1</span>
            </p>
          </div>
        </div>
      )}

      {activeTab === 'telemetry' && (
        <div>
          <form onSubmit={loadTelemetry} className="grid gap-3 border-b border-line pb-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_10rem_auto] md:items-end">
            <label className="block">
              <span className="mb-1 block text-xs text-dim">Оборудование</span>
              <select
                value={selectedAssetId}
                onChange={(event) => selectAsset(event.target.value)}
                className="h-9 w-full rounded border border-line bg-base px-2 text-sm text-ink outline-none focus:border-accent/60"
              >
                <option value="">Все участки</option>
                {assets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name}</option>)}
              </select>
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-dim">Сигнал</span>
              <select
                value={selectedTagId}
                onChange={(event) => selectTag(event.target.value)}
                className="h-9 w-full rounded border border-line bg-base px-2 text-sm text-ink outline-none focus:border-accent/60"
              >
                <option value="">Все сигналы</option>
                {tags
                  .filter((tag) => !selectedAssetId || tag.asset_id === selectedAssetId)
                  .map((tag) => <option key={tag.id} value={tag.id}>{tag.name}</option>)}
              </select>
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-dim">Период</span>
              <select
                value={historyHours}
                onChange={(event) => setHistoryHours(event.target.value)}
                className="h-9 w-full rounded border border-line bg-base px-2 text-sm text-ink outline-none focus:border-accent/60"
              >
                <option value="1">1 час</option>
                <option value="8">8 часов</option>
                <option value="24">24 часа</option>
                <option value="168">7 дней</option>
              </select>
            </label>
            <button
              type="submit"
              disabled={telemetryState === 'loading'}
              className="h-9 rounded bg-cyan-600 px-4 text-sm font-medium text-white transition-colors hover:bg-sky-300 disabled:cursor-wait disabled:bg-cyan-900 disabled:text-accent"
            >
              {telemetryState === 'loading' ? 'Загрузка' : 'Запросить'}
            </button>
          </form>

          {telemetryState === 'idle' && (
            <p className="py-8 text-sm text-dim">Выберите фильтры для просмотра принятых измерений.</p>
          )}
          {telemetryState === 'error' && (
            <p className="py-8 text-sm text-alarm">История телеметрии сейчас недоступна.</p>
          )}
          {telemetryState === 'ready' && (
            <div className="pt-5">
              <div className="mb-5 grid gap-4 sm:grid-cols-3">
                <div className="border-l-2 border-accent/30 pl-3">
                  <p className="text-xs text-dim">Получено измерений</p>
                  <p className="mt-1 text-xl font-semibold text-ink">{telemetry.length}</p>
                </div>
                <div className="border-l-2 border-line pl-3">
                  <p className="text-xs text-dim">Последнее значение</p>
                  <p className="mt-1 truncate font-mono text-base font-semibold text-ink">
                    {latestReading ? `${formatNumber(latestReading.value)} ${latestReading.unit}` : '—'}
                  </p>
                </div>
                <div className="border-l-2 border-line pl-3">
                  <p className="text-xs text-dim">Качество</p>
                  <p className="mt-1 text-sm text-ink">
                    {Object.entries(qualityCounts).map(([quality, count]) => `${QUALITY_LABELS[quality] ?? quality}: ${count}`).join(' · ') || '—'}
                  </p>
                </div>
              </div>

              <div className="max-h-72 overflow-auto border-y border-line">
                <table className="min-w-full text-left text-sm">
                  <thead className="sticky top-0 bg-panel2 text-[11px] uppercase tracking-wide text-dim">
                    <tr>
                      <th className="px-2 py-2 font-medium">Время измерения</th>
                      <th className="px-2 py-2 font-medium">Сигнал</th>
                      <th className="px-2 py-2 font-medium">Значение</th>
                      <th className="px-2 py-2 font-medium">Качество</th>
                      <th className="px-2 py-2 font-medium">Профиль</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-line">
                    {telemetry.map((reading) => (
                      <tr key={`${reading.gateway_id}-${reading.message_id}`} className="hover:bg-panel2/60">
                        <td className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-mute">{formatTimestamp(reading.observed_at)}</td>
                        <td className="max-w-[14rem] px-2 py-2.5">
                          <span className="block truncate text-ink" title={reading.tag_id}>{reading.tag_id}</span>
                          <span className="block truncate text-[11px] text-dim">{assetById.get(reading.asset_id)?.name ?? reading.asset_id}</span>
                        </td>
                        <td className="whitespace-nowrap px-2 py-2.5 font-mono text-ink">{formatNumber(reading.value)} {reading.unit}</td>
                        <td className={`whitespace-nowrap px-2 py-2.5 text-xs ${QUALITY_STYLES[reading.quality] ?? 'text-mute'}`}>
                          {QUALITY_LABELS[reading.quality] ?? reading.quality}
                        </td>
                        <td className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-dim">{reading.profile_id ?? '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!telemetry.length && (
                  <p className="px-2 py-5 text-sm text-dim">За выбранный период измерений нет</p>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {activeTab === 'access' && (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.7fr)]">
          <div className="divide-y divide-line border-y border-line">
            {roles.map((role) => (
              <button
                key={role.role}
                type="button"
                onClick={() => setSelectedRoleId(role.role)}
                className={`w-full px-2 py-3 text-left transition-colors ${
                  selectedRole?.role === role.role ? 'bg-accent/10' : 'hover:bg-panel2/60'
                }`}
              >
                <span className="block text-sm text-ink">{role.label}</span>
                <span className="mt-1 block font-mono text-[11px] text-dim">{role.role}</span>
              </button>
            ))}
            {!roles.length && (
              <p className="px-2 py-4 text-sm text-dim">Матрица ролей недоступна</p>
            )}
          </div>

          <div className="border-l-2 border-accent/30 pl-4">
            {selectedRole ? (
              <>
                <p className="text-xs uppercase tracking-wide text-dim">Уровень доступа</p>
                <h3 className="mt-1 text-lg font-semibold text-ink">{selectedRole.label}</h3>
                <p className="mt-1 font-mono text-xs text-dim">{selectedRole.role}</p>
                <div className="mt-5 border-t border-line pt-4">
                  <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-mute">Разрешения</p>
                  <ul className="space-y-2">
                    {selectedRole.permissions.map((permission) => (
                      <li key={permission} className="flex items-center gap-2 text-sm text-ink">
                        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-cyan-400" aria-hidden="true" />
                        <code className="font-mono text-xs text-ink">{permission}</code>
                      </li>
                    ))}
                  </ul>
                </div>
              </>
            ) : (
              <p className="text-sm text-dim">Выберите роль</p>
            )}
          </div>
        </div>
      )}

      {lastUpdated && registryState === 'ready' && (
        <p className="mt-5 text-right text-[11px] text-dim">Обновлено: {formatTimestamp(lastUpdated)}</p>
      )}
    </section>
  );
};

export default AdminPanel;
