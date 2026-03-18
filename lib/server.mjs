/**
 * server.mjs — HTTP server for ctx-monitor --serve mode.
 *
 * Provides a JSON API and serves the HTML dashboard.
 * Zero npm dependencies; Node.js built-ins only.
 */

import { createServer } from 'node:http';

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

function jsonResponse(res, statusCode, data) {
  const body = JSON.stringify(data, null, 2);
  res.writeHead(statusCode, {
    'Content-Type': 'application/json; charset=utf-8',
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Methods': 'GET, OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type',
    'Cache-Control': 'no-cache',
  });
  res.end(body);
}

function htmlResponse(res, statusCode, html) {
  res.writeHead(statusCode, {
    'Content-Type': 'text/html; charset=utf-8',
    'Access-Control-Allow-Origin': '*',
    'Cache-Control': 'no-cache',
  });
  res.end(html);
}

function corsPreflightResponse(res) {
  res.writeHead(204, {
    'Access-Control-Allow-Origin': '*',
    'Access-Control-Allow-Methods': 'GET, OPTIONS',
    'Access-Control-Allow-Headers': 'Content-Type',
    'Access-Control-Max-Age': '86400',
  });
  res.end();
}

function notFound(res) {
  jsonResponse(res, 404, { error: 'Not found' });
}

function errorResponse(res, err) {
  const message = err instanceof Error ? err.message : String(err);
  jsonResponse(res, 500, { error: message });
}

/**
 * Parse a URL pathname and extract route segments.
 * Returns { pathname, segments }.
 */
function parseRoute(url) {
  let pathname;
  try {
    pathname = new URL(url, 'http://localhost').pathname;
  } catch {
    pathname = url.split('?')[0] || '/';
  }
  // Normalize: remove trailing slash (except root)
  if (pathname.length > 1 && pathname.endsWith('/')) {
    pathname = pathname.slice(0, -1);
  }
  const segments = pathname.split('/').filter(Boolean);
  return { pathname, segments };
}

// ---------------------------------------------------------------------------
// Dynamic HTML template import
// ---------------------------------------------------------------------------

let _getHtmlTemplate = null;

async function loadHtmlTemplate() {
  if (_getHtmlTemplate) return _getHtmlTemplate;
  try {
    const mod = await import('./renderer/html-template.mjs');
    _getHtmlTemplate = mod.getHtmlTemplate;
    return _getHtmlTemplate;
  } catch {
    // html-template.mjs may not exist yet; return a fallback
    return null;
  }
}

// ---------------------------------------------------------------------------
// Route handler
// ---------------------------------------------------------------------------

async function handleRequest(req, res, callbacks) {
  const { getComposition, getSessions, getSessionById, getTimeline } = callbacks;

  // Handle CORS preflight
  if (req.method === 'OPTIONS') {
    corsPreflightResponse(res);
    return;
  }

  // Only accept GET
  if (req.method !== 'GET') {
    jsonResponse(res, 405, { error: 'Method not allowed' });
    return;
  }

  const { pathname, segments } = parseRoute(req.url);

  try {
    // -----------------------------------------------------------------------
    // GET / — HTML dashboard
    // -----------------------------------------------------------------------
    if (pathname === '/') {
      const getTemplate = await loadHtmlTemplate();
      if (getTemplate) {
        const html = getTemplate();
        htmlResponse(res, 200, html);
      } else {
        htmlResponse(res, 200, fallbackHtml());
      }
      return;
    }

    // -----------------------------------------------------------------------
    // GET /api/data — Current context composition
    // -----------------------------------------------------------------------
    if (pathname === '/api/data') {
      const data = typeof getComposition === 'function' ? await getComposition() : null;
      jsonResponse(res, 200, data ?? { error: 'No composition data available' });
      return;
    }

    // -----------------------------------------------------------------------
    // GET /api/sessions — List available sessions
    // -----------------------------------------------------------------------
    if (pathname === '/api/sessions') {
      const sessions = typeof getSessions === 'function' ? await getSessions() : [];
      jsonResponse(res, 200, sessions);
      return;
    }

    // -----------------------------------------------------------------------
    // GET /api/session/:id — Full parsed data for a specific session
    // -----------------------------------------------------------------------
    if (segments.length === 3 && segments[0] === 'api' && segments[1] === 'session') {
      const id = decodeURIComponent(segments[2]);
      if (typeof getSessionById === 'function') {
        const data = await getSessionById(id);
        if (data) {
          jsonResponse(res, 200, data);
        } else {
          jsonResponse(res, 404, { error: `Session not found: ${id}` });
        }
      } else {
        jsonResponse(res, 501, { error: 'getSessionById not implemented' });
      }
      return;
    }

    // -----------------------------------------------------------------------
    // GET /api/timeline/:id — Event-by-event token growth timeline
    // -----------------------------------------------------------------------
    if (segments.length === 3 && segments[0] === 'api' && segments[1] === 'timeline') {
      const id = decodeURIComponent(segments[2]);
      if (typeof getTimeline === 'function') {
        const data = await getTimeline(id);
        if (data) {
          jsonResponse(res, 200, data);
        } else {
          jsonResponse(res, 404, { error: `Timeline not found: ${id}` });
        }
      } else {
        jsonResponse(res, 501, { error: 'getTimeline not implemented' });
      }
      return;
    }

    // -----------------------------------------------------------------------
    // 404 — Unknown route
    // -----------------------------------------------------------------------
    notFound(res);

  } catch (err) {
    errorResponse(res, err);
  }
}

// ---------------------------------------------------------------------------
// Fallback HTML when html-template.mjs is not available
// ---------------------------------------------------------------------------

function fallbackHtml() {
  return `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>ctx-monitor</title>
  <style>
    body { font-family: system-ui, sans-serif; max-width: 640px; margin: 60px auto; color: #333; }
    h1 { color: #D4603A; }
    code { background: #f5f5f5; padding: 2px 6px; border-radius: 3px; }
    .endpoint { margin: 8px 0; }
  </style>
</head>
<body>
  <h1>ctx-monitor</h1>
  <p>Dashboard template not found. The API is available:</p>
  <div class="endpoint"><code>GET /api/data</code> — Current context composition</div>
  <div class="endpoint"><code>GET /api/sessions</code> — List sessions</div>
  <div class="endpoint"><code>GET /api/session/:id</code> — Session details</div>
  <div class="endpoint"><code>GET /api/timeline/:id</code> — Token growth timeline</div>
</body>
</html>`;
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/**
 * Start the HTTP server for the ctx-monitor dashboard.
 *
 * @param {object} options
 * @param {number}   [options.port=3456]          — Port to listen on
 * @param {function} [options.getComposition]      — Returns current composition data
 * @param {function} [options.getSessions]          — Returns list of available sessions
 * @param {function} [options.getSessionById]       — Returns parsed data for a session ID
 * @param {function} [options.getTimeline]          — Returns token growth timeline for a session ID
 * @returns {import('http').Server} The HTTP server instance
 */
export function startServer({ port = 3456, getComposition, getSessions, getSessionById, getTimeline } = {}) {
  const callbacks = { getComposition, getSessions, getSessionById, getTimeline };

  const server = createServer((req, res) => {
    handleRequest(req, res, callbacks).catch((err) => {
      try {
        errorResponse(res, err);
      } catch {
        // Response may already be sent; ignore
      }
    });
  });

  server.on('error', (err) => {
    if (err.code === 'EADDRINUSE') {
      process.stderr.write(`Error: Port ${port} is already in use.\n`);
      process.exit(1);
    }
    throw err;
  });

  server.listen(port, () => {
    process.stderr.write(`ctx-monitor dashboard: http://localhost:${port}\n`);
  });

  return server;
}
