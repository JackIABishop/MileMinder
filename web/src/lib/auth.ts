// Auth actions: the imperative side of the session state in session.ts. Kept
// separate from api.ts so the API client depends only on the state (session.ts),
// not on these actions — no import cycle.
import * as api from './api';
import { mode, user, authReady } from './session';

export { mode, user, authReady };

// initAuth resolves the server mode once on boot and, in hosted mode, loads the
// current user (leaving it null if no valid session). Safe to call once from the
// root layout's onMount.
export async function initAuth(): Promise<void> {
	try {
		const meta = await api.getMeta();
		mode.set(meta.mode);
		if (meta.mode === 'hosted') {
			try {
				user.set(await api.getMe());
			} catch {
				user.set(null);
			}
		}
	} catch (e) {
		console.error('Failed to determine server mode:', e);
	} finally {
		authReady.set(true);
	}
}

export async function login(email: string, password: string): Promise<void> {
	const res = await api.login(email, password);
	user.set(res.user);
}

export async function signup(email: string, password: string): Promise<void> {
	const res = await api.signup(email, password);
	user.set(res.user);
}

export async function logout(): Promise<void> {
	try {
		await api.logout();
	} finally {
		user.set(null);
	}
}
