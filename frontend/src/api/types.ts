// Canonical API types (TZ §13). One source of truth for the whole app.

export interface Asset {
  id: string;
  name: string;
  area: string;
  criticality: string;
  active: boolean;
}

export interface Tag {
  id: string;
  asset_id: string;
  name: string;
  unit: string;
  data_type: string;
  engineering_min: number | null;
  engineering_max: number | null;
  sampling_interval_seconds: number;
  criticality: string;
  active: boolean;
  direction: 'input' | 'output';
}

export interface Reading {
  message_id: string;
  gateway_id: string;
  observed_at: string;
  asset_id: string;
  tag_id: string;
  value: number | boolean | string | null;
  unit: string;
  quality: string;
  stale?: boolean;
  age_seconds?: number;
}

// WebSocket events (server → client).
export interface TelemetryEvent {
  type: 'telemetry';
  timestamp: string;
  asset_id: string;
  tag_id: string;
  value: number | boolean | string;
  unit: string;
  quality: string;
}

export interface AlarmRaisedEvent {
  type: 'alarm_raised';
  alarm_id: string;
  tag_id: string;
  metric: string;
  severity: string;
  priority: number;
  message: string;
  timestamp: string;
}

export interface AlarmClearedEvent {
  type: 'alarm_cleared';
  alarm_id?: string;
  tag_id: string;
  metric: string;
  timestamp: string;
}

export interface AlarmAckedEvent {
  type: 'alarm_acked';
  alarm_id: string;
  tag_id: string;
  actor: string;
  timestamp: string;
}

export interface LoopStateEvent {
  type: 'loop_state';
  loop_id: string;
  label: string;
  pv_tag: string;
  mv_tag: string;
  mode: 'auto' | 'manual';
  state: string;
  sp: number;
  out: number;
  timestamp: string;
}

export type WsEvent =
  | TelemetryEvent
  | AlarmRaisedEvent
  | AlarmClearedEvent
  | AlarmAckedEvent
  | LoopStateEvent;

// Alarms.
export interface Alarm {
  id: string;
  tag_id: string;
  metric: string;
  state: string;
  severity: string;
  priority: number;
  message: string;
  observed_at: string;
  ack_by: string | null;
  ack_at: string | null;
  ack_comment: string | null;
  cleared_at: string | null;
  updated_at: string;
}

export interface AlarmEvent {
  id: string;
  occurred_at: string;
  tag_id: string;
  metric: string;
  event: 'raised' | 'cleared' | 'acked' | 'shelved';
  severity: string;
  priority: number;
  message: string;
  actor: string;
  comment: string;
}

export interface AlarmLimit {
  tag_id: string;
  enabled: boolean;
  lo_lo: number | null;
  lo: number | null;
  hi: number | null;
  hi_hi: number | null;
  severity: string;
  rationalised_by: string;
  notes: string;
}

// Control (TZ §9/§13).
export interface ControlLoop {
  id: string;
  label: string;
  pv_tag: string;
  mv_tag: string;
  sp: number;
  sp_min: number;
  sp_max: number;
  out_min: number;
  out_max: number;
  kp: number;
  ki: number;
  kd: number;
  mode: 'auto' | 'manual';
  state: string;
  out: number;
  pv?: number;
  pv_at?: string;
}

export interface ActuatorOutput {
  tag_id: string;
  value: number;
  seq: number;
  status: 'auto' | 'manual' | 'hold';
}

// Metallurgy (TZ §10.5).
export interface BalanceRow {
  stream: string;
  label: string;
  tph: number;
  grade_pct: number;
  metal_tph: number;
}

export interface KpiTarget {
  min: number | null;
  max: number | null;
}

export interface Kpi {
  id: string;
  label: string;
  value: number;
  unit: string;
  status: 'ok' | 'warn' | 'bad' | 'no_data';
  formula: string;
  target?: KpiTarget;
  src: string[];
  note?: string;
}

export interface MetallurgySummary {
  computed_at: string;
  valid: boolean;
  note?: string;
  balance: BalanceRow[];
  kpis: Kpi[];
}

// Profiles.
export interface Profile {
  id: string;
  name: string;
  ore_domain: string;
  version: number;
  params: string;
  author: string;
  approved_by: string | null;
  status: 'draft' | 'approved' | 'active' | 'superseded';
  reason: string;
  created_at: string;
}

// Access (TZ §12).
export interface WhoAmI {
  subject: string;
  roles: string[];
  permissions: string[];
}

export interface Gateway {
  id: string;
  name: string;
  area: string;
  protocol: string;
  status: string;
  last_seen_at: string | null;
  buffer_size: number;
  version: string | null;
}

export interface AuditEvent {
  id: string;
  occurred_at: string;
  actor: string;
  role: string;
  action: string;
  resource_type: string;
  resource_id: string;
  detail: string;
  src_ip: string;
}

export interface Role {
  id: string;
  label: string;
  permissions: string[];
  system: boolean;
  created_at: string;
}

export interface RoleAssignment {
  id: string;
  subject: string;
  role_id: string;
  assigned_by: string;
  assigned_at: string;
}
