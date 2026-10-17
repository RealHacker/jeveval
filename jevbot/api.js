export const ENDPOINT = 'https://api.typesafe.ai/v1/systemone';

export async function askJev(body, apiKey, signal, fetcher = fetch) {
  const timeout = AbortSignal.timeout(60000);
  const combined = signal ? AbortSignal.any([signal, timeout]) : timeout;
  let response;
  try {
    response = await fetcher(ENDPOINT, {
      method: 'POST', signal: combined,
      headers: { Authorization: `Bearer ${apiKey}`, 'Content-Type': 'application/json', 'TypeSafe-Danger-Zone-Images': 'true' },
      body: JSON.stringify(body)
    });
  } catch (error) {
    if (signal?.aborted) throw new DOMException('Run stopped.', 'AbortError');
    if (timeout.aborted) throw new Error('Jev request timed out after 60 seconds.');
    throw new Error('Could not reach TypeSafe. Check your connection and try again.');
  }
  // Do not echo response bodies: gateways may reflect credentials or screenshots.
  if (!response.ok) throw new Error(`TypeSafe HTTP ${response.status}. ${response.status === 401 || response.status === 403 ? 'Check the API key in Settings.' : response.status === 429 ? 'Rate limit reached; wait before starting a new run.' : 'Request failed; check model availability and image support.'}`);
  let data;
  try { data = await response.json(); } catch { throw new Error('TypeSafe returned invalid JSON.'); }
  if (!data?.answers?.next) throw new Error('TypeSafe response is missing answers.next.');
  return data.answers.next;
}
