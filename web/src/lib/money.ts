// Money formatting. All money values in the API are integers (or floats, for
// projections) in the *minor unit* of the user's configured currency — pence,
// cents, yen — so display code converts to major units per currency here.
// SUPPORTED_CURRENCIES must stay in sync with supportedCurrencies in
// internal/api/settings_handlers.go.

export interface CurrencyOption {
	code: string;
	label: string;
	minorUnit: string;
}

export const SUPPORTED_CURRENCIES: CurrencyOption[] = [
	{ code: 'GBP', label: 'British Pound (£)', minorUnit: 'pence' },
	{ code: 'USD', label: 'US Dollar ($)', minorUnit: 'cents' },
	{ code: 'EUR', label: 'Euro (€)', minorUnit: 'cents' },
	{ code: 'JPY', label: 'Japanese Yen (¥)', minorUnit: 'yen' },
	{ code: 'CHF', label: 'Swiss Franc (CHF)', minorUnit: 'rappen' },
	{ code: 'CAD', label: 'Canadian Dollar (CA$)', minorUnit: 'cents' },
	{ code: 'AUD', label: 'Australian Dollar (A$)', minorUnit: 'cents' },
	{ code: 'NZD', label: 'New Zealand Dollar (NZ$)', minorUnit: 'cents' },
	{ code: 'SEK', label: 'Swedish Krona (kr)', minorUnit: 'öre' },
	{ code: 'NOK', label: 'Norwegian Krone (kr)', minorUnit: 'øre' },
	{ code: 'DKK', label: 'Danish Krone (kr)', minorUnit: 'øre' },
	{ code: 'PLN', label: 'Polish Złoty (zł)', minorUnit: 'grosze' },
	{ code: 'ZAR', label: 'South African Rand (R)', minorUnit: 'cents' },
	{ code: 'INR', label: 'Indian Rupee (₹)', minorUnit: 'paise' }
];

// currencyDecimals asks Intl how many minor-unit decimals a currency has
// (2 for GBP/USD, 0 for JPY), so minor→major conversion is never hard-coded.
export function currencyDecimals(code: string): number {
	try {
		return (
			new Intl.NumberFormat('en-GB', { style: 'currency', currency: code }).resolvedOptions()
				.maximumFractionDigits ?? 2
		);
	} catch {
		return 2;
	}
}

// formatMoneyMinor renders a minor-unit amount as a localised major-unit
// currency string, e.g. formatMoneyMinor(12345, 'GBP') → "£123.45".
// The locale matches formatNumber/formatDate in api.ts.
export function formatMoneyMinor(minor: number, code: string): string {
	const decimals = currencyDecimals(code);
	const major = minor / 10 ** decimals;
	try {
		return new Intl.NumberFormat('en-GB', { style: 'currency', currency: code }).format(major);
	} catch {
		return `${major.toFixed(decimals)} ${code}`;
	}
}

// minorUnitLabel names the minor unit for form labels ("pence per mile over").
export function minorUnitLabel(code: string): string {
	return SUPPORTED_CURRENCIES.find((c) => c.code === code)?.minorUnit ?? 'minor units';
}
