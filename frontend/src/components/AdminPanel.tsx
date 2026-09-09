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

const QUALITY_LABELS: Record<string, string> = {
  good: 'Годные',
  uncertain: 'Неопределённые',
  bad: 'Негодные',
  stale: 'Устаревшие',
  substituted: 'Замещённые',
  offline: 'Нет связи',
};

const QUALITY_STYLES: Record<string, string> = {
  good: 'text-emerald-300',
  uncertain: 'text-amber-300',
  bad: 'text-red-300',
  stale: 'text-amber-300',
  substituted: 'text-cyan-300',
  offline: 'text-red-300',
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
  const [newTag, setNewTag] = useState({ assetId: '', id: '', name: '', unit: '' });
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
    ? 'text-emerald-400'
    : registryState === 'loading'
      ? 'text-amber-400'
      : 'text-red-400';

  return (
    <section id="administration" className="rounded-lg border border-gray-800 bg-gray-900 p-5">
      <div className="mb-5 flex flex-wrap items-start justify-between gap-4">
        <div>
          <h2 className="text-base font-semibold text-white">Администрирование платформы</h2>
          <p className="mt-1 text-xs text-gray-500">Реестр фабрики, доступы и проверка принятых измерений</p>
        </div>
        <div className="flex items-center gap-3">
          <span className={`text-xs ${registryStatusClass}`} role="status">{registryStatus}</span>
          <button
            type="button"
            onClick={() => void loadRegistry()}
            className="grid h-8 w-8 place-items-center rounded border border-gray-700 text-base text-gray-300 transition-colors hover:border-cyan-600 hover:text-cyan-300 disabled:cursor-wait disabled:opacity-50"
            aria-label="Обновить реестр"
            title="Обновить реестр"
            disabled={registryState === 'loading'}
          >
            ↻
          </button>
        </div>
      </div>

      <dl className="mb-5 grid grid-cols-2 border-y border-gray-800 py-3 sm:grid-cols-4">
        <div className="border-r border-gray-800 px-3 first:pl-0">
          <dt className="text-[11px] uppercase tracking-wide text-gray-500">Участки</dt>
          <dd className="mt-1 text-lg font-semibold text-gray-100">{assets.length}</dd>
        </div>
        <div className="border-r border-gray-800 px-3">
          <dt className="text-[11px] uppercase tracking-wide text-gray-500">Сигналы</dt>
          <dd className="mt-1 text-lg font-semibold text-gray-100">{tags.length}</dd>
        </div>
        <div className="border-r border-gray-800 px-3">
          <dt className="text-[11px] uppercase tracking-wide text-gray-500">Активные</dt>
          <dd className="mt-1 text-lg font-semibold text-emerald-300">{tags.filter((tag) => tag.active).length}</dd>
        </div>
        <div className="px-3 last:pr-0">
          <dt className="text-[11px] uppercase tracking-wide text-gray-500">Роли</dt>
          <dd className="mt-1 text-lg font-semibold text-gray-100">{roles.length}</dd>
        </div>
      </dl>

      <div className="mb-5 flex gap-1 border-b border-gray-800" role="tablist" aria-label="Администрирование">
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
                ? 'border-cyan-400 text-cyan-300'
                : 'border-transparent text-gray-500 hover:text-gray-300'
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
              <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-400">Оборудование</h3>
              <span className="text-xs text-gray-600">{visibleAssets.length}</span>
            </div>
            <input
              value={assetSearch}
              onChange={(event) => setAssetSearch(event.target.value)}
              className="mb-3 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
              placeholder="Поиск участка или оборудования"
              aria-label="Поиск оборудования"
            />
            <div className="max-h-72 divide-y divide-gray-800 overflow-y-auto border-y border-gray-800">
              {visibleAssets.map((asset) => {
                const isSelected = selectedAssetId === asset.id;
                return (
                  <button
                    key={asset.id}
                    type="button"
                    onClick={() => selectAsset(asset.id)}
                    className={`flex w-full items-center justify-between gap-3 px-2 py-3 text-left transition-colors ${
                      isSelected ? 'bg-cyan-950/40' : 'hover:bg-gray-800/60'
                    }`}
                  >
                    <span className="min-w-0">
                      <span className="block truncate text-sm text-gray-200">{asset.name}</span>
                      <span className="block truncate text-xs text-gray-500">{asset.area} · {asset.id}</span>
                    </span>
                    <span className={`shrink-0 text-[11px] ${asset.active ? 'text-emerald-400' : 'text-gray-600'}`}>
                      {asset.active ? asset.criticality : 'inactive'}
                    </span>
                  </button>
                );
              })}
              {!visibleAssets.length && (
                <p className="px-2 py-4 text-sm text-gray-500">Совпадений нет</p>
              )}
            </div>
          </div>

          <div>
            <div className="mb-3 flex flex-wrap items-center justify-between gap-3">
              <div>
                <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-400">Сигнальный реестр</h3>
                <p className="mt-1 text-xs text-gray-600">{selectedAsset ? selectedAsset.name : 'Все участки'}</p>
              </div>
              {selectedAssetId && (
                <button
                  type="button"
                  onClick={() => selectAsset('')}
                  className="text-xs text-cyan-400 transition-colors hover:text-cyan-200"
                >
                  Все участки
                </button>
              )}
            </div>
            <input
              value={tagSearch}
              onChange={(event) => setTagSearch(event.target.value)}
              className="mb-3 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
              placeholder="Поиск тега или единицы"
              aria-label="Поиск сигнала"
            />
            <div className="max-h-72 overflow-auto border-y border-gray-800">
              <table className="min-w-full text-left text-sm">
                <thead className="sticky top-0 bg-gray-900 text-[11px] uppercase tracking-wide text-gray-500">
                  <tr>
                    <th className="px-2 py-2 font-medium">Сигнал</th>
                    <th className="px-2 py-2 font-medium">Ед.</th>
                    <th className="px-2 py-2 font-medium">Интервал</th>
                    <th className="px-2 py-2 font-medium">Статус</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-gray-800">
                  {visibleTags.map((tag) => (
                    <tr
                      key={tag.id}
                      onClick={() => selectTag(tag.id)}
                      className={`cursor-pointer transition-colors hover:bg-gray-800/60 ${selectedTagId === tag.id ? 'bg-cyan-950/40' : ''}`}
                    >
                      <td className="max-w-[14rem] px-2 py-2.5">
                        <span className="block truncate text-gray-200" title={tag.name}>{tag.name}</span>
                        <span className="block truncate font-mono text-[11px] text-gray-600" title={tag.id}>{tag.id}</span>
                      </td>
                      <td className="whitespace-nowrap px-2 py-2.5 text-gray-400">{tag.unit}</td>
                      <td className="whitespace-nowrap px-2 py-2.5 text-gray-400">
                        {tag.sampling_interval_seconds ?? '—'}{tag.sampling_interval_seconds !== undefined ? ' s' : ''}
                      </td>
                      <td className={`whitespace-nowrap px-2 py-2.5 text-xs ${tag.active ? 'text-emerald-400' : 'text-gray-600'}`}>
                        {tag.active ? 'Активен' : 'Неактивен'}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
              {!visibleTags.length && (
                <p className="px-2 py-4 text-sm text-gray-500">Зарегистрированных сигналов нет</p>
              )}
            </div>
          </div>
        </div>
      )}

      {activeTab === 'sensors' && (
        <div className="grid gap-6 lg:grid-cols-2">
          <form onSubmit={createAsset} className="rounded border border-gray-800 bg-gray-900/40 p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-400">Новый участок (asset)</h3>
            <div className="mt-3 grid gap-3">
              <label className="text-sm text-gray-300">
                Идентификатор
                <input
                  required
                  value={newAsset.id}
                  onChange={(event) => setNewAsset({ ...newAsset, id: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 font-mono text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
                  placeholder="plant-a.roasting"
                />
              </label>
              <label className="text-sm text-gray-300">
                Название
                <input
                  required
                  value={newAsset.name}
                  onChange={(event) => setNewAsset({ ...newAsset, name: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
                  placeholder="Обжиг и кальцинация"
                />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label className="text-sm text-gray-300">
                  Участок (area)
                  <input
                    required
                    value={newAsset.area}
                    onChange={(event) => setNewAsset({ ...newAsset, area: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
                    placeholder="Roasting"
                  />
                </label>
                <label className="text-sm text-gray-300">
                  Критичность
                  <select
                    value={newAsset.criticality}
                    onChange={(event) => setNewAsset({ ...newAsset, criticality: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500"
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
              className="mt-4 h-9 w-full rounded bg-cyan-600 px-4 text-sm font-medium text-white transition-colors hover:bg-cyan-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Добавить участок
            </button>
          </form>

          <form onSubmit={createTag} className="rounded border border-gray-800 bg-gray-900/40 p-4">
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-400">Новый сигнал (тег)</h3>
            <div className="mt-3 grid gap-3">
              <label className="text-sm text-gray-300">
                Участок
                <select
                  required
                  value={newTag.assetId}
                  onChange={(event) => setNewTag({ ...newTag, assetId: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500"
                >
                  <option value="">— выберите —</option>
                  {assets.filter((asset) => asset.active).map((asset) => (
                    <option key={asset.id} value={asset.id}>{asset.name} ({asset.id})</option>
                  ))}
                </select>
              </label>
              <label className="text-sm text-gray-300">
                Идентификатор тега
                <input
                  required
                  value={newTag.id}
                  onChange={(event) => setNewTag({ ...newTag, id: event.target.value })}
                  className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 font-mono text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
                  placeholder={newTag.assetId ? `${newTag.assetId}.roaster_temp` : '<участок>.<метрика>'}
                />
              </label>
              <div className="grid grid-cols-2 gap-3">
                <label className="text-sm text-gray-300">
                  Название
                  <input
                    value={newTag.name}
                    onChange={(event) => setNewTag({ ...newTag, name: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
                    placeholder="Температура обжига"
                  />
                </label>
                <label className="text-sm text-gray-300">
                  Единица
                  <input
                    required
                    value={newTag.unit}
                    onChange={(event) => setNewTag({ ...newTag, unit: event.target.value })}
                    className="mt-1 h-9 w-full rounded border border-gray-700 bg-gray-950 px-3 text-sm text-gray-100 outline-none placeholder:text-gray-600 focus:border-cyan-500"
                    placeholder="C"
                  />
                </label>
              </div>
            </div>
            <button
              type="submit"
              disabled={mutationState === 'busy' || !newTag.assetId}
              className="mt-4 h-9 w-full rounded bg-cyan-600 px-4 text-sm font-medium text-white transition-colors hover:bg-cyan-500 disabled:cursor-not-allowed disabled:opacity-50"
            >
              Зарегистрировать сигнал
            </button>
          </form>

          <div className="lg:col-span-2 flex flex-wrap items-center gap-3">
            <label className="text-sm text-gray-400">
              Оператор (аудит)
              <input
                value={actor}
                onChange={(event) => setActor(event.target.value)}
                className="ml-2 h-9 w-40 rounded border border-gray-700 bg-gray-950 px-3 font-mono text-sm text-gray-100 outline-none focus:border-cyan-500"
                aria-label="Субъект для аудита"
              />
            </label>
            {mutationNote && (
              <span className={`text-sm ${mutationNote.ok ? 'text-emerald-300' : 'text-red-300'}`} role="status">
                {mutationNote.text}
              </span>
            )}
            <p className="w-full text-xs text-gray-600">
              Регистрация в реестре делает тег доступным приёму. Для реального источника укажите его адрес
              в шлюзе, например спецификация Modbus в TAGS:
              <span className="ml-1 font-mono text-gray-400">&lt;тег&gt;:&lt;ед.&gt;|reg=hr:8:u16:0.1</span>
            </p>
          </div>
        </div>
      )}

      {activeTab === 'telemetry' && (
        <div>
          <form onSubmit={loadTelemetry} className="grid gap-3 border-b border-gray-800 pb-5 md:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_10rem_auto] md:items-end">
            <label className="block">
              <span className="mb-1 block text-xs text-gray-500">Оборудование</span>
              <select
                value={selectedAssetId}
                onChange={(event) => selectAsset(event.target.value)}
                className="h-9 w-full rounded border border-gray-700 bg-gray-950 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500"
              >
                <option value="">Все участки</option>
                {assets.map((asset) => <option key={asset.id} value={asset.id}>{asset.name}</option>)}
              </select>
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-gray-500">Сигнал</span>
              <select
                value={selectedTagId}
                onChange={(event) => selectTag(event.target.value)}
                className="h-9 w-full rounded border border-gray-700 bg-gray-950 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500"
              >
                <option value="">Все сигналы</option>
                {tags
                  .filter((tag) => !selectedAssetId || tag.asset_id === selectedAssetId)
                  .map((tag) => <option key={tag.id} value={tag.id}>{tag.name}</option>)}
              </select>
            </label>
            <label className="block">
              <span className="mb-1 block text-xs text-gray-500">Период</span>
              <select
                value={historyHours}
                onChange={(event) => setHistoryHours(event.target.value)}
                className="h-9 w-full rounded border border-gray-700 bg-gray-950 px-2 text-sm text-gray-100 outline-none focus:border-cyan-500"
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
              className="h-9 rounded bg-cyan-600 px-4 text-sm font-medium text-white transition-colors hover:bg-cyan-500 disabled:cursor-wait disabled:bg-cyan-900 disabled:text-cyan-300"
            >
              {telemetryState === 'loading' ? 'Загрузка' : 'Запросить'}
            </button>
          </form>

          {telemetryState === 'idle' && (
            <p className="py-8 text-sm text-gray-500">Выберите фильтры для просмотра принятых измерений.</p>
          )}
          {telemetryState === 'error' && (
            <p className="py-8 text-sm text-red-300">История телеметрии сейчас недоступна.</p>
          )}
          {telemetryState === 'ready' && (
            <div className="pt-5">
              <div className="mb-5 grid gap-4 sm:grid-cols-3">
                <div className="border-l-2 border-cyan-500 pl-3">
                  <p className="text-xs text-gray-500">Получено измерений</p>
                  <p className="mt-1 text-xl font-semibold text-gray-100">{telemetry.length}</p>
                </div>
                <div className="border-l-2 border-gray-600 pl-3">
                  <p className="text-xs text-gray-500">Последнее значение</p>
                  <p className="mt-1 truncate font-mono text-base font-semibold text-gray-100">
                    {latestReading ? `${formatNumber(latestReading.value)} ${latestReading.unit}` : '—'}
                  </p>
                </div>
                <div className="border-l-2 border-gray-600 pl-3">
                  <p className="text-xs text-gray-500">Качество</p>
                  <p className="mt-1 text-sm text-gray-200">
                    {Object.entries(qualityCounts).map(([quality, count]) => `${QUALITY_LABELS[quality] ?? quality}: ${count}`).join(' · ') || '—'}
                  </p>
                </div>
              </div>

              <div className="max-h-72 overflow-auto border-y border-gray-800">
                <table className="min-w-full text-left text-sm">
                  <thead className="sticky top-0 bg-gray-900 text-[11px] uppercase tracking-wide text-gray-500">
                    <tr>
                      <th className="px-2 py-2 font-medium">Время измерения</th>
                      <th className="px-2 py-2 font-medium">Сигнал</th>
                      <th className="px-2 py-2 font-medium">Значение</th>
                      <th className="px-2 py-2 font-medium">Качество</th>
                      <th className="px-2 py-2 font-medium">Профиль</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-gray-800">
                    {telemetry.map((reading) => (
                      <tr key={`${reading.gateway_id}-${reading.message_id}`} className="hover:bg-gray-800/60">
                        <td className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-gray-400">{formatTimestamp(reading.observed_at)}</td>
                        <td className="max-w-[14rem] px-2 py-2.5">
                          <span className="block truncate text-gray-200" title={reading.tag_id}>{reading.tag_id}</span>
                          <span className="block truncate text-[11px] text-gray-600">{assetById.get(reading.asset_id)?.name ?? reading.asset_id}</span>
                        </td>
                        <td className="whitespace-nowrap px-2 py-2.5 font-mono text-gray-100">{formatNumber(reading.value)} {reading.unit}</td>
                        <td className={`whitespace-nowrap px-2 py-2.5 text-xs ${QUALITY_STYLES[reading.quality] ?? 'text-gray-400'}`}>
                          {QUALITY_LABELS[reading.quality] ?? reading.quality}
                        </td>
                        <td className="whitespace-nowrap px-2 py-2.5 font-mono text-xs text-gray-500">{reading.profile_id ?? '—'}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
                {!telemetry.length && (
                  <p className="px-2 py-5 text-sm text-gray-500">За выбранный период измерений нет</p>
                )}
              </div>
            </div>
          )}
        </div>
      )}

      {activeTab === 'access' && (
        <div className="grid gap-6 lg:grid-cols-[minmax(0,0.8fr)_minmax(0,1.7fr)]">
          <div className="divide-y divide-gray-800 border-y border-gray-800">
            {roles.map((role) => (
              <button
                key={role.role}
                type="button"
                onClick={() => setSelectedRoleId(role.role)}
                className={`w-full px-2 py-3 text-left transition-colors ${
                  selectedRole?.role === role.role ? 'bg-cyan-950/40' : 'hover:bg-gray-800/60'
                }`}
              >
                <span className="block text-sm text-gray-200">{role.label}</span>
                <span className="mt-1 block font-mono text-[11px] text-gray-600">{role.role}</span>
              </button>
            ))}
            {!roles.length && (
              <p className="px-2 py-4 text-sm text-gray-500">Матрица ролей недоступна</p>
            )}
          </div>

          <div className="border-l-2 border-cyan-500 pl-4">
            {selectedRole ? (
              <>
                <p className="text-xs uppercase tracking-wide text-gray-500">Уровень доступа</p>
                <h3 className="mt-1 text-lg font-semibold text-gray-100">{selectedRole.label}</h3>
                <p className="mt-1 font-mono text-xs text-gray-600">{selectedRole.role}</p>
                <div className="mt-5 border-t border-gray-800 pt-4">
                  <p className="mb-3 text-xs font-semibold uppercase tracking-wide text-gray-400">Разрешения</p>
                  <ul className="space-y-2">
                    {selectedRole.permissions.map((permission) => (
                      <li key={permission} className="flex items-center gap-2 text-sm text-gray-300">
                        <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-cyan-400" aria-hidden="true" />
                        <code className="font-mono text-xs text-gray-200">{permission}</code>
                      </li>
                    ))}
                  </ul>
                </div>
              </>
            ) : (
              <p className="text-sm text-gray-500">Выберите роль</p>
            )}
          </div>
        </div>
      )}

      {lastUpdated && registryState === 'ready' && (
        <p className="mt-5 text-right text-[11px] text-gray-600">Обновлено: {formatTimestamp(lastUpdated)}</p>
      )}
    </section>
  );
};

export default AdminPanel;
