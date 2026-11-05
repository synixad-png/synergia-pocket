# Synergia Pocket heartbeat

Independent Netlify scheduled function: GET the public production health endpoint every 10 minutes, every day. No GitHub Actions runner, credentials or school data. Both production Pocket and Pocket TEST are kept awake by separate scheduled functions. The second function keep-warm-test requests https://synergia-pocket-own-api-test.onrender.com/api/health and checks service=synergia-pocket-own-api. Both run every ten minutes; a failure in one does not cancel the other.

Deploy this branch as a separate Netlify project (production branch `ops/heartbeat-netlify`, publish `public`). Check Functions → keep-warm → schedule and successful invocation logs. Invocation timeout is 25 seconds; a sleeping Render service may need a later invocation to complete its cold start. A configured cron is not a guarantee of uninterrupted availability; account limits and provider outages still apply.

Run the mock test: `node --test tests/heartbeat.test.mjs`.
