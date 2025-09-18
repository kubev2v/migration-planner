import type { ChromeAPI } from "@redhat-cloud-services/types";

// Local development token management
const getLocalToken = (): string | null => {
  // Try to get token from localStorage (can be set via browser console)
  const localToken = localStorage.getItem("MIGRATION_PLANNER_TOKEN");
  if (localToken) {
    return localToken;
  }

  // Default to no token (will cause 401, user needs to set token)
  return null;
};

export const createAuthFetch = (chrome: ChromeAPI): typeof fetch => {
  return async (input: RequestInfo, init: RequestInit = {}) => {
    // Nos aseguramos de crear headers a partir de init.headers o un objeto vacío
    const headers = new Headers(init.headers || {});

    // Standalone mode is defined by webpack.DefinePlugin as a boolean literal
    if (process.env.STANDALONE_MODE) {
      const localToken = getLocalToken();
      if (localToken) {
        headers.set("X-Authorization", `Bearer ${localToken}`);
      }
      // If no local token, make request without auth (will get 401)
    } else {
      // Production mode - use Chrome API
      const token = await chrome.auth.getToken();
      headers.set("X-Authorization", `Bearer ${token}`);
    }

    return fetch(input, {
      ...init,
      headers,
    });
  };
};
