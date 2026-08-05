import { afterEach, describe, expect, it, vi } from "vitest";
import { api, setAccessToken, type MeetingLiveEvent } from "./api";

afterEach(() => {
  setAccessToken("");
  vi.restoreAllMocks();
});

describe("meeting live stream", () => {
  it("parses ready and update events across response chunks", async () => {
    setAccessToken("access-token");
    const encoder = new TextEncoder();
    const stream = new ReadableStream<Uint8Array>({
      start(controller) {
        controller.enqueue(encoder.encode("event: ready\ndata: {\"version\":2}\n"));
        controller.enqueue(encoder.encode("\nevent: meeting.updated\ndata: {\"version\":4}\n\n"));
        controller.close();
      },
    });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(stream));
    const events: MeetingLiveEvent[] = [];

    await api.watchMeeting("meeting-id", new AbortController().signal, (event) => events.push(event));

    expect(events).toEqual([
      { type: "ready", version: 2 },
      { type: "meeting.updated", version: 4 },
    ]);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/meetings/meeting-id/events",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
        headers: expect.any(Headers),
      }),
    );
    const request = fetchMock.mock.calls[0]?.[1];
    expect(new Headers(request?.headers).get("Authorization")).toBe("Bearer access-token");
    expect(fetchMock.mock.calls[0]?.[0]).not.toContain("access-token");
  });
});

describe("meeting calendar export", () => {
  it("downloads an authenticated calendar file without putting the token in the URL", async () => {
    setAccessToken("calendar-access-token");
    const calendar = "BEGIN:VCALENDAR\r\nEND:VCALENDAR\r\n";
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(calendar, {
      status: 200,
      headers: { "Content-Type": "text/calendar; charset=utf-8" },
    }));

    const result = await api.exportMeetingCalendar("meeting/id");

    expect(result.type).toContain("text/calendar");
    expect(result.size).toBe(new TextEncoder().encode(calendar).byteLength);
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/meetings/meeting%2Fid/calendar.ics",
      expect.objectContaining({
        method: "GET",
        credentials: "include",
        headers: expect.any(Headers),
      }),
    );
    const request = fetchMock.mock.calls[0]?.[1];
    expect(new Headers(request?.headers).get("Authorization")).toBe("Bearer calendar-access-token");
    expect(fetchMock.mock.calls[0]?.[0]).not.toContain("calendar-access-token");
  });
});

describe("meeting photo upload", () => {
  it("sends the original bounded file with authorization and meeting version", async () => {
    setAccessToken("photo-access-token");
    const file = new File(["photo-bytes"], "place.png", { type: "image/png" });
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(new Response(JSON.stringify({
      version: 8,
      changed: true,
      updated_at: "2026-07-30T10:00:00Z",
    }), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    }));

    const result = await api.putPlanOptionPhoto("meeting/id", "plan/id", 7, file);

    expect(result).toMatchObject({ version: 8, changed: true });
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/v1/meetings/meeting%2Fid/plan-options/plan%2Fid/photo?version=7",
      expect.objectContaining({
        method: "PUT",
        credentials: "include",
        body: file,
        headers: expect.any(Headers),
      }),
    );
    const request = fetchMock.mock.calls[0]?.[1];
    const headers = new Headers(request?.headers);
    expect(headers.get("Authorization")).toBe("Bearer photo-access-token");
    expect(headers.get("Content-Type")).toBe("image/png");
  });
});

describe("direct meeting invitations", () => {
  it("sends a bounded friend selection and accepts an invitation through authenticated endpoints", async () => {
    setAccessToken("invite-access-token");
    const fetchMock = vi.spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(new Response(JSON.stringify({ changed_count: 2 }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }))
      .mockResolvedValueOnce(new Response(JSON.stringify({
        meeting_id: "meeting-1",
        changed: true,
        joined: true,
      }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }));

    await api.sendMeetingInvites("meeting/id", ["friend-1", "friend-2"]);
    const accepted = await api.acceptMeetingInvite("invite/id");

    expect(accepted).toMatchObject({ meeting_id: "meeting-1", joined: true });
    expect(fetchMock.mock.calls[0]?.[0]).toBe("/api/v1/meetings/meeting%2Fid/friend-invitations");
    expect(fetchMock.mock.calls[0]?.[1]).toEqual(expect.objectContaining({
      method: "POST",
      body: JSON.stringify({ user_ids: ["friend-1", "friend-2"] }),
    }));
    expect(fetchMock.mock.calls[1]?.[0]).toBe("/api/v1/me/meeting-invitations/invite%2Fid");
    expect(fetchMock.mock.calls[1]?.[1]).toEqual(expect.objectContaining({ method: "PUT" }));
  });
});
