import type { Relationship } from "./types";

/** One relationship as the entity sheet shows it: the row that holds now and
 *  the rows it went through to get there, oldest first, ending with it. */
export interface RelationshipChain {
  current: Relationship;
  steps: Relationship[];
}

/** Groups rows from relationships.list_by_entity with include_history into
 *  chains: a row a change ended (until_node_id set) belongs to the chain of
 *  the row that names it in supersedes_id. Rows no current row leads back to
 *  are history of a relationship since deleted, and are left out. */
export function relationshipChains(rows: Relationship[]): RelationshipChain[] {
  const byId = new Map(rows.map((r) => [r.id, r]));
  return rows
    .filter((r) => !r.until_node_id)
    .map((current) => {
      const steps: Relationship[] = [];
      const seen = new Set<string>();
      for (let r: Relationship | undefined = current; r && !seen.has(r.id); r = r.supersedes_id ? byId.get(r.supersedes_id) : undefined) {
        seen.add(r.id);
        steps.unshift(r);
      }
      return { current, steps };
    })
    // In the order the relationships were first made, so recording a change
    // does not move a row in the list.
    .sort((a, b) => (a.steps[0].id < b.steps[0].id ? -1 : a.steps[0].id > b.steps[0].id ? 1 : 0));
}
