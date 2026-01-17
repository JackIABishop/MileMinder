// API client for MileMinder backend

const API_BASE = '/api';

export interface VehicleListItem {
	id: string;
	vehicle: string;
	is_default: boolean;
}

export interface VehicleStatus {
	id: string;
	vehicle: string;
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
	projected_end: number;
	projected_over: boolean;
	plan_start: string;
	plan_end: string;
	annual_allowance: number;
	start_miles: number;
	is_default: boolean;
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
	start_date: string;
	end_date: string;
	annual_allowance: number;
	start_miles: number;
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

	if (!response.ok) {
		const text = await response.text();
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
export async function getFleet(): Promise<VehicleStatus[]> {
	return fetchJSON<VehicleStatus[]>(`${API_BASE}/fleet`);
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
