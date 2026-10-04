/**
 * Event name dispatched when any request encounters a 401 or 403 unauthorized status.
 */
export const AUTH_UNAUTHORIZED_EVENT = 'redwolf:unauthorized';

/**
 * Dispatches an event to notify the application of authentication invalidation.
 */
export function notifyUnauthorized(): void {
  window.dispatchEvent(new Event(AUTH_UNAUTHORIZED_EVENT));
}

/**
 * Retrieves standard authorization bearer header for API requests.
 */
export function getAuthHeaders(): Record<string, string> {
  const token = localStorage.getItem('redwolf_token');
  if (token) {
    return { Authorization: `Bearer ${token}` };
  }
  return {};
}
