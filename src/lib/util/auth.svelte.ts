import { base } from '$app/paths';
import { apiLogin, apiLogout, apiRegister, setUnauthorizedHandler, type User } from '$/util/api';

let user: User | null | undefined = $state.raw(undefined);

export const authState = {
  get current(): User | null | undefined {
    return user;
  }
};

export const fetchMe = async (): Promise<User | null> => {
  try {
    const response = await fetch('/api/auth/me');
    user = response.ok ? ((await response.json()) as User) : null;
  } catch {
    user = null;
  }
  return user;
};

export const login = async (email: string, password: string): Promise<User> => {
  user = await apiLogin(email, password);
  return user;
};

export const register = async (email: string, password: string): Promise<User> => {
  user = await apiRegister(email, password);
  return user;
};

export const logout = async (): Promise<void> => {
  await apiLogout().catch(() => undefined);
  user = null;
};

// 401 from any storage API call means the session died: drop state and go to
// the login page, preserving the current location for post-login return.
setUnauthorizedHandler(() => {
  user = null;
  if (location.pathname === `${base}/login`) {
    return; // already there — no redirect loop
  }
  location.assign(
    `${base}/login?redirect=${encodeURIComponent(location.pathname + location.hash)}`
  );
});
