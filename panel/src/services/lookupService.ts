// services/lookupService.ts
// API service for GeoIP and WHOIS lookups

import type { GeoIPResult, WhoisResult, IPLookupResult } from '@/types';
import request from './request';

/**
 * Perform a GeoIP lookup for a single IP
 */
export async function lookupGeoIP(ip: string): Promise<GeoIPResult> {
    const { data } = await request.get<GeoIPResult>(`/geoip/lookup?ip=${encodeURIComponent(ip)}`);
    return data;
}

/**
 * Perform bulk GeoIP lookups for multiple IPs
 */
export async function bulkLookupGeoIP(ips: string[]): Promise<{ data: GeoIPResult[]; total: number }> {
    const { data } = await request.post<{ data: GeoIPResult[]; total: number }>('/geoip/lookup', { ips });
    return data;
}

/**
 * Get GeoIP lookup history for an IP
 */
export async function getGeoIPHistory(ip: string, limit = 10): Promise<{ data: GeoIPResult[]; total: number }> {
    const { data } = await request.get<{ data: GeoIPResult[]; total: number }>(
        `/geoip/history?ip=${encodeURIComponent(ip)}&limit=${limit}`
    );
    return data;
}

/**
 * Check GeoIP service status
 */
export async function getGeoIPStatus(): Promise<{ configured: boolean; city: boolean; country: boolean; asn: boolean }> {
    try {
        const { data } = await request.get<{ configured: boolean; city: boolean; country: boolean; asn: boolean }>('/geoip/status');
        return data;
    } catch {
        return { configured: false, city: false, country: false, asn: false };
    }
}

/**
 * Perform a WHOIS lookup
 */
export async function lookupWhois(query: string): Promise<WhoisResult> {
    const { data } = await request.get<WhoisResult>(`/whois/lookup?query=${encodeURIComponent(query)}`);
    return data;
}

/**
 * Get WHOIS lookup history
 */
export async function getWhoisHistory(query: string, limit = 10): Promise<{ data: WhoisResult[]; total: number }> {
    const { data } = await request.get<{ data: WhoisResult[]; total: number }>(
        `/whois/history?query=${encodeURIComponent(query)}&limit=${limit}`
    );
    return data;
}

/**
 * Perform a combined GeoIP + WHOIS lookup
 */
export async function lookupCombined(ip: string): Promise<IPLookupResult> {
    const { data } = await request.get<IPLookupResult>(`/lookup/combined?ip=${encodeURIComponent(ip)}`);
    return data;
}

/**
 * Get country flag emoji from country code
 */
export function countryCodeToFlag(code: string): string {
    if (!code || code.length !== 2) return '🌐';
    const codePoints = code
        .toUpperCase()
        .split('')
        .map(char => 127397 + char.charCodeAt(0));
    return String.fromCodePoint(...codePoints);
}

/**
 * Format ASN for display
 */
export function formatASN(asn?: { number?: number; organization?: string }): string {
    if (!asn) return '-';
    if (asn.number && asn.organization) {
        return `AS${asn.number} (${asn.organization})`;
    }
    if (asn.number) return `AS${asn.number}`;
    if (asn.organization) return asn.organization;
    return '-';
}

/**
 * Format location for display
 */
export function formatLocation(geoip?: GeoIPResult): string {
    if (!geoip) return '-';
    const parts: string[] = [];

    if (geoip.city?.name) parts.push(geoip.city.name);
    if (geoip.city?.subdivision) parts.push(geoip.city.subdivision);
    if (geoip.country?.name) parts.push(geoip.country.name);

    return parts.length > 0 ? parts.join(', ') : '-';
}

/**
 * Validate IP address format
 */
export function isValidIP(ip: string): boolean {
    // IPv4
    const ipv4Regex = /^(?:(?:25[0-5]|2[0-4]\d|[01]?\d\d?)\.){3}(?:25[0-5]|2[0-4]\d|[01]?\d\d?)$/;
    // IPv6 (simplified)
    const ipv6Regex = /^(?:[a-fA-F0-9]{1,4}:){7}[a-fA-F0-9]{1,4}$|^::(?:[a-fA-F0-9]{1,4}:){0,6}[a-fA-F0-9]{1,4}$|^(?:[a-fA-F0-9]{1,4}:){1,7}:$/;

    return ipv4Regex.test(ip) || ipv6Regex.test(ip);
}
