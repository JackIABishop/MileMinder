// API client for MileMinder backend

import { goto } from '$app/navigation';
import { get } from 'svelte/store';
import { mode } from './session';

const API_BASE = '/api/v1';

export interface VehicleListItem {
	id: string;
	vehicle: string;
	is_default: boolean;
}

export interface VehicleStatus {
	id: string;
	vehicle: string;
	has_plan: boolean;
	latest_reading: number;
	latest_date: string;
	target_today: number;
	delta: number;
	percent_used: number;
	days_left_year: number;
	miles_left_year: number;
	days_left_term: number;
	miles_left_term: number;
	years_left_term: number;
	daily_rate: number;
	avg_annual_mileage: number;
	recent_annual_mileage: number;
	projected_end: number;
	projected_over: boolean;
	plan_start: string;
	plan_end: string;
	annual_allowance: number;
	start_miles: number;
	is_default: boolean;
	// Renewal countdown + final-mileage estimate (#3)
	days_to_end: number;
	estimated_final_mileage: number;
	// Drivable-rate budget (#4)
	drivable_daily_rate: number;
	// Overage cost estimate (#5) — excess_rate omitted from JSON when 0 (no rate set);
	// the projected figures are always present (cost is 0 without a rate).
	excess_rate?: number;
	projected_excess_miles: number;
	projected_overage_cost: number;
	// Trend signal (#7)
	pace_trend_delta: number;
	pace_trend: string;
}

// Household roll-up over all vehicles. Mirrors calc.FleetInsights (Go).
// "Worst offender" and comparative pace are ranked by percent_used.
export interface FleetInsights {
	total_vehicles: number;
	policy_vehicles: number;
	plain_vehicles: number;
	count_over: number;
	count_under: number;
	net_delta: number; // miles; +ve = household collectively over the allowance line
	total_avg_annual_mileage: number;
	avg_percent_used: number;
	worst_offender_id: string; // "" when there are no vehicles
	worst_offender_vehicle: string;
}

export interface FleetResponse {
	vehicles: VehicleStatus[];
	insights: FleetInsights;
}

export interface Reading {
	date: string;
	miles: number;
}

export interface GraphData {
	dates: string[];
	actuals: number[];
	ideals: number[];
}

export interface CreateVehicleRequest {
	id: string;
	vehicle: string;
	start_date?: string;
	end_date?: string;
	annual_allowance?: number;
	start_miles: number;
	excess_rate?: number;
}

export interface UpdatePlanRequest {
	excess_rate?: number;
	start_date?: string;
	end_date?: string;
	annual_allowance?: number;
	start_miles?: number;
}

export interface AddReadingRequest {
	date?: string;
	miles: number;
	force?: boolean;
}

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
	const response = await fetch(url, {
		...options,
		headers: {
			'Content-Type': 'application/json',
			...options?.headers
		}
	});

	// In hosted mode a 401 on a data endpoint means the session lapsed: bounce to
	// the login page. Auth endpoints (/auth/*) are excluded — the login page shows
	// their errors inline rather than redirecting to itself.
	if (response.status === 401 && get(mode) === 'hosted' && !url.includes('/auth/')) {
		goto('/login');
		throw new Error('authentication required');
	}

	if (!response.ok) {
		const text = await response.text();
		let parsed: any = null;
		try {
			parsed = JSON.parse(text);
		} catch {
			parsed = null;
		}
		if (parsed?.error?.message) {
			const code = parsed.error.code ? `${parsed.error.code}: ` : '';
			throw new Error(`${code}${parsed.error.message}`);
		}
		throw new Error(text || `HTTP ${response.status}`);
	}

	return response.json();
}

// Vehicle endpoints
export async function listVehicles(): Promise<VehicleListItem[]> {
	return fetchJSON<VehicleListItem[]>(`${API_BASE}/vehicles`);
}

export async function getVehicle(id: string): Promise<VehicleStatus> {
	return fetchJSON<VehicleStatus>(`${API_BASE}/vehicles/${encodeURIComponent(id)}`);
}

export async function createVehicle(data: CreateVehicleRequest): Promise<{ status: string; id: string }> {
	return fetchJSON(`${API_BASE}/vehicles`, {
		method: 'POST',
		body: JSON.stringify(data)
	});
}

export async function updatePlan(id: string, data: UpdatePlanRequest): Promise<{ status: string; id: string }> {
	return fetchJSON(`${API_BASE}/vehicles/${encodeURIComponent(id)}`, {
		method: 'PATCH',
		body: JSON.stringify(data)
	});
}

// Reading endpoints
export async function addReading(vehicleId: string, data: AddReadingRequest): Promise<{ status: string; date: string; miles: number }> {
	return fetchJSON(`${API_BASE}/vehicles/${encodeURIComponent(vehicleId)}/readings`, {
		method: 'POST',
		body: JSON.stringify(data)
	});
}

export async function getReadings(vehicleId: string): Promise<Reading[]> {
	return fetchJSON<Reading[]>(`${API_BASE}/vehicles/${encodeURIComponent(vehicleId)}/readings`);
}

export async function deleteReading(vehicleId: string, date: string): Promise<{ status: string }> {
	return fetchJSON(`${API_BASE}/vehicles/${encodeURIComponent(vehicleId)}/readings/${encodeURIComponent(date)}`, {
		method: 'DELETE'
	});
}

// Graph data
export async function getGraphData(vehicleId: string): Promise<GraphData> {
	return fetchJSON<GraphData>(`${API_BASE}/vehicles/${encodeURIComponent(vehicleId)}/graph`);
}

// Current vehicle
export async function getCurrentVehicle(): Promise<{ current: string }> {
	return fetchJSON<{ current: string }>(`${API_BASE}/current`);
}

export async function setCurrentVehicle(id: string): Promise<{ status: string; current: string }> {
	return fetchJSON(`${API_BASE}/current`, {
		method: 'PUT',
		body: JSON.stringify({ id })
	});
}

// Fleet
export async function getFleet(): Promise<FleetResponse> {
	return fetchJSON<FleetResponse>(`${API_BASE}/fleet`);
}

// Server mode. "single-user" is the self-hosted / local binary (no auth UI);
// "hosted" is the multi-tenant deployment (login required to reach data).
export type ServerMode = 'single-user' | 'hosted';

export interface Meta {
	mode: ServerMode;
}

// getMeta is called on boot, before any login, to decide whether to show the
// login flow. It requires no authentication.
export async function getMeta(): Promise<Meta> {
	return fetchJSON<Meta>(`${API_BASE}/meta`);
}

// Auth (hosted mode only)
export interface User {
	id: string;
	email: string;
	created_at: string;
}

export interface AuthResponse {
	token: string;
	user: User;
}

export async function login(email: string, password: string): Promise<AuthResponse> {
	return fetchJSON<AuthResponse>(`${API_BASE}/auth/login`, {
		method: 'POST',
		body: JSON.stringify({ email, password })
	});
}

export async function signup(email: string, password: string): Promise<AuthResponse> {
	return fetchJSON<AuthResponse>(`${API_BASE}/auth/signup`, {
		method: 'POST',
		body: JSON.stringify({ email, password })
	});
}

export async function logout(): Promise<void> {
	await fetchJSON(`${API_BASE}/auth/logout`, { method: 'POST' });
}

export async function getMe(): Promise<User> {
	return fetchJSON<User>(`${API_BASE}/auth/me`);
}

// Export CSV (returns download URL)
export function getExportURL(vehicleId: string): string {
	return `${API_BASE}/vehicles/${encodeURIComponent(vehicleId)}/export`;
}

// Utility functions
export function formatDate(dateString: string): string {
	if (!dateString) return '-';
	const date = new Date(dateString);
	return date.toLocaleDateString('en-GB', {
		day: '2-digit',
		month: 'short',
		year: 'numeric'
	});
}

export function formatNumber(num: number, decimals: number = 0): string {
	return num.toLocaleString('en-GB', {
		minimumFractionDigits: decimals,
		maximumFractionDigits: decimals
	});
}

export function getStatusColor(percentUsed: number): 'green' | 'amber' | 'red' {
	if (percentUsed <= 90) return 'green';
	if (percentUsed <= 100) return 'amber';
	return 'red';
}

export function getDeltaStatus(delta: number): { color: string; icon: string; label: string } {
	if (delta <= 0) {
		return {
			color: 'text-gauge-green',
			icon: '✓',
			label: 'Within allowance'
		};
	}
	if (delta <= 500) {
		return {
			color: 'text-gauge-amber',
			icon: '⚠',
			label: 'Slightly over limit'
		};
	}
	return {
		color: 'text-gauge-red',
		icon: '✗',
		label: 'Over limit'
	};
}
