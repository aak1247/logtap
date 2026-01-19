export type LogtapLevel = "debug" | "info" | "warn" | "error" | "fatal" | "event";

export interface LogtapUser {
  id?: string;
  email?: string;
  username?: string;
  traits?: Record<string, any>;
}

export interface LogtapLog {
  level: LogtapLevel;
  message: string;
  timestamp?: string;
  device_id?: string;
  trace_id?: string;
  span_id?: string;
  fields?: Record<string, any>;
  tags?: Record<string, string>;
  user?: Record<string, any>;
  contexts?: Record<string, any>;
  extra?: Record<string, any>;
  sdk?: Record<string, any>;
}

export interface LogtapTrackEvent {
  name: string;
  timestamp?: string;
  device_id?: string;
  trace_id?: string;
  span_id?: string;
  properties?: Record<string, any>;
  tags?: Record<string, string>;
  user?: Record<string, any>;
  contexts?: Record<string, any>;
  extra?: Record<string, any>;
  sdk?: Record<string, any>;
}

export interface LogtapClientOptions {
  baseUrl: string;
  projectId: number | string;
  projectKey?: string;
  flushIntervalMs?: number;
  minBatchSize?: number;
  immediateEvents?: string[];
  immediateEvent?: (name: string) => boolean;
  maxBatchSize?: number;
  maxQueueSize?: number;
  timeoutMs?: number;
  gzip?: boolean;
  persistDeviceId?: boolean;
  deviceId?: string;
  user?: LogtapUser;
  globalFields?: Record<string, any>;
  globalProperties?: Record<string, any>;
  globalTags?: Record<string, string>;
  globalContexts?: Record<string, any>;
  beforeSend?: (payload: LogtapLog | LogtapTrackEvent) => LogtapLog | LogtapTrackEvent | null;
}

export class LogtapClient {
  constructor(options: LogtapClientOptions);

  setUser(user?: LogtapUser | null): void;
  identify(userId: string, traits?: Record<string, any>): void;
  clearUser(): void;
  setDeviceId(deviceId: string, options?: { persist?: boolean }): void;

  log(
    level: LogtapLevel,
    message: string,
    fields?: Record<string, any>,
    options?: {
      traceId?: string;
      spanId?: string;
      timestamp?: string | Date;
      tags?: Record<string, string>;
      deviceId?: string;
      user?: LogtapUser;
      contexts?: Record<string, any>;
      extra?: Record<string, any>;
    },
  ): void;

  debug(message: string, fields?: Record<string, any>, options?: { traceId?: string; spanId?: string; timestamp?: string | Date; tags?: Record<string, string>; deviceId?: string; user?: LogtapUser; contexts?: Record<string, any>; extra?: Record<string, any> }): void;
  info(message: string, fields?: Record<string, any>, options?: { traceId?: string; spanId?: string; timestamp?: string | Date; tags?: Record<string, string>; deviceId?: string; user?: LogtapUser; contexts?: Record<string, any>; extra?: Record<string, any> }): void;
  warn(message: string, fields?: Record<string, any>, options?: { traceId?: string; spanId?: string; timestamp?: string | Date; tags?: Record<string, string>; deviceId?: string; user?: LogtapUser; contexts?: Record<string, any>; extra?: Record<string, any> }): void;
  error(message: string, fields?: Record<string, any>, options?: { traceId?: string; spanId?: string; timestamp?: string | Date; tags?: Record<string, string>; deviceId?: string; user?: LogtapUser; contexts?: Record<string, any>; extra?: Record<string, any> }): void;
  fatal(message: string, fields?: Record<string, any>, options?: { traceId?: string; spanId?: string; timestamp?: string | Date; tags?: Record<string, string>; deviceId?: string; user?: LogtapUser; contexts?: Record<string, any>; extra?: Record<string, any> }): void;

  track(
    name: string,
    properties?: Record<string, any>,
    options?: {
      traceId?: string;
      spanId?: string;
      timestamp?: string | Date;
      tags?: Record<string, string>;
      deviceId?: string;
      user?: LogtapUser;
      contexts?: Record<string, any>;
      extra?: Record<string, any>;
      immediate?: boolean;
    },
  ): void;

  flush(): Promise<void>;
  close(): Promise<void>;

  captureBrowserErrors(options?: { includeSource?: boolean }): void;
  captureNodeErrors(): void;
}
