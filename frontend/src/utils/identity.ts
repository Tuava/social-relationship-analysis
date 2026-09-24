/**
 * Identity helpers for person-like objects.
 *
 * `identifiers` arrives in two shapes depending on the endpoint:
 * - list endpoints: string[] of bare platform ids (e.g. ["10000001"])
 * - detail endpoints: { platform, value, confidence }[]
 *
 * qqOf resolves the canonical QQ in both shapes and returns "" when absent,
 * so callers never feed an identifier object into an avatar URL builder.
 */
export function qqOf(value: any): string {
  const ids = value?.identifiers;
  if (!Array.isArray(ids)) return "";
  const pair = ids.find((v: any) => v && typeof v === "object" && v.platform === "qq");
  if (pair?.value) return String(pair.value);
  const bare = ids.find((v: any) => typeof v === "string" && !v.includes(":"));
  if (bare) return bare;
  const obj = ids.find((v: any) => v && typeof v === "object" && v.value);
  if (obj?.value) return String(obj.value);
  return "";
}
