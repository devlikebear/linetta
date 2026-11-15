/** A small force-directed layout for the relationship graph (#386 B2):
 *  Fruchterman–Reingold with a fixed start and no randomness, so the same
 *  people and ties always land in the same places. A cast is dozens of
 *  nodes; the O(n²) repulsion is fine there and keeps this dependency-free. */

export interface Point {
  x: number;
  y: number;
}

export interface LayoutInput {
  ids: string[];
  edges: Array<{ from: string; to: string; weight?: number }>;
  width: number;
  height: number;
  /** Positions to keep: the writer dragged these, or they were laid out before. */
  fixed?: Record<string, Point>;
  iterations?: number;
}

export function forceLayout({ ids, edges, width, height, fixed = {}, iterations = 300 }: LayoutInput): Record<string, Point> {
  const n = ids.length;
  const out: Record<string, Point> = {};
  if (n === 0) return out;
  const cx = width / 2;
  const cy = height / 2;
  const radius = Math.min(width, height) * 0.38;
  // A golden-angle spiral: spread out, the same every time.
  const golden = Math.PI * (3 - Math.sqrt(5));
  const pos = ids.map((id, i) => {
    if (fixed[id]) return { ...fixed[id] };
    const r = radius * Math.sqrt((i + 0.5) / n);
    return { x: cx + r * Math.cos(i * golden), y: cy + r * Math.sin(i * golden) };
  });
  const index = new Map(ids.map((id, i) => [id, i]));
  const links = edges
    .map((e) => ({ a: index.get(e.from), b: index.get(e.to), w: e.weight ?? 1 }))
    .filter((l): l is { a: number; b: number; w: number } => l.a !== undefined && l.b !== undefined && l.a !== l.b);
  // The ideal spacing: the box shared out, no wider than half of it.
  const k = Math.min(Math.sqrt((width * height) / n) * 0.9, Math.min(width, height) / 2);
  let temperature = Math.min(width, height) / 8;
  const cooling = temperature / (iterations + 1);
  const margin = 28;

  for (let step = 0; step < iterations; step++) {
    const dx = new Array<number>(n).fill(0);
    const dy = new Array<number>(n).fill(0);
    for (let i = 0; i < n; i++) {
      for (let j = i + 1; j < n; j++) {
        let ox = pos[i].x - pos[j].x;
        let oy = pos[i].y - pos[j].y;
        let d = Math.hypot(ox, oy);
        if (d < 0.01) {
          // Two at one spot: part them along a fixed direction.
          ox = 0.01 * (i - j);
          oy = 0.01;
          d = Math.hypot(ox, oy);
        }
        const f = (k * k) / d;
        dx[i] += (ox / d) * f;
        dy[i] += (oy / d) * f;
        dx[j] -= (ox / d) * f;
        dy[j] -= (oy / d) * f;
      }
    }
    for (const { a, b, w } of links) {
      const ox = pos[a].x - pos[b].x;
      const oy = pos[a].y - pos[b].y;
      const d = Math.max(Math.hypot(ox, oy), 0.01);
      const f = ((d * d) / k) * Math.min(w, 3);
      dx[a] -= (ox / d) * f;
      dy[a] -= (oy / d) * f;
      dx[b] += (ox / d) * f;
      dy[b] += (oy / d) * f;
    }
    for (let i = 0; i < n; i++) {
      if (fixed[ids[i]]) continue;
      // A pull to the centre keeps loose nodes from drifting off.
      dx[i] += (cx - pos[i].x) * 0.003 * k;
      dy[i] += (cy - pos[i].y) * 0.003 * k;
      const d = Math.max(Math.hypot(dx[i], dy[i]), 0.01);
      const move = Math.min(d, temperature);
      pos[i].x = clamp(pos[i].x + (dx[i] / d) * move, margin, width - margin);
      pos[i].y = clamp(pos[i].y + (dy[i] / d) * move, margin, height - margin);
    }
    temperature = Math.max(temperature - cooling, 0.5);
  }
  ids.forEach((id, i) => {
    out[id] = { x: Math.round(pos[i].x), y: Math.round(pos[i].y) };
  });
  return out;
}

function clamp(v: number, lo: number, hi: number) {
  return Math.max(lo, Math.min(hi, v));
}
