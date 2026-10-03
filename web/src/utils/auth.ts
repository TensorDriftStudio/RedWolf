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
