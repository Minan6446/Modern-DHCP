import { ref, onScopeDispose } from 'vue';
import {
  useLeakDetector,
  useSafeEventListener,
  useSafeInterval,
  useSafeTimeout
} from '@/shared/utils/useLeakDetector';

export interface WsOptions {
  url: string;
  protocols?: string | string[];
  heartbeatMs?: number;
  reconnect?: boolean;
  maxRetry?: number;
  pauseOnHidden?: boolean;
  reconnectOnVisible?: boolean;
}

export const useWs = (options: WsOptions) => {
  const state = ref<'idle' | 'connecting' | 'open' | 'closed' | 'error'>('idle');
  const retry = ref(0);
  let socket: WebSocket | null = null;
  let heartbeatTimer: (() => void) | undefined;
  let reconnectTimer: (() => void) | undefined;
  let visibilityHandlerAttached = false;
  const leakDetector = useLeakDetector('useWs');
  let disposed = false;
  const pauseOnHidden = options.pauseOnHidden !== false; // default true
  const reconnectOnVisible = options.reconnectOnVisible !== false; // default true
  let messageHandler: ((ev: MessageEvent) => void) | null = null;
  let errorHandler: ((ev: Event) => void) | null = null;

  const connect = () => {
    if (disposed) return;
    state.value = 'connecting';
    socket = new WebSocket(options.url, options.protocols);
    socket.onopen = () => {
      state.value = 'open';
      retry.value = 0;
      startHeartbeat();
    };
    socket.onclose = () => {
      state.value = 'closed';
      stopHeartbeat();
      if (options.reconnect !== false && retry.value < (options.maxRetry ?? 5)) {
        retry.value += 1;
        reconnectTimer = useSafeTimeout(
          connect,
          Math.min(30000, 1000 * 2 ** retry.value),
          leakDetector
        );
      }
    };
    socket.onerror = (ev) => {
      state.value = 'error';
      errorHandler?.(ev);
    };
    socket.onmessage = (ev) => {
      messageHandler?.(ev);
    };
  };

  const startHeartbeat = () => {
    if (!options.heartbeatMs) return;
    stopHeartbeat();
    heartbeatTimer = useSafeInterval(
      () => {
        socket?.readyState === WebSocket.OPEN && socket.send(JSON.stringify({ type: 'ping' }));
      },
      options.heartbeatMs,
      leakDetector
    );
  };

  const stopHeartbeat = () => {
    heartbeatTimer?.();
    heartbeatTimer = undefined;
    reconnectTimer?.();
    reconnectTimer = undefined;
  };

  const send = (data: unknown) => {
    if (socket?.readyState === WebSocket.OPEN) {
      socket.send(typeof data === 'string' ? data : JSON.stringify(data));
    }
  };

  const close = () => {
    options.reconnect = false;
    stopHeartbeat();
    socket?.close();
  };

  const handleVisibility = () => {
    if (!pauseOnHidden) return;
    if (document.visibilityState === 'hidden') {
      close();
    } else if (reconnectOnVisible && state.value !== 'open') {
      connect();
    }
  };

  const onMessage = (handler: (ev: MessageEvent) => void) => {
    messageHandler = handler;
    socket && (socket.onmessage = (ev) => messageHandler?.(ev));
  };

  const onError = (handler: (ev: Event) => void) => {
    errorHandler = handler;
    socket &&
      (socket.onerror = (ev) => {
        state.value = 'error';
        errorHandler?.(ev);
      });
  };

  connect();
  if (!visibilityHandlerAttached && typeof document !== 'undefined') {
    useSafeEventListener(document, 'visibilitychange', handleVisibility, false, leakDetector);
    visibilityHandlerAttached = true;
  }

  const dispose = () => {
    disposed = true;
    close();
    leakDetector.disposeAll();
    visibilityHandlerAttached = false;
  };

  onScopeDispose(dispose);

  return { state, send, close: dispose, onMessage, onError };
};
