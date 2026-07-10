// User-level preferences (currency, distance unit), fetched once on boot.
// Kept separate from session.ts, which only tracks server mode and auth.

import { writable } from 'svelte/store';
import { getSettings, type Settings } from './api';

export const settings = writable<Settings>({ currency: 'GBP', distance_unit: 'mi' });

// loadSettings refreshes the store from the server. Failures are swallowed so
// a broken /settings fetch can't take down boot — the GBP/mi defaults match
// the server's own fallback.
export async function loadSettings(): Promise<void> {
	try {
		settings.set(await getSettings());
	} catch {
		// keep defaults
	}
}
