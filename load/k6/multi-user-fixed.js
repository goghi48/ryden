import http from "k6/http";
import { check, sleep } from "k6";
import exec from "k6/execution";

const baseURL = (__ENV.BASE_URL || "http://host.docker.internal:8080").replace(/\/$/, "");
const virtualUsers = Number.parseInt(__ENV.VUS || "20", 10);
const duration = __ENV.DURATION || "30s";
const pauseSeconds = Number.parseFloat(__ENV.PAUSE_SECONDS || "0.5");
const password = __ENV.PASSWORD || "Ryden-load-2026!";

export const options = {
  setupTimeout: "3m",
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    concurrent_fixed_meeting_activity: {
      executor: "constant-vus",
      vus: virtualUsers,
      duration,
    },
  },
  thresholds: {
    checks: ["rate>0.99"],
    http_req_failed: ["rate<0.01"],
    "http_req_duration{endpoint:join}": ["p(95)<1000", "p(99)<2000"],
    "http_req_duration{endpoint:attendance_write}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{endpoint:poll_vote}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{endpoint:requirement_claim}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{endpoint:meeting_detail}": ["p(95)<500", "p(99)<1000"],
    "http_req_duration{endpoint:poll_list}": ["p(95)<750", "p(99)<1500"],
    "http_req_duration{endpoint:attendance_read}": ["p(95)<500", "p(99)<1000"],
    "http_req_duration{endpoint:requirement_list}": ["p(95)<500", "p(99)<1000"],
    "http_req_duration{endpoint:note_list}": ["p(95)<500", "p(99)<1000"],
  },
};

function failUnless(response, expectedStatus, label) {
  if (!check(response, { [`${label} succeeds`]: (result) => result.status === expectedStatus })) {
    throw new Error(`${label} failed with status ${response.status}: ${response.body}`);
  }
  return response;
}

function jsonHeaders(token, endpoint, extra = {}) {
  return {
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...extra,
    },
    responseType: "text",
    tags: { endpoint },
  };
}

function register(email, displayName) {
  const response = http.post(
    `${baseURL}/api/v1/auth/register`,
    JSON.stringify({ email, password, display_name: displayName }),
    jsonHeaders("", "setup_register"),
  );
  failUnless(response, 201, `register ${displayName}`);
  return response.json("access_token");
}

function secretFor(runID) {
  const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_-";
  return `${runID}${alphabet.repeat(2)}`.replace(/[^A-Za-z0-9_-]/g, "_").slice(0, 43);
}

export function setup() {
  if (!Number.isInteger(virtualUsers) || virtualUsers < 1 || virtualUsers > 100) {
    throw new Error("VUS must be an integer between 1 and 100");
  }

  const runID = `${Date.now().toString(36)}-${Math.floor(Math.random() * 1e9).toString(36)}`;
  const ownerToken = register(`load-owner-${runID}@example.test`, "Load Owner");
  const participantTokens = [];
  for (let index = 0; index < virtualUsers; index += 1) {
    participantTokens.push(register(
      `load-participant-${runID}-${index}@example.test`,
      `Load Participant ${index + 1}`,
    ));
  }

  const start = new Date(Date.now() + 24 * 60 * 60 * 1000);
  start.setUTCSeconds(0, 0);
  start.setUTCMinutes(Math.ceil(start.getUTCMinutes() / 5) * 5);
  const createMeeting = http.post(
    `${baseURL}/api/v1/meetings`,
    JSON.stringify({
      title: `Load fixed ${runID}`,
      description: "Concurrent attendance, poll, preparation, and detail reads",
      event_type: "other",
      coordination_mode: "fixed",
      timezone: "UTC",
      starts_at: start.toISOString(),
      ends_at: null,
      cover_url: null,
      location_name: null,
      location_url: null,
    }),
    jsonHeaders(ownerToken, "setup_meeting", { "Idempotency-Key": `meeting-${runID}` }),
  );
  failUnless(createMeeting, 201, "create fixed meeting");
  const meetingID = createMeeting.json("id");

  const invitationSecret = secretFor(runID);
  const createInvitation = http.post(
    `${baseURL}/api/v1/meetings/${meetingID}/invitations`,
    JSON.stringify({ secret: invitationSecret }),
    jsonHeaders(ownerToken, "setup_invitation", { "Idempotency-Key": `invite-${runID}` }),
  );
  failUnless(createInvitation, 201, "create invitation");

  const createPoll = http.post(
    `${baseURL}/api/v1/meetings/${meetingID}/polls`,
    JSON.stringify({
      question: "Что взять дополнительно?",
      response_mode: "single",
      is_anonymous: false,
      allow_revote: true,
      deadline: null,
      options: ["Напитки", "Закуски"],
    }),
    jsonHeaders(ownerToken, "setup_poll", { "Idempotency-Key": `poll-${runID}` }),
  );
  failUnless(createPoll, 201, "create poll");
  const pollID = createPoll.json("id");
  const pollOptionIDs = createPoll.json("options").map((option) => option.id);

  const createRequirement = http.post(
    `${baseURL}/api/v1/meetings/${meetingID}/requirements`,
    JSON.stringify({ name: "Порции", required_quantity: virtualUsers }),
    jsonHeaders(ownerToken, "setup_requirement", { "Idempotency-Key": `requirement-${runID}` }),
  );
  failUnless(createRequirement, 201, "create requirement");

  return {
    invitationSecret,
    meetingID,
    ownerToken,
    participantTokens,
    pollID,
    pollOptionIDs,
    requirementID: createRequirement.json("id"),
  };
}

export default function (data) {
  const token = data.participantTokens[exec.vu.idInTest - 1];

  if (__ITER === 0) {
    const join = http.post(
      `${baseURL}/api/v1/invitations/join`,
      JSON.stringify({ token: data.invitationSecret }),
      jsonHeaders(token, "join"),
    );
    check(join, { "concurrent join succeeds": (response) => response.status === 200 });
  }

  const attendanceStatuses = ["going", "maybe", "not_going"];
  const attendance = http.put(
    `${baseURL}/api/v1/meetings/${data.meetingID}/attendance`,
    JSON.stringify({ status: attendanceStatuses[__ITER % attendanceStatuses.length] }),
    jsonHeaders(token, "attendance_write"),
  );
  check(attendance, { "attendance write succeeds": (response) => response.status === 204 });

  const pollVote = http.put(
    `${baseURL}/api/v1/polls/${data.pollID}/vote`,
    JSON.stringify({ option_ids: [data.pollOptionIDs[__ITER % data.pollOptionIDs.length]] }),
    jsonHeaders(token, "poll_vote"),
  );
  check(pollVote, { "poll vote succeeds": (response) => response.status === 204 });

  const claim = http.put(
    `${baseURL}/api/v1/meetings/${data.meetingID}/requirements/${data.requirementID}/claim`,
    JSON.stringify({ quantity: __ITER % 2 === 0 ? 1 : 0 }),
    jsonHeaders(token, "requirement_claim"),
  );
  check(claim, { "requirement claim succeeds": (response) => response.status === 204 });

  const [detail, polls, attendanceView, requirements, notes] = http.batch([
    ["GET", `${baseURL}/api/v1/meetings/${data.meetingID}`, null, jsonHeaders(token, "meeting_detail")],
    ["GET", `${baseURL}/api/v1/meetings/${data.meetingID}/polls`, null, jsonHeaders(token, "poll_list")],
    ["GET", `${baseURL}/api/v1/meetings/${data.meetingID}/attendance?limit=100&offset=0`, null, jsonHeaders(token, "attendance_read")],
    ["GET", `${baseURL}/api/v1/meetings/${data.meetingID}/requirements?limit=50&offset=0`, null, jsonHeaders(token, "requirement_list")],
    ["GET", `${baseURL}/api/v1/meetings/${data.meetingID}/notes?limit=50&offset=0`, null, jsonHeaders(token, "note_list")],
  ]);
  check(detail, { "meeting detail succeeds": (response) => response.status === 200 });
  check(polls, { "poll list succeeds": (response) => response.status === 200 });
  check(attendanceView, { "attendance read succeeds": (response) => response.status === 200 });
  check(requirements, { "requirement list succeeds": (response) => response.status === 200 });
  check(notes, { "note list succeeds": (response) => response.status === 200 });

  sleep(pauseSeconds);
}

export function teardown(data) {
  http.post(
    `${baseURL}/api/v1/meetings/${data.meetingID}/cancel`,
    null,
    jsonHeaders(data.ownerToken, "teardown_cancel"),
  );
}
