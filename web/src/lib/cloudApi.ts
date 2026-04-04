type Envelope<T> = { code: number; err?: string; data?: T };

async function fetchEnv<T>(url: string, token: string, method: string = 'GET', body?: any): Promise<T> {
  const res = await fetch(url, {
    method,
    headers: {
      Accept: "application/json",
      "Content-Type": "application/json",
      Authorization: token ? `Bearer ${token}` : "",
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await res.text().catch(() => "");
  let payload: any;
  try {
    payload = JSON.parse(text);
  } catch {
    payload = undefined;
  }
  const env: Envelope<T> | undefined =
    payload && typeof payload === "object" && "code" in payload ? (payload as Envelope<T>) : undefined;
  if (!res.ok) {
    const msg = env?.err || text || `${res.status} ${res.statusText}`;
    throw new Error(msg);
  }
  if (!env || env.code !== 0) throw new Error(env?.err || "request failed");
  return env.data as T;
}

export type PlanInfo = {
  id: string;
  name: string;
  tier: string;
  ingest_rate: number;
  ingest_burst: number;
  query_rate: number;
  query_burst: number;
  quota_bytes?: number;
};

export async function getPlans(apiBase: string, token: string): Promise<{ items: PlanInfo[] }> {
  return fetchEnv(`${apiBase}/api/cloud/plans`, token);
}

export type AccountInfo = {
  account: { id: number; email: string };
  subscription: { tier: string };
  period: { start: string; end: string };
  usage: { ingest_bytes: number; by_project: { project_id: string; ingest_bytes: number; tier: string }[] };
  cost_preview: { currency: string; total: number; line_items: { name: string; amount: number }[] };
};

export async function getAccount(apiBase: string, token: string): Promise<AccountInfo> {
  return fetchEnv(`${apiBase}/api/cloud/account`, token);
}

// ============ Project Types ============

export type Project = {
  id: string;
  owner_user_id: number;
  owner_org_id?: number;
  name: string;
  tier?: string;
};

// ============ Project APIs ============

export async function listProjects(apiBase: string, token: string, orgId?: number): Promise<Project[]> {
  const url = orgId 
    ? `${apiBase}/api/projects?org_id=${orgId}`
    : `${apiBase}/api/projects`;
  const data = await fetchEnv<{ items: Project[] }>(url, token);
  return data.items || [];
}

export async function createProject(apiBase: string, token: string, name: string, orgId?: number): Promise<Project> {
  return fetchEnv(`${apiBase}/api/projects`, token, 'POST', { name, org_id: orgId });
}

export async function getProject(apiBase: string, token: string, projectId: string): Promise<Project> {
  return fetchEnv(`${apiBase}/api/projects/${projectId}`, token);
}

export async function deleteProject(apiBase: string, token: string, projectId: string): Promise<void> {
  return fetchEnv(`${apiBase}/api/projects/${projectId}`, token, 'DELETE');
}

// ============ Organization Types ============

export type Organization = {
  id: number;
  name: string;
  display_name: string;
  owner_id: number;
  tier: string;
  member_limit: number;
  project_limit: number;
  member_count: number;
  project_count: number;
  current_user_role: string;
  created_at: string;
  updated_at: string;
};

export type OrganizationMember = {
  id: number;
  org_id: number;
  user_id: number;
  username: string;
  email: string;
  avatar_url: string;
  role: string;
  status: string;
  invited_by: number;
  invited_at: string;
  accepted_at?: string;
  created_at: string;
};

export type CreateOrgRequest = {
  name: string;
  display_name?: string;
};

export type UpdateOrgRequest = {
  name?: string;
  display_name?: string;
};

export type InviteMemberRequest = {
  email: string;
  role: string;
};

export type UpdateMemberRoleRequest = {
  role: string;
};

// ============ Organization APIs ============

export async function listOrganizations(apiBase: string, token: string): Promise<Organization[]> {
  return fetchEnv(`${apiBase}/api/orgs`, token);
}

export async function getOrganization(apiBase: string, token: string, orgId: number): Promise<Organization> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}`, token);
}

export async function createOrganization(apiBase: string, token: string, req: CreateOrgRequest): Promise<Organization> {
  return fetchEnv(`${apiBase}/api/orgs`, token, 'POST', req);
}

export async function updateOrganization(apiBase: string, token: string, orgId: number, req: UpdateOrgRequest): Promise<Organization> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}`, token, 'PUT', req);
}

export async function deleteOrganization(apiBase: string, token: string, orgId: number): Promise<void> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}`, token, 'DELETE');
}

// ============ Organization Member APIs ============

export async function listOrgMembers(apiBase: string, token: string, orgId: number, page: number = 1, perPage: number = 20): Promise<{
  items: OrganizationMember[];
  total: number;
  page: number;
  per_page: number;
}> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/members?page=${page}&per_page=${perPage}`, token);
}

export async function inviteMember(apiBase: string, token: string, orgId: number, req: InviteMemberRequest): Promise<OrganizationMember> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/members`, token, 'POST', req);
}

export async function updateMemberRole(apiBase: string, token: string, orgId: number, memberId: number, req: UpdateMemberRoleRequest): Promise<OrganizationMember> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/members/${memberId}`, token, 'PUT', req);
}

export async function removeMember(apiBase: string, token: string, orgId: number, memberId: number): Promise<void> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/members/${memberId}`, token, 'DELETE');
}

export async function leaveOrganization(apiBase: string, token: string, orgId: number): Promise<void> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/members/leave`, token, 'POST');
}

// ============ Subscription Types ============

export type Subscription = {
  org_id: number;
  plan_id: string;
  plan_name: string;
  status: string;
  billing_cycle: string;
  current_period_start: string;
  current_period_end: string;
  member_limit: number;
  project_limit: number;
  ingest_rate: number;
  query_rate: number;
  quota_bytes: number;
};

export type Plan = {
  id: string;
  name: string;
  tier: string;
  ingest_rate: number;
  ingest_burst: number;
  query_rate: number;
  query_burst: number;
  quota_bytes: number;
  member_limit: number;
  project_limit: number;
  monthly_price: number;
  yearly_price: number;
};

export type SubscriptionChange = {
  id: number;
  from_plan_id: string;
  to_plan_id: string;
  reason: string;
  changed_by: number;
  created_at: string;
};

// ============ Subscription APIs ============

export async function getSubscription(apiBase: string, token: string, orgId: number): Promise<Subscription> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/subscription`, token);
}

export async function changeSubscription(apiBase: string, token: string, orgId: number, planId: string, billingCycle?: string): Promise<Subscription> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/subscription`, token, 'PUT', { plan_id: planId, billing_cycle: billingCycle });
}

export async function cancelSubscription(apiBase: string, token: string, orgId: number): Promise<void> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/subscription`, token, 'DELETE');
}

export async function getSubscriptionHistory(apiBase: string, token: string, orgId: number): Promise<SubscriptionChange[]> {
  const data = await fetchEnv<{ items: SubscriptionChange[] }>(`${apiBase}/api/orgs/${orgId}/subscription/history`, token);
  return data.items || [];
}

export async function listPlans(apiBase: string, token: string): Promise<Plan[]> {
  const data = await fetchEnv<{ items: Plan[] }>(`${apiBase}/api/plans`, token);
  return data.items || [];
}

// ============ Usage Types ============

export type OrgUsageStats = {
  org_id: number;
  org_name: string;
  month: string;
  ingest_bytes: number;
  ingest_count: number;
  query_count: number;
  query_bytes: number;
  project_count: number;
  by_project: ProjectUsageStats[];
};

export type ProjectUsageStats = {
  project_id: string;
  project_name: string;
  ingest_bytes: number;
  ingest_count: number;
  query_count: number;
  query_bytes: number;
};

export type UsageTrendPoint = {
  date: string;
  value: number;
};

export type ProjectUsage = {
  project_id: string;
  project_name: string;
  start_date: string;
  end_date: string;
  ingest_bytes: number;
  ingest_count: number;
  query_count: number;
  query_bytes: number;
  daily_records: UsageRecord[];
};

export type UsageRecord = {
  id: number;
  project_id: number;
  day: string;
  ingest_bytes: number;
  ingest_count: number;
  query_count: number;
  query_bytes: number;
};

// ============ Usage APIs ============

export async function getOrgUsage(apiBase: string, token: string, orgId: number, month?: string): Promise<OrgUsageStats> {
  const url = month 
    ? `${apiBase}/api/orgs/${orgId}/usage?month=${month}`
    : `${apiBase}/api/orgs/${orgId}/usage`;
  return fetchEnv(url, token);
}

export async function getOrgUsageTrend(apiBase: string, token: string, orgId: number, metric?: string, startDate?: string, endDate?: string): Promise<UsageTrendPoint[]> {
  const params = new URLSearchParams();
  if (metric) params.set('metric', metric);
  if (startDate) params.set('start_date', startDate);
  if (endDate) params.set('end_date', endDate);
  const data = await fetchEnv<{ items: UsageTrendPoint[] }>(`${apiBase}/api/orgs/${orgId}/usage/trend?${params}`, token);
  return data.items || [];
}

export async function getProjectUsage(apiBase: string, token: string, projectId: string, startDate?: string, endDate?: string): Promise<ProjectUsage> {
  const params = new URLSearchParams();
  if (startDate) params.set('start_date', startDate);
  if (endDate) params.set('end_date', endDate);
  const url = params.toString() 
    ? `${apiBase}/api/projects/${projectId}/usage?${params}`
    : `${apiBase}/api/projects/${projectId}/usage`;
  return fetchEnv(url, token);
}

// ============ Billing Types ============

export type BillingAccount = {
  id: number;
  org_id: number;
  customer_name: string;
  email: string;
  phone: string;
  address: string;
  payment_method: string;
  auto_pay: boolean;
  currency: string;
  balance: number;
};

export type Invoice = {
  id: number;
  invoice_number: string;
  period_start: string;
  period_end: string;
  due_date: string;
  status: string;
  subtotal: number;
  tax: number;
  discount: number;
  total: number;
  currency: string;
  items?: InvoiceLineItem[];
  paid_at?: string;
  created_at: string;
};

export type InvoiceLineItem = {
  id: number;
  description: string;
  quantity: number;
  unit: string;
  unit_price: number;
  amount: number;
};

export type Payment = {
  id: number;
  invoice_id?: number;
  amount: number;
  currency: string;
  method: string;
  transaction_id: string;
  status: string;
  created_at: string;
};

// ============ Billing APIs ============

export async function getBillingAccount(apiBase: string, token: string, orgId: number): Promise<BillingAccount> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/billing/account`, token);
}

export async function updateBillingAccount(apiBase: string, token: string, orgId: number, data: Partial<BillingAccount>): Promise<BillingAccount> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/billing/account`, token, 'PUT', data);
}

export async function listInvoices(apiBase: string, token: string, orgId: number): Promise<Invoice[]> {
  const data = await fetchEnv<{ items: Invoice[] }>(`${apiBase}/api/orgs/${orgId}/billing/invoices`, token);
  return data.items || [];
}

export async function getInvoice(apiBase: string, token: string, orgId: number, invoiceId: number): Promise<Invoice> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/billing/invoices/${invoiceId}`, token);
}

export async function generateInvoice(apiBase: string, token: string, orgId: number, periodStart: string, periodEnd: string): Promise<Invoice> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/billing/invoices`, token, 'POST', { period_start: periodStart, period_end: periodEnd });
}

export async function payInvoice(apiBase: string, token: string, orgId: number, invoiceId: number, method: string): Promise<{ payment_id: number; status: string; paid_at: string }> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/billing/invoices/${invoiceId}/pay`, token, 'POST', { method });
}

export async function listPayments(apiBase: string, token: string, orgId: number): Promise<Payment[]> {
  const data = await fetchEnv<{ items: Payment[] }>(`${apiBase}/api/orgs/${orgId}/billing/payments`, token);
  return data.items || [];
}

export async function topUpBalance(apiBase: string, token: string, orgId: number, amount: number, method: string, currency: string = 'CNY'): Promise<{ payment_id: number; amount: number; status: string }> {
  return fetchEnv(`${apiBase}/api/orgs/${orgId}/billing/topup`, token, 'POST', { amount, method, currency });
}
