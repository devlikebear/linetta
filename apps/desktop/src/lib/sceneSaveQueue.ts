export interface VersionedSaveResult {
  content_version?: number;
}

type Persist<Result extends VersionedSaveResult> = (
  nodeId: string,
  doc: string,
  expectedContentVersion: number,
) => Promise<Result>;

/** Serializes writes for each scene and carries optimistic-lock versions. */
export class SceneSaveQueue<Result extends VersionedSaveResult = VersionedSaveResult> {
  private readonly versions = new Map<string, number>();
  private readonly tails = new Map<string, Promise<void>>();

  constructor(private readonly persist: Persist<Result>) {}

  seed(nodeId: string, contentVersion: number): void {
    const current = this.versions.get(nodeId);
    if (current === undefined || contentVersion > current) {
      this.versions.set(nodeId, contentVersion);
    }
  }

  save(nodeId: string, doc: string): Promise<Result> {
    const previous = this.tails.get(nodeId) ?? Promise.resolve();
    const run = previous.then(async () => {
      const expected = this.versions.get(nodeId);
      if (expected === undefined) {
        throw new Error(`content version is not seeded for node ${nodeId}`);
      }
      const updated = await this.persist(nodeId, doc, expected);
      const nextVersion = updated.content_version ?? expected + 1;
      this.seed(nodeId, nextVersion);
      return updated;
    });
    const tail = run.then(() => undefined, () => undefined);
    this.tails.set(nodeId, tail);
    void tail.then(() => {
      if (this.tails.get(nodeId) === tail) this.tails.delete(nodeId);
    });
    return run;
  }

  /** Resolves once every in-flight or queued save has settled — success or
   *  failure, it never rejects. Each per-scene tail already swallows its own
   *  outcome (see `save` above), so this only has to wait on the current set
   *  of tails; a save queued by a caller who is still awaiting a prior one on
   *  the same key extends that key's own tail, so re-checking after the first
   *  wait picks up anything that was still settling. */
  async idle(): Promise<void> {
    let pending = [...this.tails.values()];
    while (pending.length > 0) {
      await Promise.all(pending);
      pending = [...this.tails.values()];
    }
  }
}
