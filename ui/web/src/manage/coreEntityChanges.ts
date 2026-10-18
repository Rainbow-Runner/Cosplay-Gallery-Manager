import type { ManageCoreEntity } from "./types";

const display = (value: string | null | undefined) => (value ?? "").trim().normalize("NFC");

// Preserve alias spelling and order: both are stored metadata. Identity
// collisions/duplicates are validated separately, not silently discarded.
export function coreEntityChanged(saved: ManageCoreEntity | undefined, draft: ManageCoreEntity, aliases: string[]): boolean {
  if (!saved || saved.uuid !== draft.uuid || saved.kind !== draft.kind) return false;
  if (saved.name !== display(draft.name) || saved.sortName !== display(draft.sortName) ||
      saved.aliases.length !== aliases.length || saved.aliases.some((alias, i) => alias !== display(aliases[i]))) return true;
  if (draft.kind === "CHARACTER") return (saved.workUUID ?? "") !== (draft.workUUID ?? "");
  if (draft.kind === "TAG") return saved.useInRecommendation !== draft.useInRecommendation ||
    (saved.allowDirectAssignment ?? true) !== (draft.allowDirectAssignment ?? true);
  if (draft.kind === "COSER") return saved.profileSummary !== draft.profileSummary || saved.biography !== draft.biography ||
    saved.countryOrRegion !== display(draft.countryOrRegion);
  return false;
}
