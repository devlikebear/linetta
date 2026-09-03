package codexauth

// successPage is what the writer's browser shows when the login lands. It is
// served from the local callback rather than redirecting to OpenAI's hosted
// page: the writer's next step is in Linetta, and a hosted page would leave
// them looking at somebody else's brand wondering whether it worked.
const successPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Signed in</title>
<style>
  body { font: 16px/1.6 -apple-system, "Segoe UI", system-ui, sans-serif;
         display: grid; place-items: center; min-height: 100vh; margin: 0;
         color: #2b2722; background: #f7f4ee; }
  main { text-align: center; padding: 2rem; }
  h1 { font-size: 1.25rem; font-weight: 600; margin: 0 0 .5rem; }
  p { margin: 0; color: #6b635a; }
</style></head>
<body><main>
  <h1>Signed in to ChatGPT</h1>
  <p>You can close this tab and return to Linetta.</p>
</main></body></html>`

// failurePage explains a callback Linetta refused or could not complete.
const failurePage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<title>Sign-in failed</title>
<style>
  body { font: 16px/1.6 -apple-system, "Segoe UI", system-ui, sans-serif;
         display: grid; place-items: center; min-height: 100vh; margin: 0;
         color: #2b2722; background: #f7f4ee; }
  main { text-align: center; padding: 2rem; }
  h1 { font-size: 1.25rem; font-weight: 600; margin: 0 0 .5rem; }
  p { margin: 0; color: #6b635a; }
</style></head>
<body><main>
  <h1>Sign-in did not complete</h1>
  <p>Close this tab and try again from Linetta.</p>
</main></body></html>`
