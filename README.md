# Synergia Pocket heartbeat

Independent Netlify scheduled function: GET the public production health endpoint every 10 minutes, every day. No GitHub Actions runner, credentials or school data. Only the production Pocket service is kept awake; Pocket TEST is unchanged.

Deploy this branch as a separate Netlify project (production branch `ops/heartbeat-netlify`, publish `public`). Check Functions → keep-warm → schedule and successful invocation logs. Invocation timeout is 25 seconds; a sleeping Render service may need a later invocation to complete its cold start. A configured cron is not a guarantee of uninterrupted availability; account limits and provider outages still apply.

Run the mock test: `node --test tests/heartbeat.test.mjs`.
