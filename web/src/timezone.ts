type DateTimeParts = {
  year: number;
  month: number;
  day: number;
  hour: number;
  minute: number;
  second: number;
};

const localDateTimePattern =
  /^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?$/;

const timeZoneNames: Record<string, string> = {
  UTC: "UTC",
  "Europe/Kaliningrad": "Калининград",
  "Europe/Moscow": "Москва",
  "Europe/Samara": "Самара",
  "Asia/Yekaterinburg": "Екатеринбург",
  "Asia/Omsk": "Омск",
  "Asia/Novosibirsk": "Новосибирск",
  "Asia/Barnaul": "Барнаул",
  "Asia/Tomsk": "Томск",
  "Asia/Novokuznetsk": "Новокузнецк",
  "Asia/Krasnoyarsk": "Красноярск",
  "Asia/Irkutsk": "Иркутск",
  "Asia/Chita": "Чита",
  "Asia/Yakutsk": "Якутск",
  "Asia/Vladivostok": "Владивосток",
  "Asia/Magadan": "Магадан",
  "Asia/Sakhalin": "Сахалин",
  "Asia/Kamchatka": "Камчатка",
  "Asia/Anadyr": "Анадырь",
};

export function createMeetingDateTimeFormatter(
  timeZone: string,
  style: "regular" | "compact" = "regular",
): Intl.DateTimeFormat {
  return new Intl.DateTimeFormat("ru-RU", {
    timeZone,
    day: "numeric",
    month: "short",
    ...(style === "regular" ? { year: "numeric" as const } : {}),
    hour: "2-digit",
    minute: "2-digit",
    timeZoneName: "short",
  });
}

export function meetingTimeZoneLabel(
  timeZone: string,
  at: Date = new Date(),
): string {
  const offset = new Intl.DateTimeFormat("ru-RU", {
    timeZone,
    hour: "2-digit",
    timeZoneName: "shortOffset",
  }).formatToParts(at).find((part) => part.type === "timeZoneName")?.value;
  const name = timeZoneNames[timeZone] ?? "Часовой пояс встречи";
  const readableOffset = offset?.replace(/^GMT/, "UTC");
  return readableOffset ? `${name} · ${readableOffset}` : name;
}

export function isoToZonedDateTimeLocal(value: string, timeZone: string): string {
  const instant = new Date(value);
  if (Number.isNaN(instant.getTime())) {
    throw new RangeError("Некорректная дата.");
  }
  const parts = partsInTimeZone(instant.getTime(), timeZone);
  return [
    String(parts.year).padStart(4, "0"),
    String(parts.month).padStart(2, "0"),
    String(parts.day).padStart(2, "0"),
  ].join("-") + `T${String(parts.hour).padStart(2, "0")}:${String(parts.minute).padStart(2, "0")}`;
}

export function zonedDateTimeToISO(value: string, timeZone: string): string {
  const target = parseLocalDateTime(value);
  const targetAsUTC = Date.UTC(
    target.year,
    target.month - 1,
    target.day,
    target.hour,
    target.minute,
    target.second,
  );
  let instant = targetAsUTC;

  for (let attempt = 0; attempt < 4; attempt += 1) {
    const actual = partsInTimeZone(instant, timeZone);
    const actualAsUTC = Date.UTC(
      actual.year,
      actual.month - 1,
      actual.day,
      actual.hour,
      actual.minute,
      actual.second,
    );
    const correction = targetAsUTC - actualAsUTC;
    if (correction === 0) break;
    instant += correction;
  }

  const resolved = partsInTimeZone(instant, timeZone);
  if (!sameDateTime(resolved, target)) {
    throw new RangeError(
      `Такого местного времени нет в часовом поясе ${meetingTimeZoneLabel(timeZone, new Date(instant))}.`,
    );
  }
  return new Date(instant).toISOString();
}

function parseLocalDateTime(value: string): DateTimeParts {
  const match = localDateTimePattern.exec(value);
  if (!match) {
    throw new RangeError("Укажите корректные дату и время.");
  }
  const parts: DateTimeParts = {
    year: Number(match[1]),
    month: Number(match[2]),
    day: Number(match[3]),
    hour: Number(match[4]),
    minute: Number(match[5]),
    second: Number(match[6] ?? "0"),
  };
  const check = new Date(Date.UTC(
    parts.year,
    parts.month - 1,
    parts.day,
    parts.hour,
    parts.minute,
    parts.second,
  ));
  const valid = check.getUTCFullYear() === parts.year
    && check.getUTCMonth() + 1 === parts.month
    && check.getUTCDate() === parts.day
    && check.getUTCHours() === parts.hour
    && check.getUTCMinutes() === parts.minute
    && check.getUTCSeconds() === parts.second;
  if (!valid) {
    throw new RangeError("Укажите корректные дату и время.");
  }
  return parts;
}

function partsInTimeZone(instant: number, timeZone: string): DateTimeParts {
  const formatter = new Intl.DateTimeFormat("en-CA", {
    timeZone,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
    hourCycle: "h23",
  });
  const parts = Object.fromEntries(
    formatter.formatToParts(new Date(instant))
      .filter((part) => part.type !== "literal")
      .map((part) => [part.type, part.value]),
  );
  return {
    year: Number(parts.year),
    month: Number(parts.month),
    day: Number(parts.day),
    hour: Number(parts.hour),
    minute: Number(parts.minute),
    second: Number(parts.second),
  };
}

function sameDateTime(left: DateTimeParts, right: DateTimeParts): boolean {
  return left.year === right.year
    && left.month === right.month
    && left.day === right.day
    && left.hour === right.hour
    && left.minute === right.minute
    && left.second === right.second;
}
