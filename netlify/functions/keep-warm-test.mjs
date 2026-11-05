// Only the public health endpoint; no account data or credentials.
export default async () => {
  const started = Date.now();
  const response = await fetch('https://synergia-pocket-own-api-test.onrender.com/api/health', {
    headers: {'User-Agent': 'Synergia-Pocket-Heartbeat/2.0'},
    redirect: 'error', signal: AbortSignal.timeout(25000), cache: 'no-store',
  });
  if (!response.ok) throw new Error(`Health check HTTP ${response.status}`);
  const health = await response.json();
  if (health.status !== 'ok' || health.service !== 'synergia-pocket-own-api') {
    throw new Error('Unexpected health response');
  }
  console.log(JSON.stringify({event:'heartbeat-test', ok:true, elapsed_ms:Date.now()-started}));
  return new Response(null, {status:204});
};
// Every ten minutes, every day. UTC; interval is independent of DST.
export const config = {schedule: '3,13,23,33,43,53 * * * *'};
