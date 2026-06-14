import { useEffect, useRef, useState } from "react";
import type { WireEvent } from "./types";

export interface ConnectionState {
  connected: boolean;
}

export function useEvents(onEvent: (ev: WireEvent) => void): ConnectionState {
  const [connected, setConnected] = useState(false);
  const lastSeq = useRef(0);
  const handler = useRef(onEvent);
  useEffect(() => {
    handler.current = onEvent;
  });

  useEffect(() => {
    let ws: WebSocket | null = null;
    let retry: ReturnType<typeof setTimeout> | null = null;
    let closed = false;

    const connect = () => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      const url = `${proto}://${location.host}/events?lastSeq=${lastSeq.current}`;
      ws = new WebSocket(url);

      ws.onopen = () => setConnected(true);
      ws.onmessage = (e) => {
        const ev = JSON.parse(e.data) as WireEvent;
        if (ev.line && ev.line.seq > lastSeq.current) {
          lastSeq.current = ev.line.seq;
        }
        handler.current(ev);
      };
      ws.onclose = () => {
        setConnected(false);
        if (!closed) retry = setTimeout(connect, 1000);
      };
      ws.onerror = () => ws?.close();
    };

    connect();
    return () => {
      closed = true;
      if (retry) clearTimeout(retry);
      ws?.close();
    };
  }, []);

  return { connected };
}
