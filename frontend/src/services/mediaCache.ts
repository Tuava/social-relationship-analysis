import { api } from "@/services/api";

type CacheEntry = { url: string; touchedAt: number };

const cache = new Map<string, CacheEntry>();
const pending = new Map<string, Promise<string>>();
const waiters: Array<() => void> = [];
// Avatar URLs are shared by Cytoscape, tables and inspectors. Revoking an URL
// merely because it is old makes already-rendered nodes lose their image.
const maximumEntries = 2500;
const maximumConcurrent = 24;
let active = 0;

async function acquire() {
  if (active < maximumConcurrent) {
    active += 1;
    return;
  }
  await new Promise<void>((resolve) => waiters.push(resolve));
  active += 1;
}

function release() {
  active = Math.max(0, active - 1);
  waiters.shift()?.();
}

function trim() {
  if (cache.size <= maximumEntries) return;
  const oldest = [...cache.entries()].sort(
    (left, right) => left[1].touchedAt - right[1].touchedAt,
  );
  // Keep a generous hot set. This only runs after thousands of local assets
  // have been resolved and never touches the entries used most recently.
  for (const [key, entry] of oldest.slice(0, Math.min(100, cache.size - maximumEntries))) {
    URL.revokeObjectURL(entry.url);
    cache.delete(key);
  }
}

export async function cachedMediaURL(source: string): Promise<string> {
  if (!source) return "";
  const existing = cache.get(source);
  if (existing) {
    existing.touchedAt = Date.now();
    return existing.url;
  }
  const current = pending.get(source);
  if (current) return current;
  const request = (async () => {
    await acquire();
    try {
      const response = await api.get(source, { responseType: "blob" });
      const url = URL.createObjectURL(response.data);
      cache.set(source, { url, touchedAt: Date.now() });
      trim();
      return url;
    } finally {
      release();
      pending.delete(source);
    }
  })();
  pending.set(source, request);
  return request;
}
