import { useCallback, useEffect, useRef, useState } from 'react';

export interface TelemetryReading {
  timestamp: string;
  stage: string;
  metric: string;
  value: number;
  unit: string;
  quality?: string;
  alert: boolean;
  emergency: boolean;
}

export interface EmergencyEvent {
  type: 'emergency_start' | 'emergency_end';
  timestamp: string;
  message: string;
}

export type WSMessage = TelemetryReading | EmergencyEvent;

const WS_URL = `ws://${window.location.hostname}:8000/api/v1/ws`;
const RECONNECT_DELAYS = [1000, 2000, 4000, 8000, 16000]; // exponential backoff

export function useWebSocket() {
  const [readings, setReadings] = useState<Map<string, TelemetryReading>>(new Map());
  const [emergency, setEmergency] = useState<EmergencyEvent | null>(null);
  const [connected, setConnected] = useState(false);
  const [alerts, setAlerts] = useState<TelemetryReading[]>([]);

  const wsRef = useRef<WebSocket | null>(null);
  const retryIdx = useRef(0);
  const intentionalClose = useRef(false);

  const connect = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) return;

    const ws = new WebSocket(WS_URL);

    ws.onopen = () => {
      setConnected(true);
      retryIdx.current = 0;
      console.log('[WS] Connected');
    };

    ws.onmessage = (ev) => {
      try {
        const data: WSMessage = JSON.parse(ev.data);

        // Emergency control messages
        if (data && 'type' in data && (data.type === 'emergency_start' || data.type === 'emergency_end')) {
          const evt = data as EmergencyEvent;
          setEmergency(evt);
          if (evt.type === 'emergency_end') {
            setEmergency(null);
          }
          return;
        }

        const reading = data as TelemetryReading;
        if (!reading.metric) return;

        setReadings((prev) => {
          const next = new Map(prev);
          next.set(reading.metric, reading);
          return next;
        });

        if (reading.alert) {
          setAlerts((prev) => {
            const next = [...prev, reading].slice(-50);
            return next;
          });
        }
      } catch {
        // ignore malformed messages
      }
    };

    ws.onclose = () => {
      setConnected(false);
      wsRef.current = null;
      if (!intentionalClose.current) {
        const delay = RECONNECT_DELAYS[retryIdx.current] ?? 16000;
        console.log(`[WS] Reconnecting in ${delay}ms…`);
        setTimeout(connect, delay);
        retryIdx.current = Math.min(retryIdx.current + 1, RECONNECT_DELAYS.length - 1);
      }
    };

    ws.onerror = () => {
      ws.close();
    };

    wsRef.current = ws;
  }, []);

  const disconnect = useCallback(() => {
    intentionalClose.current = true;
    wsRef.current?.close();
    wsRef.current = null;
    setConnected(false);
  }, []);

  const sendEmergency = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'trigger_emergency' }));
    }
  }, []);

  const stopEmergency = useCallback(() => {
    if (wsRef.current?.readyState === WebSocket.OPEN) {
      wsRef.current.send(JSON.stringify({ type: 'stop_emergency' }));
    }
  }, []);

  useEffect(() => {
    connect();
    return () => disconnect();
  }, [connect, disconnect]);

  return { readings, emergency, connected, alerts, sendEmergency, stopEmergency };
}