// Hash-route normalization.
//
// The app itself only ever writes bare hashes ("#<sid>", "#scheduled", "#run:<id>",
// "" for home), but links people share, bookmark or type by hand use the path-ish
// forms — "#/", "#/scheduled", "#/s/<sid>". Those must resolve to the same route
// instead of being read as a session id (which renders "Session not found").
export function normalizeRoute(raw: string): string {
  return raw
    .replace(/^\/+/, "") // "#/scheduled" → "scheduled", "#/" → ""
    .replace(/^s\/+/, "") // "#/s/<sid>" → "<sid>"
    .replace(/\/+$/, ""); // tolerate a trailing slash
}
