export interface User {
  id: string;
  email: string;
  display_name: string;
  nickname: string;
  avatar_url: string | null;
  avatar_revision: number | null;
}

export type FriendshipRelationship = "none" | "outgoing" | "incoming" | "friend";

export interface FriendSearchItem {
  id: string;
  nickname: string;
  display_name: string;
  avatar_url: string | null;
  avatar_revision: number | null;
  relationship: FriendshipRelationship;
  request_id?: string;
}

export interface FriendItem {
  request_id: string;
  user_id: string;
  nickname: string;
  display_name: string;
  avatar_url: string | null;
  avatar_revision: number | null;
  changed_at: string;
}

export interface FriendPage {
  items: FriendItem[];
  total: number;
  limit: number;
  offset: number;
}

export interface FriendsOverview {
  friends: FriendPage;
  incoming: FriendPage;
  outgoing: FriendPage;
}

export interface MeetingInviteCandidate {
  user_id: string;
  nickname: string;
  display_name: string;
  avatar_url: string | null;
  avatar_revision: number | null;
  invitation_id: string | null;
  invitation_status: "pending" | "accepted" | "declined" | null;
  is_participant: boolean;
}

export interface MeetingInviteCandidatePage {
  items: MeetingInviteCandidate[];
  total: number;
  limit: number;
  offset: number;
}

export interface IncomingMeetingInvite {
  id: string;
  meeting_id: string;
  meeting_title: string;
  owner_display_name: string;
  starts_at: string | null;
  ends_at: string | null;
  timezone: string;
  created_at: string;
}

export interface IncomingMeetingInvitePage {
  items: IncomingMeetingInvite[];
  total: number;
  limit: number;
  offset: number;
}

export interface MeetingInviteResponseMutation {
  meeting_id: string;
  changed: boolean;
  joined: boolean;
}

export interface FriendMutation {
  changed: boolean;
}

export interface AvatarMutation {
  avatar_revision: number | null;
  changed: boolean;
  updated_at: string;
}

export interface Session {
  user: User;
  access_token: string;
  access_token_expires_at: string;
}

export interface Meeting {
  id: string;
  owner_id: string;
  title: string;
  description: string;
  event_type: string;
  coordination_mode: "planning" | "fixed";
  cover_url: string | null;
  has_photo?: boolean;
  location_name: string | null;
  location_url: string | null;
  timezone: string;
  state: "draft" | "collecting" | "scheduled" | "cancelled" | "completed";
  selected_plan_option_id?: string;
  selected_time_option_id?: string;
  version: number;
  participant_role: "owner" | "participant";
  participant_joined_at?: string;
  selected_starts_at?: string;
  selected_ends_at?: string;
  my_attendance_status?: AttendanceStatus;
  created_at: string;
  updated_at: string;
}

export type AttendanceStatus = "going" | "maybe" | "not_going" | "unanswered";

export interface AttendanceParticipant {
  user_id: string;
  display_name: string;
  role: "owner" | "participant";
  status: AttendanceStatus;
  updated_at?: string;
}

export interface AttendanceView {
  participant_count: number;
  going_count: number;
  maybe_count: number;
  not_going_count: number;
  unanswered_count: number;
  my_status: AttendanceStatus;
  participants: AttendanceParticipant[];
  limit: number;
  offset: number;
}

export interface MeetingNote {
  user_id: string;
  display_name: string;
  text: string;
  created_at: string;
  updated_at: string;
}

export interface MeetingNotePage {
  items: MeetingNote[];
  total: number;
  limit: number;
  offset: number;
}

export interface ChangedMutation {
  changed: boolean;
}

export interface PlanOption {
  id: string;
  title: string;
  description: string;
  has_photo?: boolean;
  position: number;
  created_at: string;
}

export interface TimeOption {
  id: string;
  plan_option_id?: string;
  starts_at: string;
  ends_at: string | null;
  position: number;
  created_at: string;
}

export interface UpdatePlanOptionInput {
  title: string;
  description: string;
  version: number;
}

export interface UpdateTimeOptionInput {
  plan_option_id: string | null;
  starts_at: string;
  ends_at: string | null;
  version: number;
}

export interface Participant {
  user_id: string;
  display_name: string;
  role: "owner" | "participant";
  joined_at: string;
}

export interface MeetingDetail extends Meeting {
  plan_options: PlanOption[];
  time_options: TimeOption[];
  participants: Participant[];
  active_invitation_expires_at?: string;
}

export interface UpdateMeetingInput {
  title: string;
  description: string;
  event_type: string;
  cover_url: string | null;
  location_name: string | null;
  location_url: string | null;
  starts_at?: string | null;
  ends_at?: string | null;
  version: number;
}

export interface Invitation {
  expires_at: string;
}

export interface PollOption {
  id: string;
  label: string;
  position: number;
  vote_count: number;
  selected_by_user: boolean;
  voters: PollVoter[];
}

export interface PollVoter {
  user_id: string;
  display_name: string;
  updated_at: string;
}

export interface Poll {
  id: string;
  created_by_user_id: string;
  question: string;
  response_mode: "single" | "multiple";
  is_anonymous: boolean;
  allow_revote: boolean;
  can_manage: boolean;
  deadline?: string;
  state: "open" | "closed";
  selected_option_id: string | null;
  accepting_answers: boolean;
  participant_count: number;
  respondent_count: number;
  total_selections: number;
  options: PollOption[];
  created_at: string;
  closed_at?: string;
}

export interface PollPage {
  items: Poll[];
}

export interface PollHistoryEntry {
  id: string;
  user_id: string;
  display_name: string;
  action: "cast" | "change" | "retract";
  previous_option_ids: string[];
  previous_option_labels: string[];
  new_option_ids: string[];
  new_option_labels: string[];
  created_at: string;
}

export interface PollHistoryPage {
  items: PollHistoryEntry[];
  total: number;
  limit: number;
  offset: number;
}

export interface CreatePollInput {
  question: string;
  response_mode: "single" | "multiple";
  is_anonymous: boolean;
  allow_revote: boolean;
  deadline: string | null;
  options: string[];
}

export type AvailabilityStatus =
  | "preferred"
  | "available"
  | "if_needed"
  | "unavailable"
  | "unanswered";

export interface AvailabilityCounts {
  preferred: number;
  available: number;
  if_needed: number;
  unavailable: number;
  unanswered: number;
}

export interface AvailabilityResponse {
  user_id: string;
  status: Exclude<AvailabilityStatus, "unanswered">;
}

export interface AvailabilityTime {
  id: string;
  plan_option_id: string | null;
  starts_at: string;
  ends_at: string | null;
  position: number;
  my_status: AvailabilityStatus;
  counts: AvailabilityCounts;
  responses: AvailabilityResponse[];
  score: number;
}

export interface AvailabilityRecommendation {
  plan_option_id: string;
  plan_title: string;
  time_option_id: string;
  score: number;
  provisional: boolean;
  explanation: string;
}

export interface AvailabilityView {
  weights: Record<AvailabilityStatus, number>;
  participants: Participant[];
  items: AvailabilityTime[];
  recommendations: AvailabilityRecommendation[];
}

export interface PlanVoteOption {
  id: string;
  title: string;
  description: string;
  position: number;
  vote_count: number;
  selected_by_user: boolean;
}

export interface PlanVoteResponse {
  user_id: string;
  display_name: string;
  plan_option_id: string;
  updated_at: string;
}

export interface PlanVoteHistoryEntry {
  id: string;
  user_id: string;
  display_name: string;
  action: "cast" | "change" | "retract";
  previous_plan_option_id: string | null;
  previous_plan_title: string | null;
  new_plan_option_id: string | null;
  new_plan_title: string | null;
  created_at: string;
}

export interface PlanVotePage {
  options: PlanVoteOption[];
  responses: PlanVoteResponse[];
  history: PlanVoteHistoryEntry[];
  participant_count: number;
  answered_count: number;
  history_total: number;
  limit: number;
  offset: number;
}

export interface FinalDecision {
  plan_option_id: string;
  time_option_id: string;
  state: "scheduled";
}

export interface MeetingCompletion {
  meeting_id: string;
  state: "completed";
  version: number;
  updated_at: string;
}

export interface MeetingCancellation {
  meeting_id: string;
  state: "cancelled";
  version: number;
  updated_at: string;
}

export interface RequirementAssignee {
  user_id: string;
  display_name: string;
  quantity: number;
  updated_at: string;
}

export interface Requirement {
  id: string;
  name: string;
  required_quantity: number;
  claimed_quantity: number;
  remaining_quantity: number;
  status: "open" | "completed";
  my_quantity: number;
  assignees: RequirementAssignee[];
  created_at: string;
  updated_at: string;
}

export interface RequirementPage {
  items: Requirement[];
  total: number;
  open_count: number;
  completed_count: number;
  limit: number;
  offset: number;
}

export interface RequirementInput {
  name: string;
  required_quantity: number;
}

export interface MeetingPage {
  items: Meeting[];
  limit: number;
  offset: number;
}

export interface MeetingLiveEvent {
  type: "ready" | "meeting.updated";
  version: number;
}

export interface PhotoMutation {
  version: number;
  changed: boolean;
  updated_at: string;
}

interface ProblemResponse {
  error?: {
    code?: string;
    message?: string;
  };
}

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

let accessToken = "";
let refreshPromise: Promise<Session | null> | null = null;

export function setAccessToken(token: string): void {
  accessToken = token;
}

async function parseResponse<T>(response: Response): Promise<T> {
  if (response.ok) {
    if (response.status === 204) {
      return undefined as T;
    }
    return (await response.json()) as T;
  }
  let problem: ProblemResponse = {};
  try {
    problem = (await response.json()) as ProblemResponse;
  } catch {
    // The status and safe fallback below still describe the failure.
  }
  throw new ApiError(
    response.status,
    problem.error?.code ?? "request_failed",
    problem.error?.message ?? "Не удалось выполнить запрос.",
  );
}

async function refreshSession(): Promise<Session | null> {
  if (!refreshPromise) {
    refreshPromise = fetch("/api/v1/auth/refresh", {
      method: "POST",
      credentials: "include",
    })
      .then((response) => parseResponse<Session>(response))
      .then((session) => {
        setAccessToken(session.access_token);
        return session;
      })
      .catch(() => {
        setAccessToken("");
        return null;
      })
      .finally(() => {
        refreshPromise = null;
      });
  }
  return refreshPromise;
}

async function request<T>(path: string, init: RequestInit = {}, retry = true): Promise<T> {
  const headers = new Headers(init.headers);
  if (init.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }
  const response = await fetch(path, {
    ...init,
    headers,
    credentials: "include",
  });
  if (response.status === 401 && retry && (await refreshSession())) {
    return request<T>(path, init, false);
  }
  return parseResponse<T>(response);
}

async function requestBlob(path: string, retry = true): Promise<Blob> {
  const headers = new Headers();
  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }
  const response = await fetch(path, {
    method: "GET",
    headers,
    credentials: "include",
  });
  if (response.status === 401 && retry && (await refreshSession())) {
    return requestBlob(path, false);
  }
  if (!response.ok) {
    await parseResponse<never>(response);
  }
  return response.blob();
}

async function openAuthorizedStream(
  path: string,
  signal: AbortSignal,
  retry = true,
): Promise<Response> {
  const headers = new Headers();
  if (accessToken) {
    headers.set("Authorization", `Bearer ${accessToken}`);
  }
  const response = await fetch(path, {
    method: "GET",
    headers,
    credentials: "include",
    signal,
  });
  if (response.status === 401 && retry && (await refreshSession())) {
    return openAuthorizedStream(path, signal, false);
  }
  if (!response.ok) {
    await parseResponse<never>(response);
  }
  return response;
}

function parseMeetingEvent(block: string): MeetingLiveEvent | null {
  let eventName = "";
  const dataLines: string[] = [];
  for (const line of block.split("\n")) {
    if (line.startsWith("event:")) {
      eventName = line.slice("event:".length).trim();
    } else if (line.startsWith("data:")) {
      dataLines.push(line.slice("data:".length).trimStart());
    }
  }
  if (eventName !== "ready" && eventName !== "meeting.updated") {
    return null;
  }
  const payload = JSON.parse(dataLines.join("\n")) as { version?: unknown };
  if (!Number.isSafeInteger(payload.version) || Number(payload.version) < 1) {
    throw new ApiError(502, "invalid_live_event", "Сервер прислал некорректное живое обновление.");
  }
  return { type: eventName, version: Number(payload.version) };
}

export const api = {
  async bootstrap(): Promise<Session | null> {
    return refreshSession();
  },

  async register(input: {
    email: string;
    password: string;
    display_name: string;
    nickname: string;
  }): Promise<Session> {
    const response = await fetch("/api/v1/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(input),
    });
    const session = await parseResponse<Session>(response);
    setAccessToken(session.access_token);
    return session;
  },

  async login(input: { email: string; password: string }): Promise<Session> {
    const response = await fetch("/api/v1/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      credentials: "include",
      body: JSON.stringify(input),
    });
    const session = await parseResponse<Session>(response);
    setAccessToken(session.access_token);
    return session;
  },

  async logout(): Promise<void> {
    try {
      await request<void>("/api/v1/auth/logout", { method: "POST" }, false);
    } finally {
      setAccessToken("");
    }
  },

  updateProfile(input: { display_name: string; nickname: string; avatar_url: string | null }): Promise<User> {
    return request<User>("/api/v1/me", {
      method: "PUT",
      body: JSON.stringify(input),
    });
  },

  getUserAvatar(userID: string, revision: number): Promise<Blob> {
    return requestBlob(`/api/v1/users/${encodeURIComponent(userID)}/avatar?revision=${encodeURIComponent(revision)}`);
  },

  putUserAvatar(file: File): Promise<AvatarMutation> {
    return request<AvatarMutation>("/api/v1/me/avatar", {
      method: "PUT",
      headers: { "Content-Type": file.type },
      body: file,
    });
  },

  deleteUserAvatar(): Promise<AvatarMutation> {
    return request<AvatarMutation>("/api/v1/me/avatar", { method: "DELETE" });
  },

  searchUsers(query: string): Promise<{ items: FriendSearchItem[] }> {
    const params = new URLSearchParams({ q: query, limit: "20" });
    return request<{ items: FriendSearchItem[] }>(`/api/v1/users/search?${params}`);
  },

  getFriends(): Promise<FriendsOverview> {
    return request<FriendsOverview>("/api/v1/friends?limit=50&offset=0");
  },

  sendFriendRequest(userID: string): Promise<FriendMutation> {
    return request<FriendMutation>("/api/v1/friend-requests", {
      method: "POST",
      body: JSON.stringify({ user_id: userID }),
    });
  },

  acceptFriendRequest(requestID: string): Promise<FriendMutation> {
    return request<FriendMutation>(`/api/v1/friend-requests/${encodeURIComponent(requestID)}`, {
      method: "PUT",
    });
  },

  deleteFriendRequest(requestID: string): Promise<void> {
    return request<void>(`/api/v1/friend-requests/${encodeURIComponent(requestID)}`, {
      method: "DELETE",
    });
  },

  removeFriend(userID: string): Promise<void> {
    return request<void>(`/api/v1/friends/${encodeURIComponent(userID)}`, {
      method: "DELETE",
    });
  },

  getMeetingInviteCandidates(meetingID: string): Promise<MeetingInviteCandidatePage> {
    return request<MeetingInviteCandidatePage>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/friend-invitations?limit=50&offset=0`,
    );
  },

  sendMeetingInvites(meetingID: string, userIDs: string[]): Promise<{ changed_count: number }> {
    return request<{ changed_count: number }>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/friend-invitations`,
      { method: "POST", body: JSON.stringify({ user_ids: userIDs }) },
    );
  },

  getIncomingMeetingInvites(): Promise<IncomingMeetingInvitePage> {
    return request<IncomingMeetingInvitePage>("/api/v1/me/meeting-invitations?limit=50&offset=0");
  },

  acceptMeetingInvite(invitationID: string): Promise<MeetingInviteResponseMutation> {
    return request<MeetingInviteResponseMutation>(
      `/api/v1/me/meeting-invitations/${encodeURIComponent(invitationID)}`,
      { method: "PUT" },
    );
  },

  declineMeetingInvite(invitationID: string): Promise<MeetingInviteResponseMutation> {
    return request<MeetingInviteResponseMutation>(
      `/api/v1/me/meeting-invitations/${encodeURIComponent(invitationID)}`,
      { method: "DELETE" },
    );
  },

  listMeetings(): Promise<MeetingPage> {
    return request<MeetingPage>("/api/v1/meetings?limit=50&offset=0");
  },

  getMeeting(id: string): Promise<MeetingDetail> {
    return request<MeetingDetail>(`/api/v1/meetings/${encodeURIComponent(id)}`);
  },

  exportMeetingCalendar(id: string): Promise<Blob> {
    return requestBlob(`/api/v1/meetings/${encodeURIComponent(id)}/calendar.ics`);
  },

  getMeetingPhoto(meetingID: string, revision: number): Promise<Blob> {
    return requestBlob(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/photo?revision=${encodeURIComponent(revision)}`,
    );
  },

  putMeetingPhoto(meetingID: string, version: number, file: File): Promise<PhotoMutation> {
    return request<PhotoMutation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/photo?version=${encodeURIComponent(version)}`,
      {
        method: "PUT",
        headers: { "Content-Type": file.type },
        body: file,
      },
    );
  },

  deleteMeetingPhoto(meetingID: string, version: number): Promise<PhotoMutation> {
    return request<PhotoMutation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/photo?version=${encodeURIComponent(version)}`,
      { method: "DELETE" },
    );
  },

  getPlanOptionPhoto(meetingID: string, optionID: string, revision: number): Promise<Blob> {
    return requestBlob(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-options/${encodeURIComponent(optionID)}/photo?revision=${encodeURIComponent(revision)}`,
    );
  },

  putPlanOptionPhoto(
    meetingID: string,
    optionID: string,
    version: number,
    file: File,
  ): Promise<PhotoMutation> {
    return request<PhotoMutation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-options/${encodeURIComponent(optionID)}/photo?version=${encodeURIComponent(version)}`,
      {
        method: "PUT",
        headers: { "Content-Type": file.type },
        body: file,
      },
    );
  },

  deletePlanOptionPhoto(
    meetingID: string,
    optionID: string,
    version: number,
  ): Promise<PhotoMutation> {
    return request<PhotoMutation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-options/${encodeURIComponent(optionID)}/photo?version=${encodeURIComponent(version)}`,
      { method: "DELETE" },
    );
  },

  async watchMeeting(
    meetingID: string,
    signal: AbortSignal,
    onEvent: (event: MeetingLiveEvent) => void,
  ): Promise<void> {
    const response = await openAuthorizedStream(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/events`,
      signal,
    );
    if (!response.body) {
      throw new ApiError(502, "live_stream_unavailable", "Сервер не открыл поток живых обновлений.");
    }

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";
    try {
      while (true) {
        const { done, value } = await reader.read();
        buffer += decoder.decode(value, { stream: !done }).replace(/\r\n/g, "\n");
        if (buffer.length > 64 * 1024) {
          throw new ApiError(502, "live_event_too_large", "Живое обновление превысило допустимый размер.");
        }
        let separator = buffer.indexOf("\n\n");
        while (separator >= 0) {
          const block = buffer.slice(0, separator);
          buffer = buffer.slice(separator + 2);
          const event = parseMeetingEvent(block);
          if (event) {
            onEvent(event);
          }
          separator = buffer.indexOf("\n\n");
        }
        if (done) {
          return;
        }
      }
    } finally {
      reader.releaseLock();
    }
  },

  createMeeting(input: {
    title: string;
    description: string;
    event_type: string;
    coordination_mode: "planning" | "fixed";
    cover_url: string | null;
    location_name: string | null;
    location_url: string | null;
    timezone: string;
    starts_at: string;
    ends_at: string | null;
  }): Promise<Meeting> {
    return request<Meeting>("/api/v1/meetings", {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: JSON.stringify(input),
    });
  },

  updateMeeting(meetingID: string, input: UpdateMeetingInput): Promise<Meeting> {
    return request<Meeting>(`/api/v1/meetings/${encodeURIComponent(meetingID)}`, {
      method: "PUT",
      body: JSON.stringify(input),
    });
  },

  addPlanOption(meetingID: string, input: { title: string; description: string }): Promise<PlanOption> {
    return request<PlanOption>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-options`, {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: JSON.stringify(input),
    });
  },

  updatePlanOption(
    meetingID: string,
    optionID: string,
    input: UpdatePlanOptionInput,
  ): Promise<PlanOption> {
    return request<PlanOption>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-options/${encodeURIComponent(optionID)}`,
      { method: "PUT", body: JSON.stringify(input) },
    );
  },

  deletePlanOption(meetingID: string, optionID: string): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-options/${encodeURIComponent(optionID)}`,
      { method: "DELETE" },
    );
  },

  addTimeOption(
    meetingID: string,
    input: { plan_option_id: string | null; starts_at: string; ends_at: string | null },
  ): Promise<TimeOption> {
    return request<TimeOption>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/time-options`, {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: JSON.stringify(input),
    });
  },

  updateTimeOption(
    meetingID: string,
    optionID: string,
    input: UpdateTimeOptionInput,
  ): Promise<TimeOption> {
    return request<TimeOption>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/time-options/${encodeURIComponent(optionID)}`,
      { method: "PUT", body: JSON.stringify(input) },
    );
  },

  deleteTimeOption(meetingID: string, optionID: string): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/time-options/${encodeURIComponent(optionID)}`,
      { method: "DELETE" },
    );
  },

  createInvitation(meetingID: string, secret: string): Promise<Invitation> {
    return request<Invitation>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/invitations`, {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: JSON.stringify({ secret }),
    });
  },

  revokeInvitation(meetingID: string): Promise<void> {
    return request<void>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/invitation`, {
      method: "DELETE",
    });
  },

  joinInvitation(token: string): Promise<MeetingDetail> {
    return request<MeetingDetail>("/api/v1/invitations/join", {
      method: "POST",
      body: JSON.stringify({ token }),
    });
  },

  listPolls(meetingID: string): Promise<PollPage> {
    return request<PollPage>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/polls`);
  },

  createPoll(
    meetingID: string,
    input: CreatePollInput,
  ): Promise<Poll> {
    return request<Poll>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/polls`, {
      method: "POST",
      headers: { "Idempotency-Key": crypto.randomUUID() },
      body: JSON.stringify(input),
    });
  },

  deletePoll(meetingID: string, pollID: string): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/polls/${encodeURIComponent(pollID)}`,
      { method: "DELETE" },
    );
  },

  votePoll(pollID: string, optionIDs: string[]): Promise<void> {
    return request<void>(`/api/v1/polls/${encodeURIComponent(pollID)}/vote`, {
      method: "PUT",
      body: JSON.stringify({ option_ids: optionIDs }),
    });
  },

  getPollHistory(pollID: string, limit = 50, offset = 0): Promise<PollHistoryPage> {
    const params = new URLSearchParams({
      limit: String(limit),
      offset: String(offset),
    });
    return request<PollHistoryPage>(
      `/api/v1/polls/${encodeURIComponent(pollID)}/history?${params}`,
    );
  },

  closePoll(
    meetingID: string,
    pollID: string,
    selectedOptionID: string | null,
  ): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/polls/${encodeURIComponent(pollID)}/close`,
      {
        method: "POST",
        body: JSON.stringify({ selected_option_id: selectedOptionID }),
      },
    );
  },

  getAvailability(meetingID: string): Promise<AvailabilityView> {
    return request<AvailabilityView>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/availability`,
    );
  },

  setAvailability(timeOptionID: string, status: AvailabilityStatus): Promise<void> {
    return request<void>(
      `/api/v1/time-options/${encodeURIComponent(timeOptionID)}/availability`,
      {
        method: "PUT",
        body: JSON.stringify({ status }),
      },
    );
  },

  getAttendance(meetingID: string, limit = 100, offset = 0): Promise<AttendanceView> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    return request<AttendanceView>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/attendance?${params}`,
    );
  },

  setAttendance(meetingID: string, status: AttendanceStatus): Promise<void> {
    return request<void>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/attendance`, {
      method: "PUT",
      body: JSON.stringify({ status }),
    });
  },

  listMeetingNotes(meetingID: string, limit = 100, offset = 0): Promise<MeetingNotePage> {
    const params = new URLSearchParams({ limit: String(limit), offset: String(offset) });
    return request<MeetingNotePage>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/notes?${params}`,
    );
  },

  upsertMeetingNote(meetingID: string, text: string): Promise<ChangedMutation> {
    return request<ChangedMutation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/notes/mine`,
      { method: "PUT", body: JSON.stringify({ text }) },
    );
  },

  deleteMeetingNote(meetingID: string): Promise<ChangedMutation> {
    return request<ChangedMutation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/notes/mine`,
      { method: "DELETE" },
    );
  },

  getPlanVotes(meetingID: string): Promise<PlanVotePage> {
    return request<PlanVotePage>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-votes?limit=50&offset=0`,
    );
  },

  setPlanVote(meetingID: string, planOptionID: string | null): Promise<void> {
    return request<void>(`/api/v1/meetings/${encodeURIComponent(meetingID)}/plan-vote`, {
      method: "PUT",
      body: JSON.stringify({ plan_option_id: planOptionID }),
    });
  },

  finalizeDecision(
    meetingID: string,
    planOptionID: string,
    timeOptionID: string,
  ): Promise<FinalDecision> {
    return request<FinalDecision>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/decision`,
      {
        method: "POST",
        body: JSON.stringify({
          plan_option_id: planOptionID,
          time_option_id: timeOptionID,
        }),
      },
    );
  },

  completeMeeting(meetingID: string): Promise<MeetingCompletion> {
    return request<MeetingCompletion>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/complete`,
      { method: "POST" },
    );
  },

  cancelMeeting(meetingID: string): Promise<MeetingCancellation> {
    return request<MeetingCancellation>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/cancel`,
      { method: "POST" },
    );
  },

  listRequirements(meetingID: string): Promise<RequirementPage> {
    return request<RequirementPage>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/requirements?limit=50&offset=0`,
    );
  },

  createRequirement(
    meetingID: string,
    input: RequirementInput,
  ): Promise<Requirement> {
    return request<Requirement>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/requirements`,
      {
        method: "POST",
        headers: { "Idempotency-Key": crypto.randomUUID() },
        body: JSON.stringify(input),
      },
    );
  },

  updateRequirement(
    meetingID: string,
    requirementID: string,
    input: RequirementInput,
  ): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/requirements/${encodeURIComponent(requirementID)}`,
      {
        method: "PUT",
        body: JSON.stringify(input),
      },
    );
  },

  deleteRequirement(meetingID: string, requirementID: string): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/requirements/${encodeURIComponent(requirementID)}`,
      { method: "DELETE" },
    );
  },

  setRequirementClaim(
    meetingID: string,
    requirementID: string,
    quantity: number,
  ): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/requirements/${encodeURIComponent(requirementID)}/claim`,
      {
        method: "PUT",
        body: JSON.stringify({ quantity }),
      },
    );
  },

  setRequirementStatus(
    meetingID: string,
    requirementID: string,
    status: "open" | "completed",
  ): Promise<void> {
    return request<void>(
      `/api/v1/meetings/${encodeURIComponent(meetingID)}/requirements/${encodeURIComponent(requirementID)}/status`,
      {
        method: "PUT",
        body: JSON.stringify({ status }),
      },
    );
  },
};
