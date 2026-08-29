import type { Entity } from "../types";

interface Candidate {
  id: string;
  label: string;
  surface: string;
}

export interface AutoMentionResult {
  doc: object;
  applied: number;
}

export function autoMentionDoc(doc: object, entities: Entity[]): AutoMentionResult {
  const candidates = buildCandidates(entities);
  if (candidates.length === 0) return { doc, applied: 0 };

  const converted = convertNode(doc, candidates);
  return { doc: converted.node, applied: converted.applied };
}

/** How many registered names appear in the prose without being linked yet.
 *
 *  Counting is separate from applying on purpose. Turning a name into a
 *  mention edits the manuscript, and a homonym — a place called 해윤 in a work
 *  that also has a character 해윤 — would be linked to the wrong record. So
 *  Linetta reports what it found and leaves the decision with the writer,
 *  rather than rewriting their prose on a timer. */
export function countAutoMentionCandidates(doc: object, entities: Entity[]): number {
  const candidates = buildCandidates(entities);
  if (candidates.length === 0) return 0;
  return convertNode(doc, candidates).applied;
}

function buildCandidates(entities: Entity[]): Candidate[] {
  const seen = new Set<string>();
  const out: Candidate[] = [];
  for (const entity of entities) {
    for (const raw of [entity.name, ...(entity.aliases ?? [])]) {
      const surface = raw.trim();
      if ([...surface].length < 2) continue;
      const key = `${entity.id}\n${surface}`;
      if (seen.has(key)) continue;
      seen.add(key);
      out.push({ id: entity.id, label: surface, surface });
    }
  }
  out.sort((a, b) => b.surface.length - a.surface.length);
  return out;
}

function convertNode(node: unknown, candidates: Candidate[]): { node: any; applied: number } {
  if (!node || typeof node !== "object" || Array.isArray(node)) {
    return { node, applied: 0 };
  }
  const cur = node as Record<string, any>;
  if (cur.type === "mention") {
    return { node: cur, applied: 0 };
  }
  if (cur.type === "text" && typeof cur.text === "string") {
    const converted = convertTextNode(cur, candidates);
    if (converted.nodes.length === 1) {
      return { node: converted.nodes[0], applied: converted.applied };
    }
    return { node: converted.nodes, applied: converted.applied };
  }
  if (!Array.isArray(cur.content)) {
    return { node: cur, applied: 0 };
  }

  let applied = 0;
  const content: any[] = [];
  for (const child of cur.content) {
    const converted = convertNode(child, candidates);
    applied += converted.applied;
    if (Array.isArray(converted.node)) {
      content.push(...converted.node);
    } else {
      content.push(converted.node);
    }
  }
  if (applied === 0) return { node: cur, applied: 0 };
  return { node: { ...cur, content }, applied };
}

function convertTextNode(node: Record<string, any>, candidates: Candidate[]): { nodes: any[]; applied: number } {
  const text = node.text as string;
  const nodes: any[] = [];
  let applied = 0;
  let index = 0;

  const pushText = (value: string) => {
    if (!value) return;
    nodes.push({ ...node, text: value });
  };

  while (index < text.length) {
    const match = matchAt(text, index, candidates);
    if (!match) {
      const next = nextMatchIndex(text, index + 1, candidates);
      pushText(text.slice(index, next));
      index = next;
      continue;
    }
    nodes.push({ type: "mention", attrs: { id: match.candidate.id, label: match.candidate.label } });
    applied++;
    index = match.end;
  }

  return { nodes: mergeAdjacentText(nodes), applied };
}

function matchAt(text: string, index: number, candidates: Candidate[]): { candidate: Candidate; end: number } | null {
  const atPrefixed = text[index] === "@";
  const offset = atPrefixed ? index + 1 : index;
  for (const candidate of candidates) {
    if (text.startsWith(candidate.surface, offset)) {
      return { candidate, end: offset + candidate.surface.length };
    }
  }
  return null;
}

function nextMatchIndex(text: string, from: number, candidates: Candidate[]): number {
  for (let i = from; i < text.length; i++) {
    if (matchAt(text, i, candidates)) return i;
  }
  return text.length;
}

function mergeAdjacentText(nodes: any[]): any[] {
  const out: any[] = [];
  for (const node of nodes) {
    const prev = out[out.length - 1];
    if (node?.type === "text" && prev?.type === "text" && JSON.stringify(prev.marks ?? null) === JSON.stringify(node.marks ?? null)) {
      prev.text += node.text;
    } else {
      out.push(node);
    }
  }
  return out;
}
