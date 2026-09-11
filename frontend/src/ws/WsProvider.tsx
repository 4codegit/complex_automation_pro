import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import type {
  AlarmAckedEvent,
  AlarmRaisedEvent,
  LoopStateEvent,
  TelemetryEvent,
  WsEvent,
} from '../api/types';

export interface WsStatus {
  state: 'connecting' | 'open' | 'closed';
  since: number;
}

export interface LivePoint {
  value: number;
  unit: string;
  quality: string;
  timestamp: string;
}

export interface AlarmFeedItem {
  kind: 'raised' | 'cleared' | 'acked';
  tagId: string;
  metric: string;
  severity: string;
  message: string;
  actor?: string;
  timestamp: string;
}

interface WsContextValue {
  status: WsStatus;
  // latest value per tag
  latest: Map<string, LivePoint>;
  // rolling history (oldest first), capped, for trend charts
  history: Map<string, LivePoint[]>;
  alarms: AlarmFeedItem[];
  loops: Map<string, LoopStateEvent>;
}

const HISTORY_CAP = 600;
const ALARM_CAP = 200;

const WsContext = createContext<WsContextValue | null>(null);

export const WsProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [status, setStatus] = useState<WsStatus>({ state: 'connecting', since: Date.now() });
  const [latest, setLatest] = useState<Map<string, LivePoint>>(new Map());
  const [alarms, setAlarms] = useState<AlarmFeedItem[]>([]);
  const [loops, setLoops] = useState<Map<string, LoopStateEvent>>(new Map());
  const historyRef = useRef<Map<string, LivePoint[]>>(new Map());
  const [, forceTick] = useState(0);

  // Latest-refs so the socket callbacks mutate without re-subscribing.
  const latestRef = useRef(latest);
  const loopsRef = useRef(loops);

  const pushPoint = useCallback((tagId: string, point: LivePoint) => {
    let series = historyRef.current.get(tagId);
    if (!series) {
      series = [];
      historyRef.current.set(tagId, series);
    }
    series.push(point);
    if (series.length > HISTORY_CAP) {
      series.splice(0, series.length - HISTORY_CAP);
    }
  }, []);

  useEffect(() => {
    let socket: WebSocket | null = null;
    let retry = 0;
    let stopped = false;
    let reconnectTimer: number | undefined;

    const connect = () => {
      if (stopped) return;
      const proto = window.location.protocol === 'https:' ? 'wss' : 'ws';
      const url = `${proto}://${window.location.host}/api/v1/ws`;
      setStatus({ state: 'connecting', since: Date.now() });
      socket = new WebSocket(url);

      socket.onopen = () => {
        retry = 0;
        setStatus({ state: 'open', since: Date.now() });
      };

      socket.onmessage = (msg) => {
        let ev: WsEvent;
        try {
          ev = JSON.parse(msg.data as string) as WsEvent;
        } catch {
          return;
        }
        switch (ev.type) {
          case 'telemetry': {
            const t = ev as TelemetryEvent;
            if (typeof t.value !== 'number') return;
            const point: LivePoint = {
              value: t.value,
              unit: t.unit,
              quality: t.quality,
              timestamp: t.timestamp,
            };
            latestRef.current.set(t.tag_id, point);
            pushPoint(t.tag_id, point);
            setLatest(new Map(latestRef.current));
            break;
          }
          case 'alarm_raised':
          case 'alarm_cleared':
          case 'alarm_acked': {
            const a = ev as AlarmRaisedEvent | { type: string; tag_id: string; metric?: string } & Record<string, string>;
            setAlarms((prev) => {
              const item: AlarmFeedItem = {
                kind: ev.type === 'alarm_raised' ? 'raised' : ev.type === 'alarm_cleared' ? 'cleared' : 'acked',
                tagId: a.tag_id,
                metric: (a as AlarmRaisedEvent).metric ?? '',
                severity: (a as AlarmRaisedEvent).severity ?? '',
                message: (a as AlarmRaisedEvent).message ?? '',
                actor: (ev as AlarmAckedEvent).actor,
                timestamp: a.timestamp,
              };
              const next = [item, ...prev];
              return next.length > ALARM_CAP ? next.slice(0, ALARM_CAP) : next;
            });
            break;
          }
          case 'loop_state': {
            const l = ev as LoopStateEvent;
            loopsRef.current.set(l.loop_id, l);
            setLoops(new Map(loopsRef.current));
            break;
          }
          default:
            break;
        }
        forceTick((n) => (n + 1) % 100000);
      };

      socket.onclose = () => {
        if (stopped) return;
        setStatus({ state: 'closed', since: Date.now() });
        const delay = Math.min(16000, 1000 * 2 ** retry);
        retry += 1;
        reconnectTimer = window.setTimeout(connect, delay);
      };
      socket.onerror = () => socket?.close();
    };

    connect();
    return () => {
      stopped = true;
      if (reconnectTimer) window.clearTimeout(reconnectTimer);
      socket?.close();
    };
  }, [pushPoint]);

  const value = useMemo<WsContextValue>(
    () => ({ status, latest, history: historyRef.current, alarms, loops }),
    [status, latest, alarms, loops],
  );

  return <WsContext.Provider value={value}>{children}</WsContext.Provider>;
};

export function useWs(): WsContextValue {
  const ctx = useContext(WsContext);
  if (!ctx) throw new Error('useWs must be used within WsProvider');
  return ctx;
}

export function useWsStatus(): WsStatus {
  return useWs().status;
}

export function useLive(tagId: string): LivePoint | undefined {
  return useWs().latest.get(tagId);
}

export function useLiveMap(): Map<string, LivePoint> {
  return useWs().latest;
}

export function useAlarmFeed(): AlarmFeedItem[] {
  return useWs().alarms;
}

export function useHistory(): Map<string, LivePoint[]> {
  return useWs().history;
}

export function useLoopStates(): Map<string, LoopStateEvent> {
  return useWs().loops;
}
