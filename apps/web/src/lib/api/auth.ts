export interface IdentityResponse {
  id: string;
  email?: string;
  role: string;
  created_at: string;
}

export interface ApiError {
  type: string;
  title: string;
  status: number;
  detail: string;
}

const API_BASE_URL = process.env.NEXT_PUBLIC_API_URL || 'http://localhost:8083/api/v1';

export async function requestRegistrationOTP(identifier: string): Promise<void> {
  const res = await fetch(`${API_BASE_URL}/auth/register/request-otp`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identifier }),
  });

  if (!res.ok) {
    const data = await res.json();
    throw data as ApiError;
  }
}

export async function verifyRegistrationOTP(identifier: string, password: string, otp: string): Promise<IdentityResponse> {
  const res = await fetch(`${API_BASE_URL}/auth/register/verify`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identifier, password, otp }),
  });

  const data = await res.json();
  if (!res.ok) {
    throw data as ApiError;
  }
  return data as IdentityResponse;
}
export async function loginUser(identifier: string, password: string): Promise<IdentityResponse> {
  const res = await fetch(`${API_BASE_URL}/auth/login`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ identifier, password }),
  });

  const data = await res.json();
  if (!res.ok) {
    throw data as ApiError;
  }
  return data as IdentityResponse;
}
