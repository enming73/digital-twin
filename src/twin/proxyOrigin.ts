const allowed = new Set(['http://localhost:4173', 'http://127.0.0.1:4173', 'http://localhost:5173', 'http://127.0.0.1:5173'])
export function isAllowedOrigin(origin: string | undefined): boolean { return origin !== undefined && allowed.has(origin) }
