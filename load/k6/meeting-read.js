import http from "k6/http";
import { check, sleep } from "k6";

const baseURL = (__ENV.BASE_URL || "http://host.docker.internal:8080").replace(/\/$/, "");
const virtualUsers = Number.parseInt(__ENV.VUS || "10", 10);
const duration = __ENV.DURATION || "20s";
const pauseSeconds = Number.parseFloat(__ENV.PAUSE_SECONDS || "0.2");

export const options = {
  summaryTrendStats: ["avg", "min", "med", "max", "p(90)", "p(95)", "p(99)"],
  scenarios: {
    authenticated_meeting_reads: {
      executor: "constant-vus",
      vus: virtualUsers,
      duration,
    },
  },
  thresholds: {
    checks: ["rate>0.99"],
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500", "p(99)<1000"],
  },
};

export function setup() {
  if (!__ENV.EMAIL || !__ENV.PASSWORD) {
    throw new Error("EMAIL and PASSWORD are required");
  }

  const login = http.post(
    `${baseURL}/api/v1/auth/login`,
    JSON.stringify({ email: __ENV.EMAIL, password: __ENV.PASSWORD }),
    {
      headers: { "Content-Type": "application/json" },
      responseType: "text",
      tags: { endpoint: "login" },
    },
  );
  if (!check(login, { "login succeeds": (response) => response.status === 200 })) {
    throw new Error(`login failed with status ${login.status}`);
  }

  const accessToken = login.json("access_token");
  const headers = { Authorization: `Bearer ${accessToken}` };
  let meetingID = __ENV.MEETING_ID;
  if (!meetingID) {
    const meetings = http.get(`${baseURL}/api/v1/meetings?limit=1&offset=0`, {
      headers,
      responseType: "text",
      tags: { endpoint: "meeting_list" },
    });
    if (!check(meetings, { "meeting list succeeds": (response) => response.status === 200 })) {
      throw new Error(`meeting list failed with status ${meetings.status}`);
    }
    meetingID = meetings.json("items.0.id");
  }
  if (!meetingID) {
    throw new Error("MEETING_ID is required when the account has no meetings");
  }
  return { accessToken, meetingID };
}

export default function ({ accessToken, meetingID }) {
  const response = http.get(`${baseURL}/api/v1/meetings/${meetingID}`, {
    headers: { Authorization: `Bearer ${accessToken}` },
    responseType: "none",
    tags: { endpoint: "meeting_detail" },
  });
  check(response, { "meeting detail succeeds": (result) => result.status === 200 });
  sleep(pauseSeconds);
}
