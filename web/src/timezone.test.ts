import { describe, expect, it } from "vitest";
import {
  createMeetingDateTimeFormatter,
  isoToZonedDateTimeLocal,
  meetingTimeZoneLabel,
  zonedDateTimeToISO,
} from "./timezone";

describe("meeting timezone utilities", () => {
  it("converts a meeting-local value to an absolute instant", () => {
    expect(zonedDateTimeToISO("2026-08-10T19:30", "Asia/Novosibirsk"))
      .toBe("2026-08-10T12:30:00.000Z");
  });

  it("fills a datetime-local input in the meeting timezone", () => {
    expect(isoToZonedDateTimeLocal(
      "2026-08-10T12:30:00.000Z",
      "Asia/Novosibirsk",
    )).toBe("2026-08-10T19:30");
  });

  it("formats the same instant differently in each meeting timezone", () => {
    const instant = new Date("2026-08-10T12:30:00.000Z");
    const novosibirsk = createMeetingDateTimeFormatter("Asia/Novosibirsk").format(instant);
    const moscow = createMeetingDateTimeFormatter("Europe/Moscow").format(instant);

    expect(novosibirsk).toMatch(/19:30/);
    expect(moscow).toMatch(/15:30/);
    expect(novosibirsk).toMatch(/GMT\+7/i);
    expect(moscow).toMatch(/GMT\+3/i);
  });

  it("rejects a local time skipped by a daylight-saving transition", () => {
    expect(() => zonedDateTimeToISO(
      "2026-03-08T02:30",
      "America/New_York",
    )).toThrow(/Такого местного времени нет/);
  });

  it("shows a readable timezone and current offset", () => {
    expect(meetingTimeZoneLabel(
      "Asia/Novosibirsk",
      new Date("2026-08-10T12:30:00.000Z"),
    )).toBe("Новосибирск · UTC+7");
  });
});
