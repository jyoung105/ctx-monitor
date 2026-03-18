/**
 * claude-usage-api.mjs — Optional OAuth usage API client for Claude Code plan limits.
 *
 * Fetches utilization data from the Anthropic OAuth usage endpoint.
 * Tries macOS Keychain first, falls back to ~/.claude/.credentials.json.
 *
 * Zero npm dependencies; Node.js built-ins only.
 */

import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { homedir, platform } from 'node:os';
import { execSync } from 'node:child_process';
import https from 'node:https';

// ---------------------------------------------------------------------------
// Constants
// ---------------------------------------------------------------------------

const USAGE_URL = 'https://api.anthropic.com/api/oauth/usage';
const BETA_HEADER = 'oauth-2025-04-20';
const CACHE_TTL_MS = 60_000; // 60 seconds
const REQUEST_TIMEOUT_MS = 10_000;

// ---------------------------------------------------------------------------
// Cache
// ---------------------------------------------------------------------------

let _cache = null;
let _cacheTime = 0;

// ---------------------------------------------------------------------------
// Credential retrieval
// ---------------------------------------------------------------------------

/**
 * Attempt to read the OAuth access token from macOS Keychain.
 * Returns null on non-macOS or if the keychain entry doesn't exist.
 */
function readFromKeychain() {
  if (platform() !== 'darwin') return null;

  try {
    const raw = execSync(
      'security find-generic-password -s "Claude Code-credentials" -w',
      { encoding: 'utf8', timeout: 5000, stdio: ['pipe', 'pipe', 'pipe'] },
    ).trim();

    if (!raw) return null;

    // The keychain value may be the token directly or a JSON blob
    try {
      const parsed = JSON.parse(raw);
      return parsed?.claudeAiOauth?.accessToken
        ?? parsed?.accessToken
        ?? null;
    } catch {
      // Not JSON — treat the raw string as the token
      return raw;
    }
  } catch {
    return null;
  }
}

/**
 * Read the OAuth access token from ~/.claude/.credentials.json.
 * Returns null if the file doesn't exist or lacks the expected field.
 */
function readFromCredentialsFile() {
  try {
    const filePath = join(homedir(), '.claude', '.credentials.json');
    const raw = readFileSync(filePath, 'utf8');
    const parsed = JSON.parse(raw);
    return parsed?.claudeAiOauth?.accessToken ?? null;
  } catch {
    return null;
  }
}

/**
 * Get the OAuth access token for the Anthropic usage API.
 *
 * Resolution order:
 *   1. macOS Keychain (Claude Code-credentials)
 *   2. ~/.claude/.credentials.json → claudeAiOauth.accessToken
 *
 * @returns {string|null} The access token, or null if unavailable.
 */
export function getCredentials() {
  // Try Keychain first (macOS only)
  const keychainToken = readFromKeychain();
  if (keychainToken) return keychainToken;

  // Fallback to credentials file
  return readFromCredentialsFile();
}

// ---------------------------------------------------------------------------
// HTTPS request helper
// ---------------------------------------------------------------------------

/**
 * Make a GET request using Node.js built-in https module.
 * Returns a promise that resolves with { statusCode, body }.
 */
function httpsGet(url, headers) {
  return new Promise((resolve, reject) => {
    const urlObj = new URL(url);

    const options = {
      hostname: urlObj.hostname,
      port: urlObj.port || 443,
      path: urlObj.pathname + urlObj.search,
      method: 'GET',
      headers,
      timeout: REQUEST_TIMEOUT_MS,
    };

    const req = https.request(options, (res) => {
      const chunks = [];
      res.on('data', (chunk) => chunks.push(chunk));
      res.on('end', () => {
        const body = Buffer.concat(chunks).toString('utf8');
        resolve({ statusCode: res.statusCode, body });
      });
    });

    req.on('error', reject);
    req.on('timeout', () => {
      req.destroy();
      reject(new Error('Request timed out'));
    });

    req.end();
  });
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Fetch plan usage data from the Anthropic OAuth usage API.
 *
 * Returns the usage breakdown or null on failure. Responses are cached
 * for 60 seconds to avoid excessive API calls.
 *
 * @returns {Promise<Object|null>} Plan usage data:
 *   {
 *     five_hour:   { utilization: number, resets_at: string|null },
 *     seven_day:   { utilization: number, resets_at: string|null },
 *     seven_day_opus: { utilization: number, resets_at: string|null },
 *     extra_usage: { is_enabled: boolean, monthly_limit: number|null, used_credits: number|null }
 *   }
 */
export async function fetchPlanUsage() {
  // Return cached result if still fresh
  const now = Date.now();
  if (_cache && (now - _cacheTime) < CACHE_TTL_MS) {
    return _cache;
  }

  const token = getCredentials();
  if (!token) {
    return null;
  }

  try {
    const { statusCode, body } = await httpsGet(USAGE_URL, {
      'Authorization': `Bearer ${token}`,
      'anthropic-beta': BETA_HEADER,
      'Accept': 'application/json',
    });

    if (statusCode !== 200) {
      return null;
    }

    const data = JSON.parse(body);

    // Update cache
    _cache = data;
    _cacheTime = now;

    return data;
  } catch {
    return null;
  }
}
