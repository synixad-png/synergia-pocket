import test from 'node:test';
import assert from 'node:assert/strict';
import heartbeat, {config} from '../netlify/functions/keep-warm.mjs';
test('scheduled health request uses only the public endpoint and rejects failures', async () => {
  const original = globalThis.fetch;
  try {
    globalThis.fetch = async (url, opts) => {
      assert.equal(url, 'https://synergia-pocket.onrender.com/api/health');
      assert.equal(opts.redirect, 'error');
      assert.ok(opts.signal);
      assert.deepEqual(Object.keys(opts.headers), ['User-Agent']);
      return Response.json({status:'ok',service:'synergia-pocket'});
    };
    assert.equal((await heartbeat()).status, 204);
    globalThis.fetch = async () => new Response('', {status:503});
    await assert.rejects(heartbeat, /HTTP 503/);
    globalThis.fetch = async () => Response.json({status:'ok',service:'wrong'});
    await assert.rejects(heartbeat, /Unexpected/);
    assert.equal(config.schedule, '3,13,23,33,43,53 * * * *');
  } finally {globalThis.fetch = original;}
});
