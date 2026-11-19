// Only the public health endpoint; no account data or credentials.
// Pause scheduled Render wake-ups through November 2026, but keep manual access
// to the app unchanged. 2026-11-30 23:00 UTC = 2026-12-01 00:00 in Poland (CET).
const AUTO_WAKE_RESUMES_AT_UTC = Date.parse('2026-11-30T23:00:00.000Z');

export default async () => {
  if (Date.now() < AUTO_WAKE_RESUMES_AT_UTC) {
    console.log(JSON.stringify({
      event: 'heartbeat',
      skipped: true,
      reason: 'temporary_november_pause',
      resumes_at_utc: '2026-11-30T23:00:00.000Z',
    }));
    return new Response(null, {status: 204});
  }

  const started = Date.now();
  const response = await fetch('https://synergia-pocket.onrender.com/api/health', {
    headers: {'User-Agent': 'Synergia-Pocket-Heartbeat/2.0'},
    redirect: 'error', signal: AbortSignal.timeout(25000), cache: 'no-store',
  });
  if (!response.ok) throw new Error(`Health check HTTP ${response.status}`);
  const health = await response.json();
  if (health.status !== 'ok' || health.service !== 'synergia-pocket') {
    throw new Error('Unexpected health response');
  }
  console.log(JSON.stringify({event:'heartbeat', ok:true, elapsed_ms:Date.now()-started}));
  return new Response(null, {status:204});
};
// Every ten minutes; requests to Render resume automatically on 1 December 2026 (CET).
export const config = {schedule: '3,13,23,33,43,53 * * * *'};
