// Shared auth state. Kept free of runtime imports from api.ts (only type-only
// imports, which are erased) so the API client can read `mode` for its 401
// handling without a circular dependency.
import { writable } from 'svelte/store';
import type { ServerMode, User } from './api';

// Server mode, resolved once on boot from /api/v1/meta. null until known.
export const mode = writable<ServerMode | null>(null);

// The authenticated user in hosted mode, or null when signed out / single-user.
export const user = writable<User | null>(null);

// Flips true once initAuth has resolved mode (and, in hosted mode, attempted to
// load the current user), so the layout can avoid a chrome/login flash.
export const authReady = writable(false);
