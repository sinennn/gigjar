export function onRequest({ request, env }) {
  return env.WEB.fetch(request)
}
