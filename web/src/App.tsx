import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import {
  api,
  type AttendanceStatus,
  type AttendanceView,
  type AvailabilityStatus,
  type AvailabilityView,
  type CreatePollInput,
  type FriendItem,
  type FriendSearchItem,
  type FriendsOverview,
  type IncomingMeetingInvite,
  type Meeting,
  type MeetingDetail,
  type MeetingNotePage,
  type PlanOption,
  type PlanVotePage,
  type Poll,
  type PollHistoryPage,
  type Requirement,
  type RequirementInput,
  type RequirementPage,
  type UpdateMeetingInput,
  type UpdatePlanOptionInput,
  type UpdateTimeOptionInput,
  type TimeOption,
  type User,
} from "./api";
import { AttendanceSection } from "./features/meeting/fixed-meeting";
import { MeetingInvitationsDialog } from "./features/meeting/meeting-invitations";
import { MeetingShareDialog } from "./features/meeting/meeting-share-dialog";
import {
  AppHeader,
  Avatar,
  Brand,
  LoadingScreen,
  StoredAvatarImage,
  ThemeButton,
} from "./components/app-shell";
import { getInitialTheme, type Theme } from "./theme";
import {
  createMeetingDateTimeFormatter,
  isoToZonedDateTimeLocal,
  meetingTimeZoneLabel,
  zonedDateTimeToISO,
} from "./timezone";
import {
  AVATAR_OUTPUT_WIDTH,
  AVATAR_UPLOAD_MAX_BYTES,
  cropAndCompressPhoto,
  movePhotoCrop,
  photoCropFrameGeometry,
  type PhotoCrop,
  validatePhotoSource,
} from "./photo";
import { errorMessage } from "./error-message";

type AuthMode = "login" | "register";
type LiveState = "idle" | "connecting" | "live" | "reconnecting";
type MeetingSection = "overview" | "votes" | "availability" | "attendance" | "polls" | "notes" | "preparation" | "people" | "manage";
type MeetingListView = "active" | "archive";
type MeetingSort = "start" | "joined" | "attendance";

const OPTION_PREVIEW_LIMIT = 5;
const PARTICIPANT_PREVIEW_LIMIT = 8;
const RECOMMENDATION_PREVIEW_LIMIT = 4;
const stateLabels: Record<Meeting["state"], string> = {
  draft: "Черновик",
  collecting: "Собираем ответы",
  scheduled: "Подтверждена",
  cancelled: "Отменена",
  completed: "Завершена",
};

const attendanceLabels = {
  going: "Иду",
  maybe: "Думаю",
  not_going: "Не иду",
  unanswered: "Не ответил",
} as const;

const attendanceMarks: Record<AttendanceStatus, string> = {
  going: "✓",
  maybe: "…",
  not_going: "×",
  unanswered: "?",
};

function countLabel(count: number, one: string, few: string, many: string): string {
  const absolute = Math.abs(count) % 100;
  const last = absolute % 10;
  const form = absolute > 10 && absolute < 20
    ? many
    : last === 1
      ? one
      : last >= 2 && last <= 4
        ? few
        : many;
  return `${count} ${form}`;
}

function isArchivedMeeting(meeting: Meeting, now = Date.now()): boolean {
  if (meeting.state === "cancelled" || meeting.state === "completed") return true;
  const finishedAt = meeting.selected_ends_at ?? meeting.selected_starts_at;
  return Boolean(finishedAt && new Date(finishedAt).getTime() + 24 * 60 * 60 * 1000 <= now);
}

function meetingCardDate(meeting: Meeting): string {
  if (!meeting.selected_starts_at) return "Дата ещё не выбрана";
  return new Intl.DateTimeFormat("ru-RU", {
    day: "numeric",
    month: "long",
    hour: "2-digit",
    minute: "2-digit",
    timeZone: meeting.timezone,
  }).format(new Date(meeting.selected_starts_at));
}

function invitationFromHash(): string | null {
  const match = window.location.hash.match(/^#\/invite\/([A-Za-z0-9_-]+)$/);
  return match?.[1] ?? null;
}

export function App() {
  const [theme, setTheme] = useState<Theme>(getInitialTheme);
  const [user, setUser] = useState<User | null>(null);
  const [booting, setBooting] = useState(true);
  const [invitationToken, setInvitationToken] = useState<string | null>(invitationFromHash);
  const [showProfile, setShowProfile] = useState(false);
  const [showFriends, setShowFriends] = useState(false);
  const [showMeetingInvites, setShowMeetingInvites] = useState(false);
  const [meetingInvites, setMeetingInvites] = useState<IncomingMeetingInvite[]>([]);
  const [openMeetingID, setOpenMeetingID] = useState<string | null>(null);

  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    localStorage.setItem("ryden-theme", theme);
  }, [theme]);

  useEffect(() => {
    const readInvitation = () => setInvitationToken(invitationFromHash());
    window.addEventListener("hashchange", readInvitation);
    return () => window.removeEventListener("hashchange", readInvitation);
  }, []);

  const refreshMeetingInvites = useCallback(async () => {
    if (!user) return;
    try {
      const page = await api.getIncomingMeetingInvites();
      setMeetingInvites(page.items);
    } catch {
      // The meeting list remains usable while the compact notification read retries later.
    }
  }, [user]);

  useEffect(() => {
    if (!user) {
      setMeetingInvites([]);
      return;
    }
    void refreshMeetingInvites();
    const refreshOnFocus = () => void refreshMeetingInvites();
    window.addEventListener("focus", refreshOnFocus);
    const interval = window.setInterval(refreshOnFocus, 60_000);
    return () => {
      window.removeEventListener("focus", refreshOnFocus);
      window.clearInterval(interval);
    };
  }, [refreshMeetingInvites, user]);

  useEffect(() => {
    let active = true;
    void api.bootstrap().then((session) => {
      if (active) {
        setUser(session?.user ?? null);
        setBooting(false);
      }
    });
    return () => {
      active = false;
    };
  }, []);

  const toggleTheme = () => setTheme((current) => (current === "light" ? "dark" : "light"));

  if (booting) {
    return <LoadingScreen theme={theme} onThemeChange={toggleTheme} />;
  }

  if (!user) {
    return (
      <Welcome
        theme={theme}
        invitationPending={Boolean(invitationToken)}
        onThemeChange={toggleTheme}
        onAuthenticated={setUser}
      />
    );
  }

  return (
    <>
      <Dashboard
        user={user}
        theme={theme}
        invitationToken={invitationToken}
        onInvitationHandled={() => {
          setInvitationToken(null);
          window.history.replaceState(null, "", `${window.location.pathname}${window.location.search}`);
        }}
        onThemeChange={toggleTheme}
        onFriends={() => setShowFriends(true)}
        incomingInviteCount={meetingInvites.length}
        onMeetingInvites={() => {
          setShowMeetingInvites(true);
          void refreshMeetingInvites();
        }}
        openMeetingID={openMeetingID}
        onOpenMeetingHandled={() => setOpenMeetingID(null)}
        onEditProfile={() => setShowProfile(true)}
        onLogout={async () => {
          try {
            await api.logout();
          } finally {
            setShowProfile(false);
            setShowFriends(false);
            setShowMeetingInvites(false);
            setMeetingInvites([]);
            setUser(null);
          }
        }}
      />
      {showProfile && (
        <ProfileDialog
          user={user}
          onClose={() => setShowProfile(false)}
          onSaved={(updated) => {
            setUser(updated);
            setShowProfile(false);
          }}
        />
      )}
      {showFriends && (
        <FriendsDialog onClose={() => setShowFriends(false)} />
      )}
      {showMeetingInvites && (
        <MeetingInvitationsDialog
          invitations={meetingInvites}
          onClose={() => setShowMeetingInvites(false)}
          onAccepted={(invitation) => {
            setMeetingInvites((current) => current.filter((item) => item.id !== invitation.id));
            setShowMeetingInvites(false);
            setOpenMeetingID(invitation.meeting_id);
          }}
          onDeclined={(invitationID) => {
            setMeetingInvites((current) => current.filter((item) => item.id !== invitationID));
          }}
        />
      )}
    </>
  );
}

function Welcome({
  theme,
  invitationPending,
  onThemeChange,
  onAuthenticated,
}: {
  theme: Theme;
  invitationPending: boolean;
  onThemeChange: () => void;
  onAuthenticated: (user: User) => void;
}) {
  const [mode, setMode] = useState<AuthMode>("register");
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    setError("");
    const data = new FormData(event.currentTarget);
    try {
      const session =
        mode === "register"
          ? await api.register({
              display_name: String(data.get("display_name") ?? ""),
              nickname: String(data.get("nickname") ?? ""),
              email: String(data.get("email") ?? ""),
              password: String(data.get("password") ?? ""),
            })
          : await api.login({
              email: String(data.get("email") ?? ""),
              password: String(data.get("password") ?? ""),
            });
      onAuthenticated(session.user);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="page-shell welcome-page">
      <header className="topbar">
        <Brand />
        <ThemeButton theme={theme} onChange={onThemeChange} />
      </header>
      <main className="welcome-layout">
        <section className="welcome-copy" aria-labelledby="welcome-title">
          <p className="eyebrow">ПЛАН ДЛЯ СВОИХ</p>
          <h1 id="welcome-title">
            Встреча начинается
            <br />
            с ясного <em>решения.</em>
          </h1>
          <p className="welcome-lead">
            Соберите варианты, найдите общее время и распределите подготовку — без потерянных
            сообщений и неясных обещаний.
          </p>
          <ol className="journey-list" aria-label="Как работает Ryden">
            <li>
              <span>1</span>
              <div><strong>Предложите</strong><small>планы и время</small></div>
            </li>
            <li>
              <span>2</span>
              <div><strong>Сравните</strong><small>голоса и доступность</small></div>
            </li>
            <li>
              <span>3</span>
              <div><strong>Подтвердите</strong><small>решение и подготовку</small></div>
            </li>
          </ol>
        </section>

        <section className="auth-card" aria-labelledby="auth-title">
          {invitationPending && (
            <div className="invite-pending" role="status">
              <strong>Вас пригласили на встречу.</strong>
              <span>Войдите или создайте аккаунт — и сразу откроется приглашённая встреча.</span>
            </div>
          )}
          <div className="auth-tabs" role="tablist" aria-label="Действие с аккаунтом">
            <button
              role="tab"
              aria-selected={mode === "register"}
              className={mode === "register" ? "active" : ""}
              onClick={() => {
                setMode("register");
                setError("");
              }}
              type="button"
            >
              Создать аккаунт
            </button>
            <button
              role="tab"
              aria-selected={mode === "login"}
              className={mode === "login" ? "active" : ""}
              onClick={() => {
                setMode("login");
                setError("");
              }}
              type="button"
            >
              Войти
            </button>
          </div>
          <div className="auth-card-body">
            <p className="section-kicker">{mode === "register" ? "НОВЫЙ АККАУНТ" : "ВХОД В АККАУНТ"}</p>
            <h2 id="auth-title">{mode === "register" ? "Создайте аккаунт" : "С возвращением"}</h2>
            <p className="muted">
              {mode === "register"
                ? "Приглашения открываются только после входа — ваши планы остаются среди своих."
                : "Войдите, чтобы вернуться к встречам."}
            </p>
            <form onSubmit={submit} className="auth-form">
              {mode === "register" && (
                <>
                  <Field label="Как к вам обращаться" name="display_name" autoComplete="name" minLength={1} maxLength={80} />
                  <Field
                    label="Никнейм"
                    name="nickname"
                    autoComplete="username"
                    minLength={3}
                    maxLength={24}
                    pattern="[A-Za-z][A-Za-z0-9_]{2,23}"
                    hint="3–24 символа: латинские буквы, цифры и одиночное подчёркивание"
                  />
                </>
              )}
              <Field label="Электронная почта" name="email" type="email" autoComplete="email" maxLength={254} />
              <Field
                label="Пароль"
                name="password"
                type="password"
                autoComplete={mode === "register" ? "new-password" : "current-password"}
                minLength={10}
                maxLength={128}
                hint={mode === "register" ? "Не менее 10 знаков" : undefined}
              />
              {error && <p className="form-error" role="alert">{error}</p>}
              <button className="primary-button" disabled={submitting} type="submit">
                {submitting ? "Подождите…" : mode === "register" ? "Создать аккаунт" : "Войти"}
                <span aria-hidden="true">→</span>
              </button>
            </form>
          </div>
        </section>
      </main>
      <footer className="welcome-footer">
        <span>Решение за организатором</span>
        <span>Приватно по приглашению</span>
        <span>Подготовка без хаоса</span>
      </footer>
    </div>
  );
}

interface FieldProps {
  label: string;
  name: string;
  type?: string;
  autoComplete: string;
  minLength?: number;
  maxLength?: number;
  hint?: string;
  autoFocus?: boolean;
  placeholder?: string;
  defaultValue?: string;
  pattern?: string;
}

function Field({ label, hint, ...input }: FieldProps) {
  return (
    <label className="field">
      <span>
        {label}
        {hint && <small>{hint}</small>}
      </span>
      <input {...input} required />
    </label>
  );
}

function roundedStartISO(): string {
  const interval = 5 * 60 * 1000;
  return new Date(Math.ceil(Date.now() / interval) * interval).toISOString();
}

function splitZonedDateTime(iso: string, timeZone: string): { date: string; time: string } {
  const local = isoToZonedDateTimeLocal(iso, timeZone);
  return { date: local.slice(0, 10), time: local.slice(11, 16) };
}

function durationParts(startsAt: string, endsAt: string | null | undefined): {
  days: string;
  hours: string;
  minutes: string;
} {
  if (!endsAt) return { days: "", hours: "", minutes: "" };
  const total = Math.max(
    0,
    Math.round((new Date(endsAt).getTime() - new Date(startsAt).getTime()) / 60_000),
  );
  const days = Math.floor(total / 1440);
  const hours = Math.floor((total % 1440) / 60);
  const minutes = total % 60;
  return {
    days: days > 0 ? String(days) : "",
    hours: hours > 0 ? String(hours) : "",
    minutes: minutes > 0 ? String(minutes) : "",
  };
}

function formatDurationShort(startsAt: string, endsAt: string | null | undefined): string {
  if (!endsAt) return "";
  const parts = durationParts(startsAt, endsAt);
  return [
    parts.days && `${parts.days}д`,
    parts.hours && `${parts.hours}ч`,
    parts.minutes && `${parts.minutes}м`,
  ].filter(Boolean).join(" ");
}

function normalizeDurationInput(event: FormEvent<HTMLInputElement>) {
  const input = event.currentTarget;
  const normalized = input.value.replace(/^0+(?=\d)/u, "");
  if (normalized !== input.value) input.value = normalized;
}

function readTimeSelection(data: FormData, timeZone: string): {
  startsAt: string;
  endsAt: string | null;
} {
  const date = String(data.get("start_date") ?? "");
  const timeValue = String(data.get("start_time") ?? "");
  const startsAt = zonedDateTimeToISO(`${date}T${timeValue}`, timeZone);
  const rawDuration = [
    String(data.get("duration_days") ?? "").trim(),
    String(data.get("duration_hours") ?? "").trim(),
    String(data.get("duration_minutes") ?? "").trim(),
  ];
  if (rawDuration.some((value) => value !== "" && !/^\d+$/u.test(value))) {
    throw new Error("Длительность указывается целыми числами.");
  }
  const [days, hours, minutes] = rawDuration.map((value) => value === "" ? 0 : Number(value));
  if (days > 30 || hours > 23 || minutes > 59) {
    throw new Error("Укажите не больше 30 дней, 23 часов и 59 минут.");
  }
  const duration = days * 1440 + hours * 60 + minutes;
  if (duration > 30 * 1440 + 23 * 60 + 59) {
    throw new Error("Длительность не может превышать 30 дней 23 часа 59 минут.");
  }
  return {
    startsAt,
    endsAt: duration > 0
      ? new Date(new Date(startsAt).getTime() + duration * 60_000).toISOString()
      : null,
  };
}

function MeetingTimeFields({
  timeZone,
  defaultStart,
  defaultEnd,
}: {
  timeZone: string;
  defaultStart?: string;
  defaultEnd?: string | null;
}) {
  const startISO = useMemo(() => defaultStart ?? roundedStartISO(), [defaultStart]);
  const start = splitZonedDateTime(startISO, timeZone);
  const duration = durationParts(startISO, defaultEnd);
  const today = splitZonedDateTime(new Date().toISOString(), timeZone).date;
  return (
    <fieldset className="create-time-fields">
      <legend>
        Когда встречаемся
        <small>{meetingTimeZoneLabel(timeZone)}</small>
      </legend>
      <label className="field">
        <span>Дата</span>
        <input
          defaultValue={start.date}
          min={today}
          name="start_date"
          type="date"
          required
        />
      </label>
      <label className="field">
        <span>Время</span>
        <input
          defaultValue={start.time}
          name="start_time"
          step={300}
          type="time"
          required
        />
      </label>
      <div className="duration-fields">
        <span>Длительность <small>необязательно · если указываете, заполните хотя бы одно поле</small></span>
        <label className="field">
          <span>Дни</span>
          <input defaultValue={duration.days} inputMode="numeric" max={30} min={0} name="duration_days" onInput={normalizeDurationInput} placeholder="0" step={1} type="number" />
        </label>
        <label className="field">
          <span>Часы</span>
          <input defaultValue={duration.hours} inputMode="numeric" max={23} min={0} name="duration_hours" onInput={normalizeDurationInput} placeholder="0" step={1} type="number" />
        </label>
        <label className="field">
          <span>Минуты</span>
          <input defaultValue={duration.minutes} inputMode="numeric" max={59} min={0} name="duration_minutes" onInput={normalizeDurationInput} placeholder="0" step={1} type="number" />
        </label>
      </div>
    </fieldset>
  );
}

function Dashboard({
  user,
  theme,
  invitationToken,
  onInvitationHandled,
  onThemeChange,
  onFriends,
  incomingInviteCount,
  onMeetingInvites,
  openMeetingID,
  onOpenMeetingHandled,
  onEditProfile,
  onLogout,
}: {
  user: User;
  theme: Theme;
  invitationToken: string | null;
  onInvitationHandled: () => void;
  onThemeChange: () => void;
  onFriends: () => void;
  incomingInviteCount: number;
  onMeetingInvites: () => void;
  openMeetingID: string | null;
  onOpenMeetingHandled: () => void;
  onEditProfile: () => void;
  onLogout: () => Promise<void>;
}) {
  const [meetings, setMeetings] = useState<Meeting[]>([]);
  const [selectedID, setSelectedID] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [creating, setCreating] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const [photoToCrop, setPhotoToCrop] = useState<File | null>(null);
  const [pendingPhoto, setPendingPhoto] = useState<File | null>(null);
  const [createdInvitation, setCreatedInvitation] = useState<{ meetingID: string; link: string } | null>(null);
  const [selectedNotice, setSelectedNotice] = useState("");
  const [listView, setListView] = useState<MeetingListView>("active");
  const [meetingSort, setMeetingSort] = useState<MeetingSort>("start");

  async function loadMeetings() {
    setLoading(true);
    setError("");
    try {
      const page = await api.listMeetings();
      setMeetings(page.items);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadMeetings();
  }, []);

  useEffect(() => {
    if (!openMeetingID) return;
    setSelectedID(openMeetingID);
    onOpenMeetingHandled();
  }, [onOpenMeetingHandled, openMeetingID]);

  useEffect(() => {
    if (!invitationToken) {
      return;
    }
    let active = true;
    void api.joinInvitation(invitationToken)
      .then((joined) => {
        if (!active) return;
        setMeetings((current) => [joined, ...current.filter((item) => item.id !== joined.id)]);
        setSelectedID(joined.id);
        onInvitationHandled();
      })
      .catch((requestError) => {
        if (active) setError(errorMessage(requestError));
      });
    return () => {
      active = false;
    };
  }, [invitationToken, onInvitationHandled]);

  useEffect(() => {
    if (!showCreate) return;
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape") setShowCreate(false);
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [showCreate]);

  const firstName = useMemo(() => user.display_name.split(/\s+/)[0], [user.display_name]);
  const creationTimeZone = useMemo(
    () => Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC",
    [],
  );
  const handleMeetingUpdated = useCallback((updated: Meeting) => {
    setMeetings((current) => current.map((item) => item.id === updated.id ? updated : item));
  }, []);

  const visibleMeetings = useMemo(() => {
    const attendanceOrder = { going: 0, maybe: 1, unanswered: 2, not_going: 3 } as const;
    return meetings
      .filter((item) => isArchivedMeeting(item) === (listView === "archive"))
      .slice()
      .sort((left, right) => {
        if (meetingSort === "joined") {
          return new Date(right.participant_joined_at ?? right.created_at).getTime()
            - new Date(left.participant_joined_at ?? left.created_at).getTime();
        }
        if (meetingSort === "attendance") {
          const leftRank = left.my_attendance_status ? attendanceOrder[left.my_attendance_status] : 4;
          const rightRank = right.my_attendance_status ? attendanceOrder[right.my_attendance_status] : 4;
          if (leftRank !== rightRank) return leftRank - rightRank;
        }
        const leftTime = left.selected_starts_at ? new Date(left.selected_starts_at).getTime() : Number.MAX_SAFE_INTEGER;
        const rightTime = right.selected_starts_at ? new Date(right.selected_starts_at).getTime() : Number.MAX_SAFE_INTEGER;
        return leftTime - rightTime;
      });
  }, [listView, meetingSort, meetings]);

  const activeCount = meetings.filter((item) => !isArchivedMeeting(item)).length;
  const archiveCount = meetings.length - activeCount;

  function closeCreateDialog() {
    if (creating) return;
    setShowCreate(false);
    setPhotoToCrop(null);
    setPendingPhoto(null);
  }

  function selectPhotoSource(file: File) {
    const photoError = validatePhotoSource(file);
    if (photoError) {
      setError(photoError);
      return;
    }
    setError("");
    setPhotoToCrop(file);
  }

  async function createMeeting(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setCreating(true);
    setError("");
    setSelectedNotice("");
    const form = event.currentTarget;
    const data = new FormData(form);
    let created: Meeting | null = null;
    let failedStep: "meeting" | "photo" | "invitation" = "meeting";
    try {
      const { startsAt, endsAt } = readTimeSelection(data, creationTimeZone);
      created = await api.createMeeting({
        title: String(data.get("title") ?? ""),
        description: String(data.get("description") ?? ""),
        event_type: "other",
        coordination_mode: "fixed",
        cover_url: null,
        location_name: String(data.get("location_name") ?? "").trim() || null,
        location_url: String(data.get("location_url") ?? "").trim() || null,
        timezone: creationTimeZone,
        starts_at: startsAt,
        ends_at: endsAt,
      });
      if (pendingPhoto) {
        failedStep = "photo";
        const mutation = await api.putMeetingPhoto(created.id, created.version, pendingPhoto);
        created = { ...created, has_photo: true, version: mutation.version, updated_at: mutation.updated_at };
      }
      failedStep = "invitation";
      const secret = newInvitationSecret();
      await api.createInvitation(created.id, secret);
      const link = `${window.location.origin}/#/invite/${secret}`;
      setCreatedInvitation({ meetingID: created.id, link });
      setMeetings((current) => [created as Meeting, ...current]);
      setSelectedID(created.id);
      setShowCreate(false);
      setPendingPhoto(null);
      setPhotoToCrop(null);
      form.reset();
    } catch (requestError) {
      if (created) {
        setMeetings((current) => [created as Meeting, ...current]);
        setSelectedID(created.id);
        setShowCreate(false);
        setSelectedNotice(
          failedStep === "photo"
            ? `Встреча создана, но фото не загрузилось: ${errorMessage(requestError)}`
            : `Встреча создана, но сбор участия не запустился: ${errorMessage(requestError)}`,
        );
      } else {
        setError(errorMessage(requestError));
      }
    } finally {
      setCreating(false);
    }
  }

  if (selectedID) {
    return (
      <MeetingDetailView
        meetingID={selectedID}
        user={user}
        theme={theme}
        onThemeChange={onThemeChange}
        onFriends={onFriends}
        incomingInviteCount={incomingInviteCount}
        onMeetingInvites={onMeetingInvites}
        onEditProfile={onEditProfile}
        onBack={() => {
          setSelectedID(null);
          setCreatedInvitation(null);
          setSelectedNotice("");
          void loadMeetings();
        }}
        initialInviteLink={createdInvitation?.meetingID === selectedID ? createdInvitation.link : ""}
        initialNotice={selectedNotice}
        archived={isArchivedMeeting(meetings.find((item) => item.id === selectedID) ?? {
          state: "draft",
        } as Meeting)}
        onMeetingUpdated={handleMeetingUpdated}
        onLogout={onLogout}
      />
    );
  }

  return (
    <div className="app-shell">
      <AppHeader
        user={user}
        theme={theme}
        onThemeChange={onThemeChange}
        onFriends={onFriends}
        incomingInviteCount={incomingInviteCount}
        onMeetingInvites={onMeetingInvites}
        onEditProfile={onEditProfile}
        onLogout={onLogout}
      />
      <main className="dashboard">
        <section className="dashboard-heading">
          <div>
            <p className="eyebrow">ВАШИ ВСТРЕЧИ</p>
            <h1>Добрый день, {firstName}.</h1>
            <p className="muted">Здесь планы становятся встречами.</p>
          </div>
          <button className="primary-button compact" type="button" onClick={() => setShowCreate(true)}>
            <span aria-hidden="true">＋</span>
            Создать встречу
          </button>
        </section>

        {error && (
          <div className="notice error-notice" role="alert">
            <span>{error}</span>
            <button type="button" onClick={() => void loadMeetings()}>Повторить</button>
          </div>
        )}

        <section className="meeting-section" aria-labelledby="meeting-list-title">
          <div className="section-heading">
            <div>
              <h2 id="meeting-list-title">{listView === "active" ? "Встречи" : "Архив"}</h2>
              <p className="muted">
                {listView === "active" ? "Текущие планы и предстоящие встречи" : "Завершённые и отменённые встречи"}
              </p>
            </div>
            <div className="meeting-list-controls">
              <div className="meeting-list-switch" role="group" aria-label="Раздел встреч">
                <button className={listView === "active" ? "active" : ""} onClick={() => setListView("active")} type="button">
                  Активные · {activeCount}
                </button>
                <button className={listView === "archive" ? "active" : ""} onClick={() => setListView("archive")} type="button">
                  Архив · {archiveCount}
                </button>
              </div>
              <label className="meeting-sort">
                <span className="sr-only">Сортировка встреч</span>
                <select aria-label="Сортировка встреч" value={meetingSort} onChange={(event) => setMeetingSort(event.target.value as MeetingSort)}>
                  <option value="start">Ближайшие сначала</option>
                  <option value="joined">Недавно добавленные</option>
                  <option value="attendance">Сначала те, куда я иду</option>
                </select>
              </label>
            </div>
          </div>
          {loading ? (
            <div className="list-state" aria-live="polite">
              <span className="loading-spinner" aria-hidden="true" />
              Загружаем встречи…
            </div>
          ) : visibleMeetings.length === 0 ? (
            <div className="empty-state">
              <span className="empty-icon" aria-hidden="true">+</span>
              <h3>{listView === "active" ? "Активных встреч пока нет" : "Архив пока пуст"}</h3>
              <p>{listView === "active" ? "Создайте встречу и поделитесь приглашением с друзьями." : "Встреча появится здесь через сутки после окончания."}</p>
              {listView === "active" && <button className="text-button" type="button" onClick={() => setShowCreate(true)}>
                Создать встречу <span aria-hidden="true">→</span>
              </button>}
            </div>
          ) : (
            <div className="meeting-grid">
              {visibleMeetings.map((item) => (
                <button className={`meeting-card${listView === "archive" ? " archived" : ""}`} type="button" key={item.id} onClick={() => setSelectedID(item.id)}>
                  <span className="meeting-card-topline">
                    <span className={`status status-${item.state}`}>
                      <i aria-hidden="true" />
                      {listView === "archive" && item.state !== "cancelled" ? "В архиве" : stateLabels[item.state]}
                    </span>
                    {item.my_attendance_status && (
                      <span className={`card-attendance attendance-${item.my_attendance_status}`}>
                        {attendanceLabels[item.my_attendance_status]}
                      </span>
                    )}
                  </span>
                  <strong>{item.title}</strong>
                  <span className="card-description">{item.description || "Описание можно добавить позже."}</span>
                  <time dateTime={item.selected_starts_at}>{meetingCardDate(item)}</time>
                  <span className="card-meta">
                    <span>{item.participant_role === "owner" ? "Вы организатор" : "Вы участник"}</span>
                    <span aria-hidden="true">→</span>
                  </span>
                </button>
              ))}
            </div>
          )}
        </section>
      </main>

      {showCreate && (
        <div className="dialog-backdrop" role="presentation" onMouseDown={closeCreateDialog}>
          <section
            className="create-dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="create-title"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <button className="dialog-close" type="button" onClick={closeCreateDialog} aria-label="Закрыть">×</button>
            <p className="section-kicker">НОВАЯ ВСТРЕЧА</p>
            <h2 id="create-title">Создать встречу</h2>
            <p className="muted">Укажите детали готовой встречи. После создания можно сразу пригласить участников.</p>
            <form className="create-form" onSubmit={createMeeting}>
              <Field label="Название встречи" name="title" autoComplete="off" minLength={1} maxLength={120} autoFocus />
              <label className="field">
                <span>Описание <small>необязательно</small></span>
                <textarea name="description" maxLength={2000} rows={4} />
              </label>
              <MeetingTimeFields timeZone={creationTimeZone} />
              <label className="field photo-file-field">
                <span>Фото встречи <small>необязательно · JPEG или PNG до 20 МБ</small></span>
                <input
                  accept="image/jpeg,image/png"
                  onChange={(event) => {
                    const file = event.currentTarget.files?.[0];
                    event.currentTarget.value = "";
                    if (file) selectPhotoSource(file);
                  }}
                  type="file"
                />
              </label>
              {pendingPhoto && (
                <div className="pending-photo">
                  <LocalPhotoPreview file={pendingPhoto} />
                  <div>
                    <strong>Кадр готов</strong>
                    <small>Квадратный кадр готов к загрузке</small>
                    <button className="text-button" onClick={() => setPendingPhoto(null)} type="button">Убрать фото</button>
                  </div>
                </div>
              )}
              <label className="field">
                <span>Место <small>необязательно</small></span>
                <input
                  name="location_name"
                  autoComplete="off"
                  maxLength={200}
                  placeholder="Дом Анны или Северный парк"
                />
              </label>
              <label className="field">
                <span>Ссылка на место <small>необязательно · должна начинаться с https://</small></span>
                <input
                  name="location_url"
                  type="url"
                  inputMode="url"
                  maxLength={2048}
                  pattern="https://.*"
                  placeholder="https://maps.example.com/place"
                />
              </label>
              <button className="primary-button" disabled={creating} type="submit">
                {creating ? "Создаём и готовим ссылку…" : "Создать и пригласить"}
                <span aria-hidden="true">→</span>
              </button>
            </form>
          </section>
        </div>
      )}
      {photoToCrop && (
        <PhotoCropDialog
          file={photoToCrop}
          label="встречи"
          onCancel={() => setPhotoToCrop(null)}
          onConfirm={(file) => {
            setPendingPhoto(file);
            setPhotoToCrop(null);
          }}
        />
      )}
    </div>
  );
}

function StoredPhoto({
  meetingID,
  optionID,
  alt,
  revision,
}: {
  meetingID: string;
  optionID?: string;
  alt: string;
  revision: number;
}) {
  const [source, setSource] = useState("");
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    let active = true;
    let objectURL = "";
    setSource("");
    setFailed(false);
    const request = optionID
      ? api.getPlanOptionPhoto(meetingID, optionID, revision)
      : api.getMeetingPhoto(meetingID, revision);
    void request
      .then((file) => {
        if (!active) return;
        objectURL = URL.createObjectURL(file);
        setSource(objectURL);
      })
      .catch(() => {
        if (active) {
          setSource("");
          setFailed(true);
        }
      });
    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [meetingID, optionID, revision]);

  if (failed) {
    return <span className="photo-load-error" role="status">Не удалось показать фото</span>;
  }
  return source
    ? <img src={source} alt={alt} decoding="async" loading="lazy" />
    : <span className="photo-loading" aria-label="Загружаем фото" />;
}

function MeetingCover({
  meetingID,
  hasStoredPhoto,
  source,
  title,
  revision,
  attendanceStatus,
}: {
  meetingID: string;
  hasStoredPhoto: boolean;
  source: string | null | undefined;
  title: string;
  revision: number;
  attendanceStatus?: AttendanceStatus;
}) {
  const [failed, setFailed] = useState(false);

  useEffect(() => {
    setFailed(false);
  }, [hasStoredPhoto, revision, source]);

  if ((!hasStoredPhoto && !source) || failed) {
    return null;
  }
  return (
    <figure className="meeting-cover">
      {hasStoredPhoto ? (
        <StoredPhoto
          meetingID={meetingID}
          alt={`Фото встречи «${title}»`}
          revision={revision}
        />
      ) : (
        <img
          src={source ?? undefined}
          alt={`Обложка встречи «${title}»`}
          referrerPolicy="no-referrer"
          onError={() => setFailed(true)}
        />
      )}
      {attendanceStatus && (
        <figcaption className={`meeting-cover-attendance attendance-${attendanceStatus}`}>
          <b aria-hidden="true">{attendanceMarks[attendanceStatus]}</b>
          {attendanceLabels[attendanceStatus]}
        </figcaption>
      )}
    </figure>
  );
}

function PhotoControls({
  hasPhoto,
  label,
  working,
  onDelete,
  onSelect,
}: {
  hasPhoto: boolean;
  label: string;
  working: boolean;
  onDelete: () => void;
  onSelect: (file: File) => void;
}) {
  return (
    <div className="photo-controls">
      <label className={`photo-upload-button${working ? " disabled" : ""}`}>
        {hasPhoto ? `Заменить ${label}` : `Добавить ${label}`}
        <input
          accept="image/jpeg,image/png"
          disabled={working}
          type="file"
          onChange={(event) => {
            const file = event.currentTarget.files?.[0];
            event.currentTarget.value = "";
            if (file) onSelect(file);
          }}
        />
      </label>
      {hasPhoto && (
        <button className="quiet-button" disabled={working} onClick={onDelete} type="button">
          {`Удалить ${label}`}
        </button>
      )}
    </div>
  );
}

function LocalPhotoPreview({ file }: { file: File }) {
  const [source, setSource] = useState("");
  useEffect(() => {
    const objectURL = URL.createObjectURL(file);
    setSource(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [file]);
  return source ? <img src={source} alt="Подготовленный кадр" /> : null;
}

function PhotoCropDialog({
  file,
  label,
  variant = "meeting",
  onCancel,
  onConfirm,
}: {
  file: File;
  label: string;
  variant?: "meeting" | "avatar";
  onCancel: () => void;
  onConfirm: (file: File) => void;
}) {
  const [source, setSource] = useState("");
  const [crop, setCrop] = useState<PhotoCrop>({ x: 50, y: 50, zoom: 1 });
  const [imageSize, setImageSize] = useState({ width: 0, height: 0 });
  const [stageSize, setStageSize] = useState({ width: 0, height: 0 });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const stageRef = useRef<HTMLDivElement | null>(null);
  const cropRef = useRef<PhotoCrop>(crop);
  const pendingCrop = useRef<PhotoCrop | null>(null);
  const cropFrame = useRef<number | null>(null);
  const dragStart = useRef<{
    pointerID: number;
    clientX: number;
    clientY: number;
    cropX: number;
    cropY: number;
  } | null>(null);

  useEffect(() => {
    const objectURL = URL.createObjectURL(file);
    const initialCrop = { x: 50, y: 50, zoom: 1 };
    setSource(objectURL);
    cropRef.current = initialCrop;
    pendingCrop.current = null;
    setCrop(initialCrop);
    setImageSize({ width: 0, height: 0 });
    setError("");
    return () => {
      URL.revokeObjectURL(objectURL);
      if (cropFrame.current !== null) {
        window.cancelAnimationFrame(cropFrame.current);
        cropFrame.current = null;
      }
    };
  }, [file]);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const updateSize = () => {
      const bounds = stage.getBoundingClientRect();
      setStageSize({ width: bounds.width, height: bounds.height });
    };
    updateSize();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(updateSize);
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  const displayedPhotoSize = useMemo(() => {
    if (
      imageSize.width <= 0
      || imageSize.height <= 0
      || stageSize.width <= 0
      || stageSize.height <= 0
    ) {
      return { width: 0, height: 0 };
    }
    const scale = Math.min(
      stageSize.width / imageSize.width,
      stageSize.height / imageSize.height,
    );
    return {
      width: imageSize.width * scale,
      height: imageSize.height * scale,
    };
  }, [imageSize, stageSize]);
  const frame = photoCropFrameGeometry(crop, imageSize.width, imageSize.height);

  function commitCrop(next: PhotoCrop) {
    cropRef.current = next;
    pendingCrop.current = null;
    if (cropFrame.current !== null) {
      window.cancelAnimationFrame(cropFrame.current);
      cropFrame.current = null;
    }
    setCrop(next);
  }

  function scheduleCrop(next: PhotoCrop) {
    cropRef.current = next;
    pendingCrop.current = next;
    if (cropFrame.current !== null) return;
    cropFrame.current = window.requestAnimationFrame(() => {
      cropFrame.current = null;
      const scheduled = pendingCrop.current;
      pendingCrop.current = null;
      if (scheduled) setCrop(scheduled);
    });
  }

  async function confirm() {
    setSaving(true);
    setError("");
    try {
      onConfirm(await cropAndCompressPhoto(
        file,
        cropRef.current,
        variant === "avatar"
          ? {
            outputWidth: AVATAR_OUTPUT_WIDTH,
            maxBytes: AVATAR_UPLOAD_MAX_BYTES,
            outputName: "avatar",
          }
          : undefined,
      ));
    } catch (cropError) {
      setError(cropError instanceof Error ? cropError.message : "Не удалось подготовить фото.");
    } finally {
      setSaving(false);
    }
  }

  function changeZoom(delta: number) {
    const current = cropRef.current;
    scheduleCrop({
      ...current,
      zoom: Math.min(3, Math.max(1, Number((current.zoom + delta).toFixed(2)))),
    });
  }

  return (
    <div className="dialog-backdrop crop-dialog-backdrop" role="presentation" onMouseDown={onCancel}>
      <section
        aria-labelledby="crop-dialog-title"
        aria-modal="true"
        className="crop-dialog"
        onMouseDown={(event) => event.stopPropagation()}
        role="dialog"
      >
        <button aria-label="Закрыть кадрирование" className="dialog-close" disabled={saving} onClick={onCancel} type="button">×</button>
        <p className="section-kicker">ПОДГОТОВКА ФОТО</p>
        <h2 id="crop-dialog-title">Выберите кадр для {label}</h2>
        <p className="muted">Перемещайте квадрат по фотографии. Колесо мыши и кнопки меняют размер выбранного кадра.</p>
        <div className="crop-stage" ref={stageRef}>
          {source && (
            <div
              className="crop-photo"
              style={displayedPhotoSize.width > 0 ? {
                width: `${displayedPhotoSize.width}px`,
                height: `${displayedPhotoSize.height}px`,
              } : undefined}
            >
              <img
                alt=""
                className="crop-photo-base"
                draggable="false"
                onLoad={(event) => {
                  setImageSize({
                    width: event.currentTarget.naturalWidth,
                    height: event.currentTarget.naturalHeight,
                  });
                }}
                src={source}
              />
              <div
            aria-label="Область кадрирования"
            className="crop-preview"
            style={{
              left: `${frame.left}%`,
              top: `${frame.top}%`,
              width: `${frame.width}%`,
              height: `${frame.height}%`,
            }}
            onKeyDown={(event) => {
              const step = event.shiftKey ? 5 : 2;
              const current = cropRef.current;
              if (event.key === "ArrowLeft") commitCrop({ ...current, x: Math.max(0, current.x - step) });
              else if (event.key === "ArrowRight") commitCrop({ ...current, x: Math.min(100, current.x + step) });
              else if (event.key === "ArrowUp") commitCrop({ ...current, y: Math.max(0, current.y - step) });
              else if (event.key === "ArrowDown") commitCrop({ ...current, y: Math.min(100, current.y + step) });
              else return;
              event.preventDefault();
            }}
            onPointerCancel={() => {
              dragStart.current = null;
            }}
            onPointerDown={(event) => {
              dragStart.current = {
                pointerID: event.pointerId,
                clientX: event.clientX,
                clientY: event.clientY,
                cropX: cropRef.current.x,
                cropY: cropRef.current.y,
              };
              event.currentTarget.setPointerCapture?.(event.pointerId);
            }}
            onPointerMove={(event) => {
              const drag = dragStart.current;
              if (!drag || drag.pointerID !== event.pointerId) return;
              const photoBounds = event.currentTarget.parentElement?.getBoundingClientRect();
              if (!photoBounds) return;
              scheduleCrop(movePhotoCrop(
                {
                  x: drag.cropX,
                  y: drag.cropY,
                  zoom: cropRef.current.zoom,
                },
                event.clientX - drag.clientX,
                event.clientY - drag.clientY,
                photoBounds.width,
                photoBounds.height,
              ));
            }}
            onPointerUp={(event) => {
              if (dragStart.current?.pointerID === event.pointerId) {
                dragStart.current = null;
                event.currentTarget.releasePointerCapture?.(event.pointerId);
              }
            }}
            onWheel={(event) => {
              event.preventDefault();
              changeZoom(event.deltaY < 0 ? 0.1 : -0.1);
            }}
            role="group"
            tabIndex={0}
          >
            <img
              alt=""
              draggable="false"
              src={source}
              style={{
                left: `${frame.width > 0 ? -frame.left / frame.width * 100 : 0}%`,
                top: `${frame.height > 0 ? -frame.top / frame.height * 100 : 0}%`,
                width: `${frame.width > 0 ? 10000 / frame.width : 100}%`,
                height: `${frame.height > 0 ? 10000 / frame.height : 100}%`,
              }}
            />
            <span className="crop-grid" aria-hidden="true">
              <i />
              <i />
              <i />
              <i />
            </span>
              </div>
            </div>
          )}
        </div>
        <div className="crop-toolbar" aria-label="Масштаб фото">
          <button aria-label="Уменьшить фото" disabled={crop.zoom <= 1} onClick={() => changeZoom(-0.1)} type="button">−</button>
          <span>{Math.round(crop.zoom * 100)}%</span>
          <button aria-label="Увеличить фото" disabled={crop.zoom >= 3} onClick={() => changeZoom(0.1)} type="button">＋</button>
          <button className="crop-reset" onClick={() => commitCrop({ x: 50, y: 50, zoom: 1 })} type="button">Сбросить</button>
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
        <div className="crop-actions">
          <button className="quiet-button" disabled={saving} onClick={onCancel} type="button">Отмена</button>
          <button className="primary-button" disabled={saving} onClick={() => void confirm()} type="button">
            {saving ? "Сжимаем фото…" : "Использовать этот кадр"}
          </button>
        </div>
      </section>
    </div>
  );
}

function ProfileDialog({
  user,
  onClose,
  onSaved,
}: {
  user: User;
  onClose: () => void;
  onSaved: (user: User) => void;
}) {
  const [displayName, setDisplayName] = useState(user.display_name);
  const [nickname, setNickname] = useState(user.nickname);
  const [pendingAvatar, setPendingAvatar] = useState<File | null>(null);
  const [avatarToCrop, setAvatarToCrop] = useState<File | null>(null);
  const [removeAvatar, setRemoveAvatar] = useState(false);
  const [avatarPreviewURL, setAvatarPreviewURL] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const avatarInputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    if (!pendingAvatar) {
      setAvatarPreviewURL("");
      return;
    }
    const objectURL = URL.createObjectURL(pendingAvatar);
    setAvatarPreviewURL(objectURL);
    return () => URL.revokeObjectURL(objectURL);
  }, [pendingAvatar]);

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !saving && !avatarToCrop) onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [avatarToCrop, onClose, saving]);

  function chooseAvatar(file: File | undefined) {
    if (!file) return;
    const validationError = validatePhotoSource(file);
    if (validationError) {
      setError(validationError);
      return;
    }
    setError("");
    setAvatarToCrop(file);
  }

  async function save(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSaving(true);
    setError("");
    try {
      let updated = await api.updateProfile({
        display_name: displayName,
        nickname,
        avatar_url: removeAvatar || pendingAvatar ? null : user.avatar_url,
      });
      if (pendingAvatar) {
        const mutation = await api.putUserAvatar(pendingAvatar);
        updated = {
          ...updated,
          avatar_url: null,
          avatar_revision: mutation.avatar_revision,
        };
      } else if (removeAvatar) {
        await api.deleteUserAvatar();
        updated = { ...updated, avatar_url: null, avatar_revision: null };
      }
      onSaved(updated);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setSaving(false);
    }
  }

  const hasAvatar = Boolean(
    pendingAvatar || (!removeAvatar && (user.avatar_revision || user.avatar_url)),
  );
  const previewUser = {
    ...user,
    display_name: displayName || user.display_name,
    avatar_url: removeAvatar ? null : user.avatar_url,
    avatar_revision: removeAvatar || pendingAvatar ? null : user.avatar_revision,
  };

  return (
    <>
      <div
        className="dialog-backdrop"
        role="presentation"
        onMouseDown={() => {
          if (!saving) onClose();
        }}
      >
        <section
          className="create-dialog profile-dialog"
          role="dialog"
          aria-modal="true"
          aria-labelledby="profile-title"
          onMouseDown={(event) => event.stopPropagation()}
        >
          <button
            className="dialog-close"
            type="button"
            onClick={onClose}
            disabled={saving}
            aria-label="Закрыть"
          >
            ×
          </button>
          <p className="section-kicker">ПРОФИЛЬ</p>
          <div className="profile-dialog-heading">
            <Avatar user={previewUser} large previewURL={avatarPreviewURL} />
            <div>
              <h2 id="profile-title">Как вас видит группа</h2>
              <p className="muted">Имя и фото показываются участникам ваших встреч.</p>
            </div>
          </div>
          <form className="create-form" onSubmit={save}>
            <div className="profile-avatar-control">
              <div>
                <strong>Фото профиля</strong>
                <small>Квадратный кадр, до 512 × 512 пикселей после сжатия</small>
              </div>
              <div className="profile-avatar-actions">
                <input
                  ref={avatarInputRef}
                  className="sr-only"
                  accept="image/jpeg,image/png"
                  disabled={saving}
                  onChange={(event) => {
                    chooseAvatar(event.currentTarget.files?.[0]);
                    event.currentTarget.value = "";
                  }}
                  type="file"
                />
                <button
                  className="secondary-button"
                  disabled={saving}
                  onClick={() => avatarInputRef.current?.click()}
                  type="button"
                >
                  {hasAvatar ? "Заменить фото" : "Выбрать фото"}
                </button>
                {hasAvatar && (
                  <button
                    className="quiet-button"
                    disabled={saving}
                    onClick={() => {
                      setPendingAvatar(null);
                      setRemoveAvatar(true);
                    }}
                    type="button"
                  >
                    Удалить
                  </button>
                )}
              </div>
            </div>
          <label className="field">
            <span>Отображаемое имя</span>
            <input
              name="display_name"
              autoComplete="name"
              minLength={1}
              maxLength={80}
              required
              value={displayName}
              onChange={(event) => setDisplayName(event.target.value)}
              autoFocus
            />
          </label>
          <label className="field">
            <span>
              Никнейм
              <small>По нему вас смогут найти друзья</small>
            </span>
            <input
              name="nickname"
              autoComplete="username"
              minLength={3}
              maxLength={24}
              pattern="[A-Za-z][A-Za-z0-9_]{2,23}"
              required
              value={nickname}
              onChange={(event) => setNickname(event.target.value.toLowerCase())}
            />
          </label>
          <div className="profile-email">
            <span>Аккаунт</span>
            <strong>{user.email}</strong>
            <small>Почту пока нельзя изменить.</small>
          </div>
          {error && <p className="form-error" role="alert">{error}</p>}
          <button className="primary-button" disabled={saving} type="submit">
            {saving ? "Сохраняем…" : "Сохранить профиль"}
            <span aria-hidden="true">→</span>
          </button>
          </form>
        </section>
      </div>
      {avatarToCrop && (
        <PhotoCropDialog
          file={avatarToCrop}
          label="аватара"
          variant="avatar"
          onCancel={() => setAvatarToCrop(null)}
          onConfirm={(file) => {
            setPendingAvatar(file);
            setRemoveAvatar(false);
            setAvatarToCrop(null);
          }}
        />
      )}
    </>
  );
}

function FriendAvatar({ item }: { item: FriendItem | FriendSearchItem }) {
  const userID = "user_id" in item ? item.user_id : item.id;
  return (
    <span className="friend-avatar" aria-hidden="true">
      <span>{item.display_name.slice(0, 1).toUpperCase()}</span>
      <StoredAvatarImage
        userID={userID}
        revision={item.avatar_revision}
        legacyURL={item.avatar_url}
      />
    </span>
  );
}

function FriendsDialog({ onClose }: { onClose: () => void }) {
  const [overview, setOverview] = useState<FriendsOverview | null>(null);
  const [query, setQuery] = useState("");
  const [results, setResults] = useState<FriendSearchItem[]>([]);
  const [searched, setSearched] = useState(false);
  const [loading, setLoading] = useState(true);
  const [workingID, setWorkingID] = useState("");
  const [error, setError] = useState("");

  const loadOverview = useCallback(async () => {
    setError("");
    try {
      setOverview(await api.getFriends());
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadOverview();
  }, [loadOverview]);

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !workingID) onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose, workingID]);

  async function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const normalized = query.trim().toLowerCase();
    if (normalized.length < 3) {
      setError("Введите не меньше трёх символов никнейма.");
      return;
    }
    setLoading(true);
    setError("");
    try {
      const response = await api.searchUsers(normalized);
      setResults(response.items);
      setSearched(true);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setLoading(false);
    }
  }

  async function refreshSearch() {
    if (!searched || query.trim().length < 3) return;
    const response = await api.searchUsers(query.trim().toLowerCase());
    setResults(response.items);
  }

  async function act(id: string, action: () => Promise<unknown>) {
    setWorkingID(id);
    setError("");
    try {
      await action();
      await Promise.all([loadOverview(), refreshSearch()]);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setWorkingID("");
    }
  }

  function renderPerson(item: FriendItem, action: "incoming" | "outgoing" | "friend") {
    return (
      <li className="friend-row" key={`${action}-${item.user_id}`}>
        <FriendAvatar item={item} />
        <div className="friend-identity">
          <strong>{item.display_name}</strong>
          <span>@{item.nickname}</span>
        </div>
        <div className="friend-row-actions">
          {action === "incoming" && (
            <>
              <button className="small-button primary-small" disabled={Boolean(workingID)} onClick={() => void act(item.request_id, () => api.acceptFriendRequest(item.request_id))} type="button">Принять</button>
              <button className="small-button" disabled={Boolean(workingID)} onClick={() => void act(item.request_id, () => api.deleteFriendRequest(item.request_id))} type="button">Отклонить</button>
            </>
          )}
          {action === "outgoing" && <button className="small-button" disabled={Boolean(workingID)} onClick={() => void act(item.request_id, () => api.deleteFriendRequest(item.request_id))} type="button">Отменить</button>}
          {action === "friend" && <button className="small-button" disabled={Boolean(workingID)} onClick={() => void act(item.user_id, () => api.removeFriend(item.user_id))} type="button">Удалить</button>}
        </div>
      </li>
    );
  }

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={() => { if (!workingID) onClose(); }}>
      <section className="create-dialog friends-dialog" role="dialog" aria-modal="true" aria-labelledby="friends-title" onMouseDown={(event) => event.stopPropagation()}>
        <button className="dialog-close" type="button" onClick={onClose} disabled={Boolean(workingID)} aria-label="Закрыть">×</button>
        <p className="section-kicker">ВАШ КРУГ</p>
        <h2 id="friends-title">Друзья</h2>
        <p className="muted friends-intro">Найдите знакомого по точному началу никнейма. Дружба не открывает ваши встречи — доступ по-прежнему выдаётся только по приглашению.</p>

        <form className="friends-search" onSubmit={search}>
          <label className="field">
            <span className="sr-only">Никнейм</span>
            <div className="friends-search-control">
              <span aria-hidden="true">@</span>
              <input value={query} onChange={(event) => setQuery(event.target.value.toLowerCase())} minLength={3} maxLength={24} placeholder="nickname" autoComplete="off" required />
            </div>
          </label>
          <button className="primary-button compact" disabled={loading || Boolean(workingID)} type="submit">Найти</button>
        </form>
        {error && <p className="form-error" role="alert">{error}</p>}

        {searched && (
          <section className="friend-group" aria-labelledby="friend-search-results">
            <div className="friend-group-heading"><h3 id="friend-search-results">Результаты</h3><span>{results.length}</span></div>
            {results.length === 0 ? <p className="friend-empty">Никого не нашли. Проверьте написание никнейма.</p> : (
              <ul className="friend-list">
                {results.map((item) => (
                  <li className="friend-row" key={item.id}>
                    <FriendAvatar item={item} />
                    <div className="friend-identity"><strong>{item.display_name}</strong><span>@{item.nickname}</span></div>
                    <div className="friend-row-actions">
                      {item.relationship === "none" && <button className="small-button primary-small" disabled={Boolean(workingID)} onClick={() => void act(item.id, () => api.sendFriendRequest(item.id))} type="button">Добавить</button>}
                      {item.relationship === "outgoing" && <span className="friend-status">Заявка отправлена</span>}
                      {item.relationship === "incoming" && item.request_id && <button className="small-button primary-small" disabled={Boolean(workingID)} onClick={() => void act(item.request_id as string, () => api.acceptFriendRequest(item.request_id as string))} type="button">Принять</button>}
                      {item.relationship === "friend" && <span className="friend-status friend-status-ready">Уже в друзьях</span>}
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </section>
        )}

        {loading && !overview ? <div className="friend-loading"><span className="loading-spinner" aria-hidden="true" /> Загружаем список…</div> : overview && (
          <div className="friends-columns">
            <div>
              <section className="friend-group">
                <div className="friend-group-heading"><h3>Входящие заявки</h3><span>{overview.incoming.total}</span></div>
                {overview.incoming.items.length ? <ul className="friend-list">{overview.incoming.items.map((item) => renderPerson(item, "incoming"))}</ul> : <p className="friend-empty">Новых заявок нет.</p>}
              </section>
              <section className="friend-group">
                <div className="friend-group-heading"><h3>Отправленные</h3><span>{overview.outgoing.total}</span></div>
                {overview.outgoing.items.length ? <ul className="friend-list">{overview.outgoing.items.map((item) => renderPerson(item, "outgoing"))}</ul> : <p className="friend-empty">Нет заявок в ожидании.</p>}
              </section>
            </div>
            <section className="friend-group friends-list-group">
              <div className="friend-group-heading"><h3>Ваши друзья</h3><span>{overview.friends.total}</span></div>
              {overview.friends.items.length ? <ul className="friend-list">{overview.friends.items.map((item) => renderPerson(item, "friend"))}</ul> : <p className="friend-empty">Пока никого нет. Найдите друга по никнейму выше.</p>}
            </section>
          </div>
        )}
      </section>
    </div>
  );
}

function MeetingDetailView({
  meetingID,
  initialInviteLink,
  initialNotice,
  archived,
  user,
  theme,
  onThemeChange,
  onFriends,
  incomingInviteCount,
  onMeetingInvites,
  onEditProfile,
  onBack,
  onMeetingUpdated,
  onLogout,
}: {
  meetingID: string;
  initialInviteLink: string;
  initialNotice: string;
  archived: boolean;
  user: User;
  theme: Theme;
  onThemeChange: () => void;
  onFriends: () => void;
  incomingInviteCount: number;
  onMeetingInvites: () => void;
  onEditProfile: () => void;
  onBack: () => void;
  onMeetingUpdated: (meeting: Meeting) => void;
  onLogout: () => Promise<void>;
}) {
  const [meeting, setMeeting] = useState<MeetingDetail | null>(null);
  const [polls, setPolls] = useState<Poll[]>([]);
  const [availability, setAvailability] = useState<AvailabilityView | null>(null);
  const [attendance, setAttendance] = useState<AttendanceView | null>(null);
  const [notes, setNotes] = useState<MeetingNotePage | null>(null);
  const [planVotes, setPlanVotes] = useState<PlanVotePage | null>(null);
  const [preparation, setPreparation] = useState<RequirementPage | null>(null);
  const [loading, setLoading] = useState(true);
  const [working, setWorking] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState(initialNotice);
  const [inviteLink, setInviteLink] = useState(initialInviteLink);
  const [showShare, setShowShare] = useState(false);
  const [photoCropTarget, setPhotoCropTarget] = useState<{
    file: File;
    scope: "meeting" | "plan";
    optionID?: string;
  } | null>(null);
  const [confirmingCancellation, setConfirmingCancellation] = useState(false);
  const [liveState, setLiveState] = useState<LiveState>("idle");
  const [activeSection, setActiveSection] = useState<MeetingSection | null>(null);
  const [showPollComposer, setShowPollComposer] = useState(false);
  const [showPlanForm, setShowPlanForm] = useState(false);
  const [showTimeForm, setShowTimeForm] = useState(false);
  const [showAllPlans, setShowAllPlans] = useState(false);
  const [showAllTimes, setShowAllTimes] = useState(false);
  const [showAllParticipants, setShowAllParticipants] = useState(false);
  const [editingPlanID, setEditingPlanID] = useState<string | null>(null);
  const [editingTimeID, setEditingTimeID] = useState<string | null>(null);
  const meetingVersionRef = useRef(0);
  const liveReloadingRef = useRef(false);
  const queuedLiveVersionRef = useRef(0);

  useEffect(() => {
    if (initialInviteLink) setInviteLink(initialInviteLink);
  }, [initialInviteLink]);

  useEffect(() => {
    if (initialNotice) setMessage(initialNotice);
  }, [initialNotice]);

  const load = useCallback(async (background = false): Promise<boolean> => {
    if (!background) {
      setLoading(true);
      setError("");
    }
    try {
      const meetingResult = await api.getMeeting(meetingID);
      const [
        pollResult,
        availabilityResult,
        planVoteResult,
        preparationResult,
        attendanceResult,
        noteResult,
      ] = await Promise.all([
        api.listPolls(meetingID),
        meetingResult.coordination_mode !== "fixed"
          ? api.getAvailability(meetingID)
          : Promise.resolve(null),
        meetingResult.coordination_mode !== "fixed"
          ? api.getPlanVotes(meetingID)
          : Promise.resolve(null),
        api.listRequirements(meetingID),
        meetingResult.coordination_mode === "fixed"
          ? api.getAttendance(meetingID)
          : Promise.resolve(null),
        api.listMeetingNotes(meetingID),
      ]);
      meetingVersionRef.current = meetingResult.version;
      setMeeting(meetingResult);
      setPolls(pollResult.items);
      setAvailability(availabilityResult);
      setAttendance(attendanceResult);
      setNotes(noteResult);
      setPlanVotes(planVoteResult);
      setPreparation(preparationResult);
      onMeetingUpdated(meetingResult);
      return true;
    } catch (requestError) {
      if (!background) setError(errorMessage(requestError));
      return false;
    } finally {
      if (!background) setLoading(false);
    }
  }, [meetingID, onMeetingUpdated]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    setActiveSection(null);
    setShowPollComposer(false);
    setShowPlanForm(false);
    setShowTimeForm(false);
    setShowAllPlans(false);
    setShowAllTimes(false);
    setShowAllParticipants(false);
    setEditingPlanID(null);
    setEditingTimeID(null);
  }, [meetingID]);

  const reloadFromLive = useCallback(async (version: number) => {
    if (version <= meetingVersionRef.current) {
      return;
    }
    queuedLiveVersionRef.current = Math.max(queuedLiveVersionRef.current, version);
    if (liveReloadingRef.current) {
      return;
    }
    liveReloadingRef.current = true;
    try {
      while (queuedLiveVersionRef.current > meetingVersionRef.current) {
        queuedLiveVersionRef.current = 0;
        if (!(await load(true))) {
          return;
        }
      }
    } finally {
      liveReloadingRef.current = false;
    }
  }, [load]);

  const liveEnabled = meeting !== null
    && meeting.state !== "cancelled"
    && meeting.state !== "completed";

  useEffect(() => {
    if (!liveEnabled) {
      setLiveState("idle");
      return;
    }

    let cancelled = false;
    let controller: AbortController | null = null;
    let retryTimer = 0;

    async function connect() {
      let attempt = 0;
      while (!cancelled) {
        controller = new AbortController();
        setLiveState(attempt === 0 ? "connecting" : "reconnecting");
        try {
          await api.watchMeeting(meetingID, controller.signal, (event) => {
            if (cancelled) {
              return;
            }
            setLiveState("live");
            attempt = 0;
            if (event.type === "meeting.updated") {
              void reloadFromLive(event.version);
            }
          });
        } catch {
          if (controller.signal.aborted || cancelled) {
            return;
          }
        }
        if (cancelled) {
          return;
        }
        attempt = Math.min(attempt + 1, 4);
        setLiveState("reconnecting");
        await new Promise<void>((resolve) => {
          retryTimer = window.setTimeout(resolve, Math.min(1000 * 2 ** (attempt - 1), 10_000));
        });
      }
    }

    void connect();
    return () => {
      cancelled = true;
      controller?.abort();
      window.clearTimeout(retryTimer);
    };
  }, [liveEnabled, meetingID, reloadFromLive]);

  async function mutateAndReport(action: () => Promise<unknown>, success: string): Promise<boolean> {
    setWorking(true);
    setError("");
    setMessage("");
    try {
      await action();
      await load();
      setMessage(success);
      return true;
    } catch (requestError) {
      setError(errorMessage(requestError));
      return false;
    } finally {
      setWorking(false);
    }
  }

  async function mutate(action: () => Promise<unknown>, success: string): Promise<void> {
    await mutateAndReport(action, success);
  }

  async function downloadCalendar() {
    setWorking(true);
    setError("");
    setMessage("");
    try {
      const file = await api.exportMeetingCalendar(meetingID);
      const url = URL.createObjectURL(file);
      const link = document.createElement("a");
      link.href = url;
      link.download = `ryden-${meetingID}.ics`;
      document.body.appendChild(link);
      link.click();
      link.remove();
      URL.revokeObjectURL(url);
      setMessage("Файл календаря скачан. Откройте его, чтобы добавить встречу.");
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setWorking(false);
    }
  }

  function selectPhotoForCrop(
    file: File,
    scope: "meeting" | "plan",
    optionID?: string,
  ) {
    const photoError = validatePhotoSource(file);
    if (photoError) {
      setError(photoError);
      return;
    }
    setError("");
    setPhotoCropTarget({ file, scope, optionID });
  }

  function saveCroppedPhoto(file: File) {
    if (!meeting) return;
    const target = photoCropTarget;
    setPhotoCropTarget(null);
    if (!target) return;
    if (target.scope === "meeting") {
      void mutate(
        () => api.putMeetingPhoto(meetingID, meeting.version, file),
        "Фото встречи сохранено.",
      );
    } else if (target.optionID) {
      void mutate(
        () => api.putPlanOptionPhoto(meetingID, target.optionID as string, meeting.version, file),
        "Фото варианта сохранено.",
      );
    }
  }

  async function addPlan(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await mutate(
      () => api.addPlanOption(meetingID, {
        title: String(data.get("title") ?? ""),
        description: String(data.get("description") ?? ""),
      }),
      "Вариант плана добавлен.",
    );
    form.reset();
  }

  async function updateMeeting(input: UpdateMeetingInput) {
    await mutate(
      () => api.updateMeeting(meetingID, input),
      "Основные данные встречи сохранены.",
    );
  }

  async function addTime(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!meeting) return;
    const form = event.currentTarget;
    const data = new FormData(form);
    let startsAt: string;
    let endsAt: string | null;
    try {
      ({ startsAt, endsAt } = readTimeSelection(data, meeting.timezone));
    } catch (dateError) {
      setError(dateError instanceof Error ? dateError.message : "Проверьте дату и время.");
      return;
    }
    const planOptionID = String(data.get("plan_option_id") ?? "");
    if (await mutateAndReport(
      () => api.addTimeOption(meetingID, {
        plan_option_id: planOptionID || null,
        starts_at: startsAt,
        ends_at: endsAt,
      }),
      "Вариант времени добавлен.",
    )) {
      form.reset();
    }
  }

  async function deletePlanOption(optionID: string) {
    const linkedTimes = meeting?.time_options.filter((item) => item.plan_option_id === optionID).length ?? 0;
    if (linkedTimes > 0 && !window.confirm(
      `К этому плану привязано вариантов времени: ${linkedTimes}. Удалить план вместе с ними?`,
    )) {
      return;
    }
    await mutate(() => api.deletePlanOption(meetingID, optionID), "Вариант удалён.");
  }

  async function updatePlanOption(optionID: string, input: UpdatePlanOptionInput) {
    if (await mutateAndReport(
      () => api.updatePlanOption(meetingID, optionID, input),
      "Вариант плана обновлён.",
    )) {
      setEditingPlanID(null);
    }
  }

  async function updateTimeOption(optionID: string, input: UpdateTimeOptionInput) {
    if (await mutateAndReport(
      () => api.updateTimeOption(meetingID, optionID, input),
      "Вариант времени обновлён.",
    )) {
      setEditingTimeID(null);
    }
  }

  async function createPoll(input: CreatePollInput): Promise<boolean> {
    return mutateAndReport(
      () => api.createPoll(meetingID, input),
      "Опрос добавлен.",
    );
  }

  async function createRequirement(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const form = event.currentTarget;
    const data = new FormData(form);
    await mutate(
      () => api.createRequirement(meetingID, {
        name: String(data.get("name") ?? ""),
        required_quantity: Number(data.get("required_quantity") ?? 0),
      }),
      "Позиция подготовки добавлена.",
    );
    form.reset();
  }

  async function createInvitation(): Promise<string | null> {
    const secret = newInvitationSecret();
    setWorking(true);
    setError("");
    setMessage("");
    try {
      await api.createInvitation(meetingID, secret);
      const link = `${window.location.origin}/#/invite/${secret}`;
      setInviteLink(link);
      await load();
      setMessage("Ссылка готова. Отправьте её участникам.");
      return link;
    } catch (requestError) {
      setError(errorMessage(requestError));
      return null;
    } finally {
      setWorking(false);
    }
  }

  async function copyInvitation() {
    try {
      await navigator.clipboard.writeText(inviteLink);
      setMessage("Ссылка скопирована.");
    } catch {
      setMessage("Выделите ссылку и скопируйте её вручную.");
    }
  }

  async function openShare() {
    if (inviteLink) {
      setShowShare(true);
      return;
    }
    if (!meeting?.active_invitation_expires_at && canInvite) {
      const link = await createInvitation();
      if (link) setShowShare(true);
      return;
    }
    setShowShare(true);
  }

  async function shareMeetingSummary() {
    if (!meeting) return;
    const sharedTime = meeting.time_options.find((item) => item.id === meeting.selected_time_option_id)
      ?? (meeting.coordination_mode === "fixed" ? meeting.time_options[0] : undefined);
    const when = sharedTime
      ? createMeetingDateTimeFormatter(meeting.timezone).format(new Date(sharedTime.starts_at))
      : "дата уточняется";
    const summary = `${meeting.title} — ${when}${meeting.location_name ? `, ${meeting.location_name}` : ""}`;
    try {
      if (typeof navigator.share === "function") {
        await navigator.share({ title: meeting.title, text: summary });
        return;
      }
      await navigator.clipboard.writeText(summary);
      setMessage("Информация о встрече скопирована.");
    } catch (shareError) {
      if (shareError instanceof DOMException && shareError.name === "AbortError") return;
      setError("Не удалось поделиться встречей.");
    }
  }

  if (loading && !meeting) {
    return <LoadingScreen theme={theme} onThemeChange={onThemeChange} />;
  }

  if (!meeting) {
    return (
      <div className="app-shell">
        <AppHeader
          user={user}
          theme={theme}
          onThemeChange={onThemeChange}
          onFriends={onFriends}
          incomingInviteCount={incomingInviteCount}
          onMeetingInvites={onMeetingInvites}
          onEditProfile={onEditProfile}
          onLogout={onLogout}
        />
        <main className="meeting-detail">
          <button className="back-button" type="button" onClick={onBack}>← Все встречи</button>
          <div className="notice error-notice" role="alert">{error || "Встреча не найдена."}</div>
        </main>
      </div>
    );
  }

  const canEdit = meeting.participant_role === "owner" && meeting.state === "draft";
  const canInvite = canEdit && (
    meeting.coordination_mode !== "fixed"
      ? meeting.plan_options.length >= 2 && meeting.time_options.length >= 2
      : meeting.plan_options.length === 1 && meeting.time_options.length === 1
  );
  const isOwner = meeting.participant_role === "owner";
  const dateFormatter = createMeetingDateTimeFormatter(meeting.timezone);
  const selectedTime = meeting.time_options.find(
    (option) => option.id === meeting.selected_time_option_id,
  );
  const heroTime = selectedTime ?? (
    meeting.coordination_mode === "fixed" ? meeting.time_options[0] : undefined
  );
  const readOnly = archived || meeting.state === "cancelled" || meeting.state === "completed"
    || Boolean(heroTime && new Date(heroTime.ends_at ?? heroTime.starts_at).getTime() + 24 * 60 * 60 * 1000 <= Date.now());
  const canEditMeetingDetails = isOwner && !readOnly && (
    meeting.state === "draft"
    || (meeting.coordination_mode === "fixed" && meeting.state === "scheduled")
  );
  const canCreatePoll = !readOnly && (
    meeting.state === "draft" || meeting.state === "collecting" || meeting.state === "scheduled"
  );
  const heroDateFormatter = new Intl.DateTimeFormat("ru-RU", {
    day: "2-digit",
    month: "2-digit",
    year: "numeric",
    timeZone: meeting.timezone,
  });
  const heroClockFormatter = new Intl.DateTimeFormat("ru-RU", {
    hour: "2-digit",
    minute: "2-digit",
    timeZone: meeting.timezone,
  });
  const visiblePlanOptions = showAllPlans
    ? meeting.plan_options
    : meeting.plan_options.slice(0, OPTION_PREVIEW_LIMIT);
  const visibleTimeOptions = showAllTimes
    ? meeting.time_options
    : meeting.time_options.slice(0, OPTION_PREVIEW_LIMIT);
  const visibleParticipants = showAllParticipants
    ? meeting.participants
    : meeting.participants.slice(0, PARTICIPANT_PREVIEW_LIMIT);
  const showsVotes = meeting.coordination_mode !== "fixed"
    && meeting.state !== "draft" && planVotes && meeting.plan_options.length > 0;
  const showsAvailability = (
    meeting.coordination_mode !== "fixed" &&
    (meeting.state === "collecting" || meeting.state === "cancelled")
  ) && availability && meeting.time_options.length > 0;
  const showsAttendance = meeting.coordination_mode === "fixed"
    && meeting.state !== "draft"
    && attendance !== null;
  const showsPreparation = (
    meeting.state === "scheduled"
    || meeting.state === "completed"
    || (meeting.state === "cancelled" && preparation && preparation.total > 0)
  ) && preparation;
  const preparationPercent = preparation?.total
    ? Math.round((preparation.completed_count / preparation.total) * 100)
    : 0;
  const planAnsweredPercent = planVotes?.participant_count
    ? Math.round((planVotes.answered_count / planVotes.participant_count) * 100)
    : 0;

  return (
    <div className="app-shell">
      <AppHeader
        user={user}
        theme={theme}
        onThemeChange={onThemeChange}
        onFriends={onFriends}
        incomingInviteCount={incomingInviteCount}
        onMeetingInvites={onMeetingInvites}
        onEditProfile={onEditProfile}
        onLogout={onLogout}
      />
      <main className="meeting-detail">
        <button className="back-button" type="button" onClick={onBack}><span aria-hidden="true">←</span> Все встречи</button>
        {liveEnabled && (
          <div className={`meeting-live meeting-live-${liveState}`} role="status" aria-live="polite">
            <i aria-hidden="true" />
            <span>
              {liveState === "live"
                ? "Данные обновляются сами"
                : liveState === "reconnecting"
                  ? "Восстанавливаем связь…"
                  : "Проверяем обновления…"}
            </span>
          </div>
        )}
        <section className="meeting-hero">
          {isOwner && !readOnly && meeting.state !== "cancelled" && meeting.state !== "completed" && (
            <button
              aria-label="Управление встречей"
              className="meeting-settings-corner"
              onClick={() => setActiveSection("manage")}
              title="Настройки встречи"
              type="button"
            >
              <svg aria-hidden="true" viewBox="0 0 24 24">
                <path d="M12 8.5a3.5 3.5 0 1 0 0 7 3.5 3.5 0 0 0 0-7Z" />
                <path d="m19 13.8 1.2 1.1-2 3.4-1.6-.5a7.8 7.8 0 0 1-2.2 1.3L14 21h-4l-.4-1.9a7.8 7.8 0 0 1-2.2-1.3l-1.6.5-2-3.4L5 13.8a8 8 0 0 1 0-2.6L3.8 10l2-3.4 1.6.5a7.8 7.8 0 0 1 2.2-1.3L10 4h4l.4 1.8a7.8 7.8 0 0 1 2.2 1.3l1.6-.5 2 3.4-1.2 1.2a8 8 0 0 1 0 2.6Z" />
              </svg>
            </button>
          )}
          <div className="meeting-hero-copy">
            <span className={`status status-${meeting.state}`}><i aria-hidden="true" />{stateLabels[meeting.state]}</span>
            <h1>{meeting.title}</h1>
            <p>{meeting.description || "Описание пока не добавлено."}</p>
          </div>
          {heroTime && (
            <div className="meeting-hero-schedule" aria-label="Дата и время встречи">
              <strong>{heroDateFormatter.format(new Date(heroTime.starts_at))}</strong>
              <span>
                {heroClockFormatter.format(new Date(heroTime.starts_at))}
                {heroTime.ends_at && ` / ${formatDurationShort(heroTime.starts_at, heroTime.ends_at)}`}
              </span>
              <small>{meetingTimeZoneLabel(meeting.timezone)}</small>
            </div>
          )}
          <div className="meeting-side">
            {(meeting.has_photo || meeting.cover_url) && (
              <MeetingCover
                meetingID={meetingID}
                hasStoredPhoto={Boolean(meeting.has_photo)}
                source={meeting.cover_url}
                title={meeting.title}
                revision={meeting.version}
                attendanceStatus={showsAttendance && attendance ? attendance.my_status : undefined}
              />
            )}
            <div className={`meeting-hero-actions${showsAttendance ? " meeting-hero-actions-fixed" : ""}`}>
              {((isOwner && !readOnly && meeting.state !== "cancelled" && meeting.state !== "completed")
                || (meeting.state === "scheduled" || meeting.state === "completed")) && (
                <button
                  aria-label="Поделиться встречей"
                  className="meeting-action-button share-meeting-button"
                  disabled={working || (meeting.state === "draft" && !canInvite)}
                  onClick={() => void (isOwner && !readOnly ? openShare() : shareMeetingSummary())}
                  type="button"
                >
                  <svg aria-hidden="true" viewBox="0 0 24 24">
                    <circle cx="18" cy="5" r="2.5" />
                    <circle cx="6" cy="12" r="2.5" />
                    <circle cx="18" cy="19" r="2.5" />
                    <path d="m8.2 10.8 7.6-4.5M8.2 13.2l7.6 4.5" />
                  </svg>
                </button>
              )}
              {(meeting.state === "scheduled" || meeting.state === "completed") && (
                <button
                  aria-label="Добавить в календарь"
                  className="meeting-action-button calendar-export-button"
                  disabled={working}
                  onClick={() => void downloadCalendar()}
                  type="button"
                >
                  <svg aria-hidden="true" viewBox="0 0 24 24">
                    <path d="M6 3v3M18 3v3M4 9h16M5 5h14a1 1 0 0 1 1 1v14H4V6a1 1 0 0 1 1-1Z" />
                    <path d="M12 12v5m0 0-2-2m2 2 2-2" />
                  </svg>
                </button>
              )}
              {showsAttendance && attendance && (
                <button
                  aria-label={`Участие: ${attendanceLabels[attendance.my_status]}. Открыть и изменить ответ`}
                  className={`meeting-action-button attendance-action-button attendance-${attendance.my_status}`}
                  onClick={() => setActiveSection("attendance")}
                  title={`Участие — ${attendanceLabels[attendance.my_status]}`}
                  type="button"
                >
                  <svg aria-hidden="true" viewBox="0 0 24 24">
                    <circle cx="9" cy="8" r="3" />
                    <path d="M3.5 19c.6-3.5 2.4-5.3 5.5-5.3s4.9 1.8 5.5 5.3M16 7.5a2.5 2.5 0 0 1 0 4.8M16 14c2.5.3 3.9 2 4.3 5" />
                  </svg>
                  <span aria-hidden="true">{attendanceMarks[attendance.my_status]}</span>
                </button>
              )}
            </div>
          </div>
          {(meeting.location_name || meeting.location_url) && (
            <div className="meeting-hero-location">
              <span className="meeting-location-icon" aria-hidden="true">⌖</span>
              <div>
                <small>Место встречи</small>
                {meeting.location_name && meeting.location_url ? (
                  <a href={meeting.location_url} target="_blank" rel="noreferrer noopener">
                    {meeting.location_name}
                  </a>
                ) : meeting.location_name ? (
                  <strong>{meeting.location_name}</strong>
                ) : (
                  <a href={meeting.location_url ?? ""} target="_blank" rel="noreferrer noopener">
                    {meeting.location_url}
                  </a>
                )}
              </div>
            </div>
          )}
        </section>

        <section className="meeting-summary-grid" aria-label="Краткая информация о встрече">
          {meeting.coordination_mode !== "fixed" && (
            <button className="meeting-summary-card summary-plan" onClick={() => setActiveSection("overview")} type="button">
              <span className="summary-card-heading"><small>Варианты</small><b aria-hidden="true">→</b></span>
              <strong>{countLabel(meeting.plan_options.length, "вариант плана", "варианта плана", "вариантов плана")} · {countLabel(meeting.time_options.length, "вариант времени", "варианта времени", "вариантов времени")}</strong>
              <p>Посмотреть и сравнить варианты</p>
            </button>
          )}
          {showsAttendance && attendance && (
            <button className="meeting-summary-card attendance-card summary-attendance" onClick={() => setActiveSection("attendance")} type="button">
              <span className="summary-card-heading"><small>Участие</small><b aria-hidden="true">→</b></span>
              <strong>Идут: {attendance.going_count} · думают: {attendance.maybe_count}</strong>
              <div className="summary-bar" aria-label={`Идут ${attendance.going_count}, думают ${attendance.maybe_count}, не идут ${attendance.not_going_count}, без ответа ${attendance.unanswered_count}`}>
                <i className="bar-going" style={{ flexGrow: attendance.going_count }} />
                <i className="bar-maybe" style={{ flexGrow: attendance.maybe_count }} />
                <i className="bar-not-going" style={{ flexGrow: attendance.not_going_count }} />
                <i className="bar-unanswered" style={{ flexGrow: attendance.unanswered_count }} />
              </div>
              <p>Ваш ответ: {attendanceLabels[attendance.my_status]}</p>
            </button>
          )}
          {showsVotes && (
            <button className="meeting-summary-card summary-votes" onClick={() => setActiveSection("votes")} type="button">
              <span className="summary-card-heading"><small>Выбор плана</small><b aria-hidden="true">→</b></span>
              <strong>{planVotes?.answered_count ?? 0} из {planVotes?.participant_count ?? meeting.participants.length} ответили</strong>
              <div className="summary-progress"><i style={{ width: `${planAnsweredPercent}%` }} /></div>
              <p>{planAnsweredPercent}% ответов собрано</p>
            </button>
          )}
          {showsAvailability && (
            <button className="meeting-summary-card summary-time" onClick={() => setActiveSection("availability")} type="button">
              <span className="summary-card-heading"><small>Время</small><b aria-hidden="true">→</b></span>
              <strong>{countLabel(meeting.time_options.length, "вариант", "варианта", "вариантов")}</strong>
              <p>{availability?.recommendations[0] ? "Есть рекомендация Ryden" : "Ждём больше отметок"}</p>
            </button>
          )}
          <button className="meeting-summary-card summary-polls" onClick={() => setActiveSection("polls")} type="button">
              <span className="summary-card-heading"><small>Опросы</small><b aria-hidden="true">→</b></span>
              <strong>{countLabel(polls.length, "опрос", "опроса", "опросов")}</strong>
              <p>Открыто: {polls.filter((poll) => poll.accepting_answers).length}</p>
          </button>
          <button className="meeting-summary-card summary-notes" onClick={() => setActiveSection("notes")} type="button">
            <span className="summary-card-heading"><small>Заметки</small><b aria-hidden="true">→</b></span>
            <strong>{countLabel(notes?.total ?? 0, "заметка", "заметки", "заметок")}</strong>
            <p>{notes?.items.some((item) => item.user_id === user.id) ? "Ваша заметка сохранена" : "Добавьте важную деталь"}</p>
          </button>
          {showsPreparation && preparation && (
            <button className="meeting-summary-card summary-preparation" onClick={() => setActiveSection("preparation")} type="button">
              <span className="summary-card-heading"><small>Подготовка</small><b aria-hidden="true">→</b></span>
              <strong>{preparation.completed_count} из {preparation.total} готово</strong>
              <div className="summary-progress"><i style={{ width: `${preparationPercent}%` }} /></div>
              <p>{preparationPercent}% завершено</p>
            </button>
          )}
        </section>

        {meeting.state === "cancelled" && (
          <aside className="meeting-cancelled-note" role="status">
            <span aria-hidden="true">!</span>
            <div>
              <p className="section-kicker">ВСТРЕЧА ОТМЕНЕНА</p>
              <h2>Сбор остановлен</h2>
              <p>Ссылка приглашения закрыта. Варианты, ответы и подготовка сохранены для просмотра.</p>
            </div>
          </aside>
        )}
        {readOnly && meeting.state !== "cancelled" && meeting.state !== "completed" && (
          <aside className="meeting-cancelled-note meeting-archive-note" role="status">
            <span aria-hidden="true">✓</span>
            <div>
              <p className="section-kicker">В АРХИВЕ</p>
              <h2>Встреча завершилась</h2>
              <p>Сохранили краткий итог. Ответы и подготовка теперь доступны только для чтения.</p>
            </div>
          </aside>
        )}
        {(error || message) && (
          <div className={`notice ${error ? "error-notice" : "success-notice"}`} role={error ? "alert" : "status"}>
            {error || message}
          </div>
        )}

        {activeSection && (
          <div className="meeting-panel-backdrop" role="presentation" onMouseDown={() => setActiveSection(null)}>
            <section
              aria-label="Подробности встречи"
              aria-modal="true"
              className="meeting-panel-dialog"
              onMouseDown={(event) => event.stopPropagation()}
              role="dialog"
            >
              <button className="dialog-close" type="button" onClick={() => setActiveSection(null)} aria-label="Закрыть">×</button>
              <div className="meeting-panel-scroll">
        {activeSection === "overview" && (
          <div
            aria-labelledby="meeting-tab-overview"
            className="meeting-tab-panel"
            id="meeting-section-overview"
            role="tabpanel"
          >
            <section
              className="setup-grid"
              aria-label={meeting.coordination_mode === "fixed" ? "План и время" : "Варианты плана и времени"}
            >
          <article className="setup-panel" id="meeting-plan-options" aria-labelledby="plan-options-title">
            <div className="section-heading">
              <div>
                <p className="section-kicker">ЧТО ДЕЛАЕМ</p>
                <h2 id="plan-options-title">{meeting.coordination_mode === "fixed" ? "План встречи" : "Варианты плана"}</h2>
              </div>
              <div className="setup-heading-actions">
                <span>{meeting.plan_options.length}</span>
                {canEdit && (meeting.coordination_mode !== "fixed" || meeting.plan_options.length === 0) && (
                  <button
                    aria-expanded={showPlanForm}
                    onClick={() => {
                      setEditingPlanID(null);
                      setShowPlanForm((current) => !current);
                    }}
                    type="button"
                  >
                    {showPlanForm ? "Закрыть" : "Добавить"}
                  </button>
                )}
              </div>
            </div>
            <div className="option-list">
              {meeting.plan_options.length === 0 && (
                <p className="panel-empty">
                  {meeting.coordination_mode === "fixed" ? "Укажите, что именно уже решено." : "Пока ни одного варианта."}
                </p>
              )}
              {visiblePlanOptions.map((option, index) => (
                <div className="option-entry" key={option.id}>
                  {option.has_photo && (
                    <figure className="plan-option-photo">
                      <StoredPhoto
                        meetingID={meetingID}
                        optionID={option.id}
                        alt={`Фото варианта «${option.title}»`}
                        revision={meeting.version}
                      />
                    </figure>
                  )}
                  <div className="option-row">
                    <span className="option-number">{String(index + 1).padStart(2, "0")}</span>
                    <div><strong>{option.title}</strong>{option.description && <small>{option.description}</small>}</div>
                    {canEdit && meeting.coordination_mode !== "fixed" && (
                      <div className="option-row-actions">
                        <button
                          aria-expanded={editingPlanID === option.id}
                          type="button"
                          disabled={working}
                          onClick={() => {
                            setShowPlanForm(false);
                            setEditingPlanID((current) => current === option.id ? null : option.id);
                          }}
                        >
                          {editingPlanID === option.id ? "Закрыть" : "Изменить"}
                        </button>
                        <button type="button" disabled={working} onClick={() => void deletePlanOption(option.id)}>
                          Удалить
                        </button>
                      </div>
                    )}
                  </div>
                  {canEdit && meeting.coordination_mode !== "fixed" && editingPlanID === option.id && (
                    <>
                      <PlanOptionEditForm
                        option={option}
                        version={meeting.version}
                        working={working}
                        onCancel={() => setEditingPlanID(null)}
                        onSave={(input) => updatePlanOption(option.id, input)}
                      />
                      <PhotoControls
                        hasPhoto={Boolean(option.has_photo)}
                        label="фото места или плана"
                        working={working}
                        onDelete={() => void mutate(
                          () => api.deletePlanOptionPhoto(meetingID, option.id, meeting.version),
                          "Фото варианта удалено.",
                        )}
                        onSelect={(file) => selectPhotoForCrop(file, "plan", option.id)}
                      />
                    </>
                  )}
                </div>
              ))}
              {meeting.plan_options.length > OPTION_PREVIEW_LIMIT && (
                <button
                  aria-expanded={showAllPlans}
                  className="list-expand-button"
                  onClick={() => setShowAllPlans((current) => !current)}
                  type="button"
                >
                  {showAllPlans
                    ? "Свернуть варианты"
                    : `Показать все · ${meeting.plan_options.length}`}
                </button>
              )}
            </div>
            {canEdit && showPlanForm && (
              <form className="inline-form" onSubmit={addPlan}>
                <Field label={meeting.coordination_mode === "fixed" ? "Готовый план" : "Новый вариант"} name="title" autoComplete="off" maxLength={120} />
                <label className="field"><span>Пояснение <small>необязательно</small></span><textarea name="description" rows={2} maxLength={500} /></label>
                <button className="secondary-button" disabled={working || meeting.plan_options.length >= 20} type="submit">Добавить план</button>
              </form>
            )}
          </article>

          <article className="setup-panel" id="meeting-time-options" aria-labelledby="time-options-title">
            <div className="section-heading">
              <div>
                <p className="section-kicker">КОГДА ВСТРЕЧАЕМСЯ</p>
                <h2 id="time-options-title">{meeting.coordination_mode === "fixed" ? "Время встречи" : "Варианты времени"}</h2>
              </div>
              <div className="setup-heading-actions">
                <span>{meeting.time_options.length}</span>
                {canEdit && (meeting.coordination_mode !== "fixed" || meeting.time_options.length === 0) && (
                  <button
                    aria-expanded={showTimeForm}
                    onClick={() => {
                      setEditingTimeID(null);
                      setShowTimeForm((current) => !current);
                    }}
                    type="button"
                  >
                    {showTimeForm ? "Закрыть" : "Добавить"}
                  </button>
                )}
              </div>
            </div>
            <div className="option-list">
              {meeting.time_options.length === 0 && (
                <p className="panel-empty">
                  {meeting.coordination_mode === "fixed" ? "Укажите подтверждённые дату и время." : "Пока ни одного варианта."}
                </p>
              )}
              {visibleTimeOptions.map((option, index) => (
                <div className="option-entry" key={option.id}>
                  <div className="option-row">
                    <span className="option-number">{String(index + 1).padStart(2, "0")}</span>
                    <div>
                      <strong>{dateFormatter.format(new Date(option.starts_at))}</strong>
                      {meeting.coordination_mode !== "fixed" && <small className="time-scope-label">
                        {option.plan_option_id
                          ? `Для плана: ${meeting.plan_options.find((plan) => plan.id === option.plan_option_id)?.title ?? "удалённый план"}`
                          : "Общее для всех планов"}
                      </small>}
                      <small>
                        {option.ends_at
                          ? `до ${dateFormatter.format(new Date(option.ends_at))}`
                          : "Длительность не указана"}
                      </small>
                    </div>
                    {canEdit && (
                      <div className="option-row-actions">
                        <button
                          aria-expanded={editingTimeID === option.id}
                          type="button"
                          disabled={working}
                          onClick={() => {
                            setShowTimeForm(false);
                            setEditingTimeID((current) => current === option.id ? null : option.id);
                          }}
                        >
                          {editingTimeID === option.id ? "Закрыть" : "Изменить"}
                        </button>
                        {meeting.coordination_mode !== "fixed" && (
                          <button type="button" disabled={working} onClick={() => void mutate(
                            () => api.deleteTimeOption(meetingID, option.id), "Время удалено.",
                          )}>Удалить</button>
                        )}
                      </div>
                    )}
                  </div>
                  {canEdit && editingTimeID === option.id && (
                    <TimeOptionEditForm
                      option={option}
                      plans={meeting.plan_options}
                      timeZone={meeting.timezone}
                      version={meeting.version}
                      working={working}
                      onCancel={() => setEditingTimeID(null)}
                      onInvalid={setError}
                      onSave={(input) => updateTimeOption(option.id, input)}
                    />
                  )}
                </div>
              ))}
              {meeting.time_options.length > OPTION_PREVIEW_LIMIT && (
                <button
                  aria-expanded={showAllTimes}
                  className="list-expand-button"
                  onClick={() => setShowAllTimes((current) => !current)}
                  type="button"
                >
                  {showAllTimes
                    ? "Свернуть варианты"
                    : `Показать все · ${meeting.time_options.length}`}
                </button>
              )}
            </div>
            {canEdit && showTimeForm && (
              <form className="inline-form time-form" onSubmit={addTime}>
                <p className="time-zone-note">
                  Время встречи: <strong>{meetingTimeZoneLabel(meeting.timezone)}</strong>
                </p>
                {meeting.coordination_mode !== "fixed" && <label className="field time-scope-field">
                  <span>К какому плану относится</span>
                  <select name="plan_option_id" defaultValue="">
                    <option value="">Общее для всех планов</option>
                    {meeting.plan_options.map((plan) => (
                      <option value={plan.id} key={plan.id}>{plan.title}</option>
                    ))}
                  </select>
                </label>}
                <MeetingTimeFields timeZone={meeting.timezone} />
                <button className="secondary-button" disabled={working || meeting.time_options.length >= 20} type="submit">Добавить время</button>
              </form>
            )}
          </article>
            </section>
          </div>
        )}

        {activeSection === "polls" && (
          <section className="poll-workspace poll-workspace-dialog" aria-labelledby="polls-title">
            <div className="poll-workspace-heading">
              <div>
                <p className="section-kicker">ВОПРОСЫ ГРУППЫ</p>
                <h2 id="polls-title">Все опросы</h2>
                <p>Быстрые решения по деталям встречи — отдельно от выбора основного плана и участия.</p>
              </div>
              {canCreatePoll && (
                <button
                  aria-expanded={showPollComposer}
                  className="primary-button compact"
                  disabled={working || polls.length >= 10}
                  onClick={() => setShowPollComposer((current) => !current)}
                  type="button"
                >
                  {showPollComposer ? "Закрыть" : "+ Новый опрос"}
                </button>
              )}
            </div>
            {showPollComposer && canCreatePoll && (
              <PollComposer
                disabled={working || polls.length >= 10}
                timeZone={meeting.timezone}
                onCreate={async (input) => {
                  const created = await createPoll(input);
                  if (created) setShowPollComposer(false);
                  return created;
                }}
              />
            )}
            <div className="poll-list">
              {polls.map((item) => (
                <PollCard
                  key={item.id}
                  poll={item}
                  isOwner={isOwner}
                  meetingState={meeting.state}
                  timeZone={meeting.timezone}
                  working={working}
                  onDelete={() => mutate(() => api.deletePoll(meetingID, item.id), "Опрос удалён.")}
                  onLoadHistory={() => api.getPollHistory(item.id)}
                  onVote={(optionIDs) => mutate(() => api.votePoll(item.id, optionIDs), "Ответ сохранён.")}
                  onClose={(optionID) => mutate(
                    () => api.closePoll(meetingID, item.id, optionID),
                    optionID ? "Опрос закрыт, итог закреплён." : "Опрос остановлен без закреплённого итога.",
                  )}
                />
              ))}
              {polls.length === 0 && (
                <div className="poll-workspace-empty">
                  <strong>Опросов пока нет</strong>
                  <p>Любой участник может задать группе вопрос и выбрать настройки ответа.</p>
                </div>
              )}
            </div>
          </section>
        )}

        {activeSection === "notes" && notes && (
          <MeetingNotesSection
            currentUserID={user.id}
            editable={!readOnly && (meeting.state === "draft" || meeting.state === "collecting" || meeting.state === "scheduled")}
            page={notes}
            timeZone={meeting.timezone}
            working={working}
            onDelete={() => mutate(() => api.deleteMeetingNote(meetingID), "Заметка удалена.")}
            onSave={(text) => mutate(() => api.upsertMeetingNote(meetingID, text), "Заметка сохранена.")}
          />
        )}

        {activeSection === "votes" && showsVotes && (
          <div
            aria-labelledby="meeting-tab-votes"
            className="meeting-tab-panel"
            id="meeting-section-votes"
            role="tabpanel"
          >
          <PlanDecisionSection
            availability={availability}
            meeting={meeting}
            planVotes={planVotes}
            working={working}
            onVote={(planOptionID) => mutate(
              () => api.setPlanVote(meetingID, planOptionID),
              planOptionID ? "Голос за план сохранён." : "Голос за план отозван.",
            )}
            onFinalize={(planOptionID, timeOptionID) => mutate(
              () => api.finalizeDecision(meetingID, planOptionID, timeOptionID),
              "Решение закреплено. Встреча подтверждена.",
            )}
          />
          </div>
        )}

        {activeSection === "preparation" && showsPreparation && preparation && (
          <div
            aria-labelledby="meeting-tab-preparation"
            className="meeting-tab-panel"
            id="meeting-section-preparation"
            role="tabpanel"
          >
          <PreparationSection
            isOwner={isOwner}
            meetingState={readOnly ? "completed" : meeting.state}
            page={preparation}
            working={working}
            onClaim={(requirementID, quantity) => mutate(
              () => api.setRequirementClaim(meetingID, requirementID, quantity),
              quantity === 0 ? "Вы отказались от позиции." : "Ваша доля сохранена.",
            )}
            onCreate={createRequirement}
            onUpdate={(requirementID, input) => mutate(
              () => api.updateRequirement(meetingID, requirementID, input),
              "Позиция подготовки обновлена.",
            )}
            onDelete={(requirementID) => mutate(
              () => api.deleteRequirement(meetingID, requirementID),
              "Пустая позиция удалена.",
            )}
            onStatus={(requirementID, status) => mutate(
              () => api.setRequirementStatus(meetingID, requirementID, status),
              status === "completed" ? "Позиция отмечена готовой." : "Позиция снова в работе.",
            )}
            onComplete={() => mutate(
              () => api.completeMeeting(meetingID),
              "Встреча завершена. Итог и история сохранены.",
            )}
          />
          </div>
        )}

        {activeSection === "availability" && showsAvailability && availability && (
          <div
            aria-labelledby="meeting-tab-availability"
            className="meeting-tab-panel"
            id="meeting-section-availability"
            role="tabpanel"
          >
          <AvailabilitySection
            availability={availability}
            meeting={meeting}
            currentUserID={user.id}
            working={working}
            onRespond={(timeOptionID, status) => mutate(
              () => api.setAvailability(timeOptionID, status),
              status === "unanswered" ? "Ответ очищен." : "Доступность сохранена.",
            )}
          />
          </div>
        )}

        {activeSection === "attendance" && showsAttendance && attendance && (
          <div
            aria-labelledby="meeting-tab-attendance"
            className="meeting-tab-panel"
            id="meeting-section-attendance"
            role="tabpanel"
          >
            <AttendanceSection
              attendance={attendance}
              meetingState={meeting.state}
              readOnly={readOnly}
              working={working}
              onRespond={(status) => mutate(
                () => api.setAttendance(meetingID, status),
                status === "going"
                  ? "Вы отметили, что пойдёте."
                  : status === "maybe"
                    ? "Вы отметили, что пока думаете."
                  : status === "not_going"
                    ? "Вы отметили, что не пойдёте."
                    : "Ответ об участии очищен.",
              )}
            />
          </div>
        )}

        {activeSection === "people" && (
          <div
            aria-labelledby="meeting-tab-people"
            className="meeting-tab-panel"
            id="meeting-section-people"
            role="tabpanel"
          >
          <section className="invitation-panel">
          <div>
            <p className="section-kicker">ПРИГЛАШЕНИЕ И УЧАСТНИКИ</p>
            <h2>
              {meeting.state === "draft"
                ? meeting.coordination_mode === "fixed" ? "Подтвердите встречу и пригласите" : "Откройте сбор ответов"
                : "Приглашение и участники"}
            </h2>
            <p className="muted">
              {meeting.state === "draft"
                ? meeting.coordination_mode === "fixed"
                  ? "Ссылка станет доступна после одного готового плана и одного времени. Встреча сразу станет подтверждённой."
                  : "Ссылка станет доступна после двух вариантов плана и двух вариантов времени."
                : meeting.state === "cancelled"
                  ? "Приглашение закрыто. Участники сохраняют доступ к итогам встречи."
                : "Ссылка работает семь дней. Новая ссылка сразу отменяет предыдущую."}
            </p>
          </div>
          <div className="participant-list">
            {visibleParticipants.map((participant) => (
              <span className="participant-chip" key={participant.user_id}>
                <i aria-hidden="true">{participant.display_name.slice(0, 1).toUpperCase()}</i>
                {participant.display_name}
                {participant.role === "owner" && <small>организатор</small>}
              </span>
            ))}
            {meeting.participants.length > PARTICIPANT_PREVIEW_LIMIT && (
              <button
                aria-expanded={showAllParticipants}
                className="list-expand-button participant-expand"
                onClick={() => setShowAllParticipants((current) => !current)}
                type="button"
              >
                {showAllParticipants
                  ? "Свернуть список"
                  : `Показать всех · ${meeting.participants.length}`}
              </button>
            )}
          </div>
          {isOwner && !readOnly && (
            <div className="invite-actions">
              {meeting.active_invitation_expires_at && (
                <p className="invite-expiry">
                  Активна до {dateFormatter.format(new Date(meeting.active_invitation_expires_at))}
                </p>
              )}
              {inviteLink && (
                <div className="invite-link">
                  <input aria-label="Ссылка приглашения" readOnly value={inviteLink} onFocus={(event) => event.currentTarget.select()} />
                  <button type="button" onClick={() => void copyInvitation()}>Копировать</button>
                </div>
              )}
              {(canInvite || meeting.state === "collecting" || (
                meeting.coordination_mode === "fixed" && meeting.state === "scheduled"
              )) && (
                <button className="primary-button" disabled={working} type="button" onClick={() => void createInvitation()}>
                  {meeting.active_invitation_expires_at ? "Создать новую ссылку" : "Создать ссылку приглашения"}
                  <span aria-hidden="true">→</span>
                </button>
              )}
              {meeting.active_invitation_expires_at && (
                meeting.state === "collecting" || (
                  meeting.coordination_mode === "fixed" && meeting.state === "scheduled"
                )
              ) && (
                <button className="quiet-button" disabled={working} type="button" onClick={() => void mutate(
                  () => api.revokeInvitation(meetingID), "Ссылка приглашения отозвана.",
                )}>Отключить ссылку</button>
              )}
            </div>
          )}
          </section>
          </div>
        )}

        {activeSection === "manage" && isOwner && meeting.state !== "cancelled" && meeting.state !== "completed" && (
          <div
            aria-labelledby="meeting-tab-manage"
            className="meeting-tab-panel"
            id="meeting-section-manage"
            role="tabpanel"
          >
          <div className="meeting-management-stack">
          {canEditMeetingDetails ? (
            <MeetingMetadataEditor
              key={`${meeting.id}-${meeting.version}`}
              meeting={meeting}
              working={working}
              onInvalid={setError}
              onPhotoDelete={() => void mutate(
                () => api.deleteMeetingPhoto(meetingID, meeting.version),
                "Фото встречи удалено.",
              )}
              onPhotoSelect={(file) => selectPhotoForCrop(file, "meeting")}
              onSave={updateMeeting}
            />
          ) : (
            <section className="meeting-editor-locked" aria-labelledby="meeting-editor-locked-title">
              <p className="section-kicker">О ВСТРЕЧЕ</p>
              <h2 id="meeting-editor-locked-title">Редактирование недоступно</h2>
              <p>
                {meeting.coordination_mode === "fixed"
                  ? "После подтверждения встречи основные детали нельзя менять, чтобы все отвечали про один и тот же план."
                  : "После начала сбора ответов название, формат и место нельзя менять, чтобы участники голосовали за один и тот же контекст."}
              </p>
            </section>
          )}
          <section className="meeting-danger-zone" aria-labelledby="cancel-meeting-title">
            <div>
              <p className="section-kicker">ОТМЕНА ВСТРЕЧИ</p>
              <h2 id="cancel-meeting-title">Отменить встречу</h2>
              <p>Сбор ответов и подготовка закроются, активная ссылка перестанет работать. Карточка останется доступна участникам только для чтения.</p>
            </div>
            {confirmingCancellation ? (
              <div className="cancellation-confirmation" role="group" aria-label="Подтверждение отмены встречи">
                <p>Отменить без возможности восстановления?</p>
                <button
                  className="danger-button"
                  disabled={working}
                  onClick={() => {
                    setConfirmingCancellation(false);
                    void mutate(
                      () => api.cancelMeeting(meetingID),
                      "Встреча отменена. Данные сохранены для просмотра.",
                    ).then(() => setActiveSection(null));
                  }}
                  type="button"
                >
                  Да, отменить
                </button>
                <button
                  className="quiet-button"
                  disabled={working}
                  onClick={() => setConfirmingCancellation(false)}
                  type="button"
                >
                  Оставить встречу
                </button>
              </div>
            ) : (
              <button
                className="quiet-button"
                disabled={working}
                onClick={() => setConfirmingCancellation(true)}
                type="button"
              >
                Отменить встречу
              </button>
            )}
          </section>
          </div>
          </div>
        )}
              </div>
            </section>
          </div>
        )}
      </main>
      {showShare && (
        <MeetingShareDialog
          inviteLink={inviteLink}
          meetingID={meeting.id}
          meetingTitle={meeting.title}
          onClose={() => setShowShare(false)}
          onCopy={copyInvitation}
          onReplace={createInvitation}
          working={working}
        />
      )}
      {photoCropTarget && (
        <PhotoCropDialog
          file={photoCropTarget.file}
          label={photoCropTarget.scope === "meeting" ? "встречи" : "места или плана"}
          onCancel={() => setPhotoCropTarget(null)}
          onConfirm={saveCroppedPhoto}
        />
      )}
    </div>
  );
}

function MeetingNotesSection({
  currentUserID,
  editable,
  page,
  timeZone,
  working,
  onDelete,
  onSave,
}: {
  currentUserID: string;
  editable: boolean;
  page: MeetingNotePage;
  timeZone: string;
  working: boolean;
  onDelete: () => Promise<void>;
  onSave: (text: string) => Promise<void>;
}) {
  const myNote = page.items.find((item) => item.user_id === currentUserID);
  const [text, setText] = useState(myNote?.text ?? "");
  const characterCount = Array.from(text).length;
  const normalizedText = text.trim();
  const changed = normalizedText !== (myNote?.text ?? "");
  const noteDateFormatter = createMeetingDateTimeFormatter(timeZone);

  useEffect(() => {
    setText(myNote?.text ?? "");
  }, [myNote?.text]);

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!editable || !normalizedText || characterCount > 200 || !changed) return;
    void onSave(normalizedText);
  }

  return (
    <section className="meeting-notes" aria-labelledby="meeting-notes-title">
      <header className="meeting-notes-heading">
        <div>
          <p className="section-kicker">ЗАМЕТКИ УЧАСТНИКОВ</p>
          <h2 id="meeting-notes-title">Важное от участников</h2>
          <p>По одной короткой заметке от человека — без лишней переписки.</p>
        </div>
        <span>{page.total}</span>
      </header>

      {editable ? (
        <form className="meeting-note-editor" onSubmit={submit}>
          <label>
            <span>{myNote ? "Ваша заметка" : "Добавить заметку"}</span>
            <textarea
              aria-label="Ваша заметка"
              onChange={(event) => {
                const value = event.currentTarget.value;
                if (Array.from(value).length <= 200) setText(value);
              }}
              placeholder="Например: буду на машине, могу забрать двоих"
              rows={3}
              value={text}
            />
          </label>
          <div className="meeting-note-editor-footer">
            <small>{characterCount} / 200</small>
            <div>
              {myNote && (
                <button className="quiet-button" disabled={working} onClick={() => void onDelete()} type="button">
                  Удалить
                </button>
              )}
              <button className="primary-button compact" disabled={working || !normalizedText || !changed} type="submit">
                {myNote ? "Сохранить изменения" : "Сохранить заметку"}
              </button>
            </div>
          </div>
        </form>
      ) : (
        <p className="meeting-notes-readonly">Встреча уже в архиве — заметки сохранены только для чтения.</p>
      )}

      <div className="meeting-note-list">
        {page.items.map((item) => (
          <article className={item.user_id === currentUserID ? "meeting-note-card mine" : "meeting-note-card"} key={item.user_id}>
            <header>
              <i aria-hidden="true">{item.display_name.slice(0, 1).toUpperCase()}</i>
              <div>
                <strong>{item.display_name}</strong>
                <time dateTime={item.updated_at}>{noteDateFormatter.format(new Date(item.updated_at))}</time>
              </div>
              {item.user_id === currentUserID && <small>ваша</small>}
            </header>
            <p>{item.text}</p>
          </article>
        ))}
        {page.items.length === 0 && (
          <div className="meeting-notes-empty">
            <strong>Заметок пока нет</strong>
            <p>Оставьте первую короткую деталь, которую полезно знать всей группе.</p>
          </div>
        )}
      </div>
      {page.items.length < page.total && (
        <p className="panel-empty">Показаны первые {page.items.length} из {page.total} заметок.</p>
      )}
    </section>
  );
}


function PlanOptionEditForm({
  option,
  version,
  working,
  onCancel,
  onSave,
}: {
  option: PlanOption;
  version: number;
  working: boolean;
  onCancel: () => void;
  onSave: (input: UpdatePlanOptionInput) => Promise<void>;
}) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    void onSave({
      title: String(data.get("title") ?? ""),
      description: String(data.get("description") ?? ""),
      version,
    });
  }

  return (
    <form className="option-edit-form" onSubmit={submit}>
      <Field
        label="Название варианта"
        name="title"
        autoComplete="off"
        minLength={1}
        maxLength={120}
        defaultValue={option.title}
      />
      <label className="field">
        <span>Пояснение <small>необязательно</small></span>
        <textarea name="description" rows={2} maxLength={500} defaultValue={option.description} />
      </label>
      <div className="option-edit-actions">
        <button className="secondary-button" disabled={working} type="submit">Сохранить план</button>
        <button className="quiet-button" disabled={working} onClick={onCancel} type="button">Отмена</button>
      </div>
    </form>
  );
}

function TimeOptionEditForm({
  option,
  plans,
  timeZone,
  version,
  working,
  onCancel,
  onInvalid,
  onSave,
}: {
  option: TimeOption;
  plans: PlanOption[];
  timeZone: string;
  version: number;
  working: boolean;
  onCancel: () => void;
  onInvalid: (message: string) => void;
  onSave: (input: UpdateTimeOptionInput) => Promise<void>;
}) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const planOptionID = String(data.get("plan_option_id") ?? "");
    let startsAt: string;
    let endsAt: string | null;
    try {
      ({ startsAt, endsAt } = readTimeSelection(data, timeZone));
    } catch (dateError) {
      onInvalid(dateError instanceof Error ? dateError.message : "Проверьте дату и время.");
      return;
    }
    void onSave({
      plan_option_id: planOptionID || null,
      starts_at: startsAt,
      ends_at: endsAt,
      version,
    });
  }

  return (
    <form className="option-edit-form time-option-edit-form" onSubmit={submit}>
      <p className="time-zone-note option-edit-wide">
        Время встречи: <strong>{meetingTimeZoneLabel(timeZone)}</strong>
      </p>
      <label className="field option-edit-wide">
        <span>К какому плану относится</span>
        <select name="plan_option_id" defaultValue={option.plan_option_id ?? ""}>
          <option value="">Общее для всех планов</option>
          {plans.map((plan) => <option value={plan.id} key={plan.id}>{plan.title}</option>)}
        </select>
      </label>
      <MeetingTimeFields
        timeZone={timeZone}
        defaultStart={option.starts_at}
        defaultEnd={option.ends_at}
      />
      <div className="option-edit-actions option-edit-wide">
        <button className="secondary-button" disabled={working} type="submit">Сохранить время</button>
        <button className="quiet-button" disabled={working} onClick={onCancel} type="button">Отмена</button>
      </div>
    </form>
  );
}

function MeetingMetadataEditor({
  meeting,
  working,
  onInvalid,
  onPhotoDelete,
  onPhotoSelect,
  onSave,
}: {
  meeting: MeetingDetail;
  working: boolean;
  onInvalid: (message: string) => void;
  onPhotoDelete: () => void;
  onPhotoSelect: (file: File) => void;
  onSave: (input: UpdateMeetingInput) => Promise<void>;
}) {
  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    let startsAt: string | null = null;
    let endsAt: string | null = null;
    if (meeting.coordination_mode === "fixed") {
      try {
        ({ startsAt, endsAt } = readTimeSelection(data, meeting.timezone));
      } catch (dateError) {
        onInvalid(dateError instanceof Error ? dateError.message : "Проверьте дату и время.");
        return;
      }
    }
    void onSave({
      title: String(data.get("title") ?? ""),
      description: String(data.get("description") ?? ""),
      event_type: meeting.event_type,
      cover_url: meeting.cover_url,
      location_name: String(data.get("location_name") ?? "").trim() || null,
      location_url: String(data.get("location_url") ?? "").trim() || null,
      ...(meeting.coordination_mode === "fixed" ? { starts_at: startsAt, ends_at: endsAt } : {}),
      version: meeting.version,
    });
  }

  return (
    <section className="meeting-editor" aria-labelledby="meeting-editor-title">
      <div className="meeting-editor-heading">
        <div>
          <p className="section-kicker">О ВСТРЕЧЕ</p>
          <h2 id="meeting-editor-title">Редактировать встречу</h2>
        </div>
        <p>
          {meeting.state === "draft"
            ? "Уточните детали до приглашения участников."
            : "Изменения сразу увидят все участники; их ответы и подготовка сохранятся."}
        </p>
      </div>
      <form className="meeting-editor-form" onSubmit={submit}>
        <Field
          label="Название встречи"
          name="title"
          autoComplete="off"
          minLength={1}
          maxLength={120}
          defaultValue={meeting.title}
        />
        <label className="field meeting-editor-wide">
          <span>Коротко о плане <small>необязательно</small></span>
          <textarea name="description" maxLength={2000} rows={4} defaultValue={meeting.description} />
        </label>
        {meeting.coordination_mode === "fixed" && (
          <div className="meeting-editor-wide meeting-editor-time">
            <MeetingTimeFields
              timeZone={meeting.timezone}
              defaultStart={meeting.time_options[0]?.starts_at}
              defaultEnd={meeting.time_options[0]?.ends_at}
            />
          </div>
        )}
        <label className="field">
          <span>Место <small>необязательно</small></span>
          <input
            name="location_name"
            autoComplete="off"
            maxLength={200}
            defaultValue={meeting.location_name ?? ""}
            placeholder="Дом Анны или Северный парк"
          />
        </label>
        <div className="meeting-editor-wide meeting-editor-photo">
          <div>
            <strong>Фото встречи</strong>
            <small>Необязательно · после выбора можно настроить квадратный кадр</small>
          </div>
          {(meeting.has_photo || meeting.cover_url) && (
            <MeetingCover
              meetingID={meeting.id}
              hasStoredPhoto={Boolean(meeting.has_photo)}
              source={meeting.cover_url}
              title={meeting.title}
              revision={meeting.version}
            />
          )}
          <PhotoControls
            hasPhoto={Boolean(meeting.has_photo || meeting.cover_url)}
            label="фото встречи"
            working={working}
            onDelete={onPhotoDelete}
            onSelect={onPhotoSelect}
          />
        </div>
        <label className="field">
          <span>Ссылка на место <small>необязательно · должна начинаться с https://</small></span>
          <input
            name="location_url"
            type="url"
            inputMode="url"
            maxLength={2048}
            pattern="https://.*"
            defaultValue={meeting.location_url ?? ""}
            placeholder="https://maps.example.com/place"
          />
        </label>
        <div className="meeting-editor-actions">
          <small>После сохранения изменения увидят все участники.</small>
          <button className="primary-button compact" disabled={working} type="submit">
            {working ? "Сохраняем…" : "Сохранить изменения"}
            <span aria-hidden="true">→</span>
          </button>
        </div>
      </form>
    </section>
  );
}

function PreparationSection({
  isOwner,
  meetingState,
  page,
  working,
  onClaim,
  onCreate,
  onUpdate,
  onDelete,
  onStatus,
  onComplete,
}: {
  isOwner: boolean;
  meetingState: Meeting["state"];
  page: RequirementPage;
  working: boolean;
  onClaim: (requirementID: string, quantity: number) => Promise<void>;
  onCreate: (event: FormEvent<HTMLFormElement>) => Promise<void>;
  onUpdate: (requirementID: string, input: RequirementInput) => Promise<void>;
  onDelete: (requirementID: string) => Promise<void>;
  onStatus: (requirementID: string, status: "open" | "completed") => Promise<void>;
  onComplete: () => Promise<void>;
}) {
  const editable = meetingState === "scheduled";
  const completion = page.total === 0 ? 0 : Math.round((page.completed_count / page.total) * 100);
  const [showCompleted, setShowCompleted] = useState(false);
  const [showCreate, setShowCreate] = useState(false);
  const openItems = page.items.filter((item) => item.status === "open");
  const visibleItems = showCompleted ? page.items : openItems;

  return (
    <section className="preparation-section" aria-labelledby="preparation-title">
      <div className="preparation-heading">
        <div>
          <p className="section-kicker">ПОДГОТОВКА</p>
          <h2 id="preparation-title">Что ещё нужно</h2>
          <p>Возьмите нужное количество на себя. Готовые позиции скрыты, чтобы список оставался коротким.</p>
        </div>
        <div className="preparation-summary" aria-label="Прогресс подготовки">
          <strong>{page.completed_count} / {page.total}</strong>
          <span>готово</span>
          <i><b style={{ width: `${completion}%` }} /></i>
        </div>
      </div>

      <div className="preparation-toolbar">
        <label className="preparation-completed-toggle">
          <input
            checked={showCompleted}
            onChange={(event) => setShowCompleted(event.currentTarget.checked)}
            type="checkbox"
          />
          <span>Показывать готовые</span>
          <small>{page.completed_count}</small>
        </label>
        {isOwner && editable && (
          <button
            aria-expanded={showCreate}
            className="secondary-button preparation-add-button"
            onClick={() => setShowCreate((current) => !current)}
            type="button"
          >
            {showCreate ? "Закрыть" : "+ Добавить"}
          </button>
        )}
      </div>

      <div className="requirement-list">
        {visibleItems.map((item) => (
          <RequirementCard
            editable={editable}
            isOwner={isOwner}
            item={item}
            key={item.id}
            number={page.items.findIndex((candidate) => candidate.id === item.id) + 1}
            working={working}
            onClaim={onClaim}
            onDelete={onDelete}
            onStatus={onStatus}
            onUpdate={onUpdate}
          />
        ))}
        {visibleItems.length === 0 && (
          <p className="panel-empty">
            {page.items.length === 0
              ? "Организатор ещё не добавил, что нужно подготовить."
              : "Все позиции уже готовы. Включите «Показывать готовые», чтобы посмотреть итог."}
          </p>
        )}
      </div>

      {isOwner && editable && showCreate && (
        <form className="requirement-create-form" onSubmit={(event) => void onCreate(event)}>
          <Field
            autoComplete="off"
            label="Что нужно"
            maxLength={120}
            name="name"
            placeholder="Например, вода в бутылках"
          />
          <label className="field">
            <span>Сколько нужно</span>
            <input max={100000} min={1} name="required_quantity" required type="number" />
          </label>
          <button
            className="primary-button"
            disabled={working || page.total >= 50}
            type="submit"
          >
            Добавить позицию <span aria-hidden="true">→</span>
          </button>
        </form>
      )}

      <div className={`preparation-footer ${meetingState}`}>
        <span>
          {meetingState === "cancelled" ? "Подготовка закрыта после отмены встречи."
            : meetingState === "completed" ? "Итог подготовки сохранён."
              : page.open_count > 0 ? `Осталось завершить позиций: ${page.open_count}.`
                : "Подготовка завершена."}
        </span>
        {isOwner && meetingState === "scheduled" && (
          <button
            className="secondary-button"
            disabled={working || page.open_count > 0}
            onClick={() => void onComplete()}
            type="button"
          >
            Завершить встречу
          </button>
        )}
      </div>
    </section>
  );
}

function RequirementCard({
  editable,
  isOwner,
  item,
  number,
  working,
  onClaim,
  onDelete,
  onStatus,
  onUpdate,
}: {
  editable: boolean;
  isOwner: boolean;
  item: Requirement;
  number: number;
  working: boolean;
  onClaim: (requirementID: string, quantity: number) => Promise<void>;
  onDelete: (requirementID: string) => Promise<void>;
  onStatus: (requirementID: string, status: "open" | "completed") => Promise<void>;
  onUpdate: (requirementID: string, input: RequirementInput) => Promise<void>;
}) {
  const [quantity, setQuantity] = useState(String(item.my_quantity));
  const [editing, setEditing] = useState(false);
  const [editName, setEditName] = useState(item.name);
  const [editRequiredQuantity, setEditRequiredQuantity] = useState(String(item.required_quantity));
  useEffect(() => {
    setQuantity(String(item.my_quantity));
  }, [item.id, item.my_quantity]);
  useEffect(() => {
    setEditName(item.name);
    setEditRequiredQuantity(String(item.required_quantity));
    setEditing(false);
  }, [item.id, item.name, item.required_quantity]);
  const parsedQuantity = Number(quantity);
  const maximum = item.my_quantity + item.remaining_quantity;
  const validQuantity = Number.isInteger(parsedQuantity) &&
    parsedQuantity >= 0 &&
    parsedQuantity <= maximum;
  const allocation = Math.min(
    100,
    Math.round((item.claimed_quantity / item.required_quantity) * 100),
  );
  const completed = item.status === "completed";
  const parsedRequiredQuantity = Number(editRequiredQuantity);
  const validEdit = editName.trim().length > 0 &&
    editName.trim().length <= 120 &&
    Number.isInteger(parsedRequiredQuantity) &&
    parsedRequiredQuantity >= item.claimed_quantity &&
    parsedRequiredQuantity >= 1 &&
    parsedRequiredQuantity <= 100_000;
  const editChanged = editName.trim() !== item.name ||
    parsedRequiredQuantity !== item.required_quantity;

  return (
    <article
      aria-labelledby={`requirement-title-${item.id}`}
      className={`requirement-card ${completed ? "completed" : ""} ${item.my_quantity > 0 ? "mine" : ""}`}
    >
      <header>
        <span className="requirement-number">{String(number).padStart(2, "0")}</span>
        <div>
          <div className="requirement-flags">
            <span className="requirement-status">{completed ? "Готово" : "В работе"}</span>
            {item.my_quantity > 0 && <span className="requirement-mine">Моя ответственность · {item.my_quantity}</span>}
          </div>
          <h3 id={`requirement-title-${item.id}`}>{item.name}</h3>
        </div>
        <div className="requirement-total">
          <strong>{item.claimed_quantity} / {item.required_quantity}</strong>
          <span>{item.remaining_quantity > 0 ? `осталось ${item.remaining_quantity}` : "распределено"}</span>
        </div>
      </header>

      <div
        aria-label={`Распределено ${item.claimed_quantity} из ${item.required_quantity}`}
        className="requirement-progress"
        role="progressbar"
        aria-valuemax={item.required_quantity}
        aria-valuemin={0}
        aria-valuenow={item.claimed_quantity}
      >
        <i style={{ width: `${allocation}%` }} />
      </div>

      <div className="requirement-body">
        <div>
          <span className="requirement-label">Ответственные</span>
          <div className="assignee-list">
            {item.assignees.map((assignee) => (
              <span className="assignee-chip" key={assignee.user_id}>
                <i aria-hidden="true">{assignee.display_name.slice(0, 1).toUpperCase()}</i>
                {assignee.display_name}
                <strong>{assignee.quantity}</strong>
              </span>
            ))}
            {item.assignees.length === 0 && <small>Пока никто не взял на себя.</small>}
          </div>
        </div>

        {editable && !completed && (
          <form
            className="claim-form"
            onSubmit={(event) => {
              event.preventDefault();
              if (validQuantity) void onClaim(item.id, parsedQuantity);
            }}
          >
            <span className="claim-title">Ваша доля</span>
            <div className="claim-quantity-control">
              <button
                aria-label={`Уменьшить долю: ${item.name}`}
                disabled={working || !validQuantity || parsedQuantity <= 0}
                onClick={() => setQuantity(String(Math.max(0, parsedQuantity - 1)))}
                type="button"
              >
                −
              </button>
              <input
                aria-label={`Ваша доля: ${item.name}`}
                max={maximum}
                min={0}
                onChange={(event) => setQuantity(event.currentTarget.value)}
                required
                type="number"
                value={quantity}
              />
              <button
                aria-label={`Увеличить долю: ${item.name}`}
                disabled={working || !validQuantity || parsedQuantity >= maximum}
                onClick={() => setQuantity(String(Math.min(maximum, parsedQuantity + 1)))}
                type="button"
              >
                +
              </button>
            </div>
            <div className="claim-quick-actions">
              {item.remaining_quantity > 0 && (
                <button
                  className="quiet-button"
                  disabled={working}
                  onClick={() => setQuantity(String(maximum))}
                  type="button"
                >
                  Взять остаток · {item.remaining_quantity}
                </button>
              )}
              {item.my_quantity > 0 && (
                <button
                  className="quiet-button"
                  disabled={working}
                  onClick={() => {
                    setQuantity("0");
                    void onClaim(item.id, 0);
                  }}
                  type="button"
                >
                  Отказаться
                </button>
              )}
            </div>
            <button
              className="secondary-button"
              disabled={working || !validQuantity || parsedQuantity === item.my_quantity}
              type="submit"
            >
              Сохранить · {validQuantity ? parsedQuantity : "—"}
            </button>
          </form>
        )}
      </div>

      {isOwner && editable && (
        <div className="requirement-owner-action">
          {editing && !completed && (
            <form
              className="requirement-edit-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (validEdit && editChanged) {
                  void onUpdate(item.id, {
                    name: editName.trim(),
                    required_quantity: parsedRequiredQuantity,
                  });
                }
              }}
            >
              <label className="field">
                <span>Название позиции</span>
                <input
                  maxLength={120}
                  onChange={(event) => setEditName(event.currentTarget.value)}
                  required
                  value={editName}
                />
              </label>
              <label className="field">
                <span>Нужно всего <small>уже взято: {item.claimed_quantity}</small></span>
                <input
                  max={100000}
                  min={Math.max(1, item.claimed_quantity)}
                  onChange={(event) => setEditRequiredQuantity(event.currentTarget.value)}
                  required
                  type="number"
                  value={editRequiredQuantity}
                />
              </label>
              <button
                className="secondary-button"
                disabled={working || !validEdit || !editChanged}
                type="submit"
              >
                Сохранить изменения
              </button>
              <button
                className="quiet-button"
                disabled={working}
                onClick={() => {
                  setEditName(item.name);
                  setEditRequiredQuantity(String(item.required_quantity));
                  setEditing(false);
                }}
                type="button"
              >
                Отмена
              </button>
            </form>
          )}
          <div className="requirement-owner-buttons">
            {!completed && (
              <button
                className="quiet-button"
                disabled={working || editing}
                onClick={() => setEditing(true)}
                type="button"
              >
                Изменить
              </button>
            )}
            {!completed && (
              <button
                className="quiet-button"
                disabled={working || item.claimed_quantity > 0}
                onClick={() => {
                  if (window.confirm(`Удалить пустую позицию «${item.name}»?`)) {
                    void onDelete(item.id);
                  }
                }}
                title={item.claimed_quantity > 0 ? "Сначала участники должны снять свои доли" : undefined}
                type="button"
              >
                Удалить
              </button>
            )}
          <button
            className={completed ? "quiet-button" : "secondary-button"}
            disabled={working || (!completed && item.claimed_quantity !== item.required_quantity)}
            onClick={() => void onStatus(item.id, completed ? "open" : "completed")}
            type="button"
          >
            {completed ? "Вернуть в работу" : "Отметить готовым"}
          </button>
          </div>
        </div>
      )}
    </article>
  );
}

function PlanDecisionSection({
  availability,
  meeting,
  planVotes,
  working,
  onVote,
  onFinalize,
}: {
  availability: AvailabilityView | null;
  meeting: MeetingDetail;
  planVotes: PlanVotePage;
  working: boolean;
  onVote: (planOptionID: string | null) => Promise<void>;
  onFinalize: (planOptionID: string, timeOptionID: string) => Promise<void>;
}) {
  const currentVoteID = planVotes.options.find((option) => option.selected_by_user)?.id ?? "";
  const rankedOptions = useMemo(
    () => [...planVotes.options].sort((left, right) =>
      right.vote_count - left.vote_count || left.position - right.position),
    [planVotes.options],
  );
  const initialPlanID = meeting.selected_plan_option_id ?? rankedOptions[0]?.id ?? "";
  const [voteID, setVoteID] = useState(currentVoteID);
  const [finalPlanID, setFinalPlanID] = useState(initialPlanID);
  const [finalTimeID, setFinalTimeID] = useState(meeting.selected_time_option_id ?? "");
  const [reviewingDecision, setReviewingDecision] = useState(false);
  const [showHistory, setShowHistory] = useState(false);
  const [expandedVotersID, setExpandedVotersID] = useState("");
  const dateFormatter = createMeetingDateTimeFormatter(meeting.timezone);

  const compatibleTimes = useMemo(
    () => meeting.time_options.filter(
      (option) => !option.plan_option_id || option.plan_option_id === finalPlanID,
    ),
    [finalPlanID, meeting.time_options],
  );

  useEffect(() => {
    setVoteID(currentVoteID);
  }, [currentVoteID]);

  useEffect(() => {
    setReviewingDecision(false);
  }, [meeting.version]);

  useEffect(() => {
    if (!finalPlanID && rankedOptions[0]) {
      setFinalPlanID(rankedOptions[0].id);
      return;
    }
    if (meeting.selected_time_option_id) {
      setFinalTimeID(meeting.selected_time_option_id);
      return;
    }
    const currentIsCompatible = compatibleTimes.some((option) => option.id === finalTimeID);
    if (!currentIsCompatible) {
      const recommendedID = availability?.recommendations.find(
        (item) => item.plan_option_id === finalPlanID,
      )?.time_option_id;
      setFinalTimeID(
        compatibleTimes.find((option) => option.id === recommendedID)?.id
          ?? compatibleTimes[0]?.id
          ?? "",
      );
    }
  }, [
    availability,
    compatibleTimes,
    finalPlanID,
    finalTimeID,
    meeting.selected_time_option_id,
    rankedOptions,
  ]);

  const selectedPlan = meeting.plan_options.find(
    (option) => option.id === meeting.selected_plan_option_id,
  );
  const selectedTime = meeting.time_options.find(
    (option) => option.id === meeting.selected_time_option_id,
  );
  const decisionPlan = rankedOptions.find((option) => option.id === finalPlanID);
  const decisionTime = meeting.time_options.find((option) => option.id === finalTimeID);
  const decisionTimeResult = availability?.items.find((option) => option.id === finalTimeID);
  const decisionRecommendation = availability?.recommendations.find(
    (item) => item.plan_option_id === finalPlanID,
  );
  const decisionUsesRecommendation = decisionRecommendation?.time_option_id === finalTimeID;
  const planVotePercent = !decisionPlan || planVotes.answered_count === 0
    ? 0
    : Math.round((decisionPlan.vote_count / planVotes.answered_count) * 100);
  const missingPlanVotes = Math.max(0, planVotes.participant_count - planVotes.answered_count);
  const missingTimeAnswers = decisionTimeResult?.counts.unanswered ?? availability?.participants.length ?? 0;
  const hardConflicts = decisionTimeResult?.counts.unavailable ?? 0;

  function historyText(entry: PlanVotePage["history"][number]): string {
    if (entry.action === "change") {
      return `сменил выбор: «${entry.previous_plan_title}» → «${entry.new_plan_title}»`;
    }
    if (entry.action === "retract") {
      return `отозвал голос за «${entry.previous_plan_title}»`;
    }
    return `выбрал «${entry.new_plan_title}»`;
  }

  return (
    <section className="plan-decision-section" aria-labelledby="plan-vote-title">
      <div className="section-heading">
        <div>
          <p className="section-kicker">ЧТО ВЫБИРАЕМ</p>
          <h2 id="plan-vote-title">Голоса за план</h2>
        </div>
        <span>{planVotes.answered_count} / {planVotes.participant_count} ответили</span>
      </div>
      <p className="decision-intro">
        У каждого один текущий голос. Его можно сменить или отозвать до решения организатора;
        реальные изменения остаются в истории.
      </p>

      <form
        className="plan-vote-form"
        onSubmit={(event) => {
          event.preventDefault();
          if (voteID) void onVote(voteID);
        }}
      >
        <div className="plan-vote-options">
          {rankedOptions.map((option) => {
            const voters = planVotes.responses
              .filter((response) => response.plan_option_id === option.id);
            const percent = planVotes.answered_count === 0
              ? 0
              : Math.round((option.vote_count / planVotes.answered_count) * 100);
            return (
              <article className={`plan-vote-card ${voteID === option.id ? "selected" : ""}`} key={option.id}>
                <label className="plan-vote-choice">
                  <input
                    checked={voteID === option.id}
                    disabled={working || meeting.state !== "collecting"}
                    name="plan-vote"
                    onChange={() => setVoteID(option.id)}
                    type="radio"
                    value={option.id}
                  />
                  <span className="plan-vote-copy">
                    <strong>{option.title}</strong>
                    {option.description && <small>{option.description}</small>}
                    <span className="plan-vote-track"><i style={{ width: `${percent}%` }} aria-hidden="true" /></span>
                  </span>
                  <span className="plan-vote-result">
                    <strong>{percent}%</strong>
                    <small>{option.vote_count} {option.vote_count === 1 ? "голос" : "голосов"}</small>
                  </span>
                </label>
                <div className="result-voters">
                  <button
                    aria-expanded={expandedVotersID === option.id}
                    disabled={voters.length === 0}
                    onClick={() => setExpandedVotersID((current) => current === option.id ? "" : option.id)}
                    type="button"
                  >
                    {voters.length === 0 ? "Пока без голосов" : `Кто выбрал · ${voters.length}`}
                  </button>
                  {expandedVotersID === option.id && voters.length > 0 && (
                    <div className="plan-voters">
                      {voters.map((voter) => (
                        <span title={voter.display_name} key={voter.user_id}>
                          <i aria-hidden="true">{voter.display_name.slice(0, 1).toUpperCase()}</i>
                          {voter.display_name}
                        </span>
                      ))}
                    </div>
                  )}
                </div>
              </article>
            );
          })}
        </div>
        {meeting.state === "collecting" && (
          <div className="plan-vote-actions">
            <button
              className="secondary-button"
              disabled={working || !voteID || voteID === currentVoteID}
              type="submit"
            >
              Сохранить голос
            </button>
            {currentVoteID && (
              <button
                className="quiet-button"
                disabled={working}
                onClick={() => void onVote(null)}
                type="button"
              >
                Отозвать голос
              </button>
            )}
          </div>
        )}
      </form>

      <div className="decision-and-history">
        <article className="final-decision-panel">
          <p className="section-kicker">РЕШЕНИЕ ОРГАНИЗАТОРА</p>
          {(meeting.state === "scheduled" || meeting.state === "completed" || meeting.state === "cancelled") && selectedPlan && selectedTime ? (
            <>
              <h3>{meeting.state === "cancelled" ? "Решение до отмены" : "Встреча подтверждена"}</h3>
              <dl className="confirmed-decision">
                <div><dt>План</dt><dd>{selectedPlan.title}</dd></div>
                <div><dt>Время</dt><dd>{dateFormatter.format(new Date(selectedTime.starts_at))}</dd></div>
              </dl>
              <p>Эта пара закреплена; голоса и история остались доступны для проверки.</p>
            </>
          ) : meeting.state === "cancelled" ? (
            <>
              <h3>Встреча отменена до решения</h3>
              <p>Организатор остановил сбор до закрепления итогового плана и времени. Голоса и их история сохранены.</p>
            </>
          ) : meeting.participant_role === "owner" ? (
            <form
              className="final-decision-form"
              onSubmit={(event) => {
                event.preventDefault();
                if (finalPlanID && finalTimeID) setReviewingDecision(true);
              }}
            >
              {reviewingDecision && decisionPlan && decisionTime ? (
                <div aria-label="Проверка решения" className="decision-review" role="region">
                  <div>
                    <p className="section-kicker">ПОСЛЕДНЯЯ ПРОВЕРКА</p>
                    <h3>Подтвердить эту встречу?</h3>
                  </div>
                  <dl className="decision-review-pair">
                    <div>
                      <dt>План</dt>
                      <dd>{decisionPlan.title}</dd>
                    </div>
                    <div>
                      <dt>Время</dt>
                      <dd>{dateFormatter.format(new Date(decisionTime.starts_at))}</dd>
                    </div>
                  </dl>
                  {(missingPlanVotes > 0 || missingTimeAnswers > 0 || hardConflicts > 0) && (
                    <div className="decision-review-warning">
                      <strong>Решение можно подтвердить, но ответы ещё не идеальны.</strong>
                      <ul>
                        {missingPlanVotes > 0 && (
                          <li>План не выбрали: {missingPlanVotes}.</li>
                        )}
                        {missingTimeAnswers > 0 && (
                          <li>Выбранное время не отметили: {missingTimeAnswers}.</li>
                        )}
                        {hardConflicts > 0 && (
                          <li>Не смогут в выбранное время: {hardConflicts}.</li>
                        )}
                      </ul>
                    </div>
                  )}
                  <p>
                    После подтверждения ответы закроются, активная ссылка приглашения
                    будет отозвана, а для группы откроется подготовка.
                  </p>
                  <div className="decision-review-actions">
                    <button
                      className="primary-button"
                      disabled={working}
                      onClick={() => void onFinalize(finalPlanID, finalTimeID)}
                      type="button"
                    >
                      Да, подтвердить встречу
                    </button>
                    <button
                      className="quiet-button"
                      disabled={working}
                      onClick={() => setReviewingDecision(false)}
                      type="button"
                    >
                      Вернуться к выбору
                    </button>
                  </div>
                </div>
              ) : (
                <>
                  <h3>Закрепить план и время</h3>
                  <div className="decision-selectors">
                    <label className="field">
                      <span>План</span>
                      <select
                        disabled={working}
                        onChange={(event) => {
                          setFinalPlanID(event.currentTarget.value);
                          setReviewingDecision(false);
                        }}
                        value={finalPlanID}
                      >
                        {rankedOptions.map((option) => (
                          <option value={option.id} key={option.id}>
                            {option.title} · {option.vote_count}
                          </option>
                        ))}
                      </select>
                    </label>
                    <label className="field">
                      <span>Совместимое время</span>
                      <select
                        disabled={working}
                        onChange={(event) => {
                          setFinalTimeID(event.currentTarget.value);
                          setReviewingDecision(false);
                        }}
                        value={finalTimeID}
                      >
                        {compatibleTimes.map((option) => {
                          const recommended = availability?.recommendations.some(
                            (item) => item.plan_option_id === finalPlanID && item.time_option_id === option.id,
                          );
                          return (
                            <option value={option.id} key={option.id}>
                              {dateFormatter.format(new Date(option.starts_at))}
                              {option.plan_option_id ? " · для этого плана" : " · общее"}
                              {recommended ? " · рекомендация" : ""}
                            </option>
                          );
                        })}
                      </select>
                    </label>
                  </div>

                  {decisionPlan && decisionTime && (
                    <div
                      aria-label="Основания решения"
                      className="decision-evidence"
                      role="region"
                    >
                      <article>
                        <small>Голоса за план</small>
                        <strong>{planVotePercent}%</strong>
                        <span>
                          {decisionPlan.vote_count} из {planVotes.answered_count} ответивших
                        </span>
                      </article>
                      <article>
                        <small>Ответили по времени</small>
                        <strong>
                          {decisionTimeResult
                            ? availability!.participants.length - missingTimeAnswers
                            : "—"}
                        </strong>
                        <span>
                          из {availability?.participants.length ?? 0} участников
                        </span>
                      </article>
                      <article className={hardConflicts > 0 ? "has-conflict" : ""}>
                        <small>Не смогут</small>
                        <strong>{decisionTimeResult ? hardConflicts : "—"}</strong>
                        <span>
                          {hardConflicts > 0 ? "есть жёсткие ограничения" : "нет конфликтов"}
                        </span>
                      </article>
                    </div>
                  )}

                  {decisionTimeResult && (
                    <dl className="decision-availability" aria-label="Доступность выбранного времени">
                      <div><dt>Предпочитают</dt><dd>{decisionTimeResult.counts.preferred}</dd></div>
                      <div><dt>Могут</dt><dd>{decisionTimeResult.counts.available}</dd></div>
                      <div><dt>Если нужно</dt><dd>{decisionTimeResult.counts.if_needed}</dd></div>
                      <div><dt>Не могут</dt><dd>{decisionTimeResult.counts.unavailable}</dd></div>
                      <div><dt>Без ответа</dt><dd>{decisionTimeResult.counts.unanswered}</dd></div>
                    </dl>
                  )}

                  {decisionRecommendation && (
                    <div className={`decision-recommendation ${decisionUsesRecommendation ? "is-match" : ""}`}>
                      <strong>
                        {decisionUsesRecommendation
                          ? decisionRecommendation.provisional
                            ? "Выбрана предварительная рекомендация Ryden"
                            : "Выбрана рекомендация Ryden"
                          : "Ryden рекомендует другое совместимое время"}
                      </strong>
                      <p>{decisionRecommendation.explanation}</p>
                    </div>
                  )}

                  <div className="decision-readiness" aria-label="Готовность ответов">
                    <span className={missingPlanVotes === 0 ? "is-ready" : "is-pending"}>
                      План: {missingPlanVotes === 0 ? "все ответили" : `без ответа ${missingPlanVotes}`}
                    </span>
                    <span className={missingTimeAnswers === 0 ? "is-ready" : "is-pending"}>
                      Время: {missingTimeAnswers === 0 ? "все ответили" : `без ответа ${missingTimeAnswers}`}
                    </span>
                  </div>

                  <p>
                    Ryden объясняет данные, но решение остаётся за организатором.
                    Перед необратимым подтверждением покажем выбранную пару ещё раз.
                  </p>
                  <button
                    className="primary-button"
                    disabled={working || !finalPlanID || !finalTimeID}
                    type="submit"
                  >
                    Проверить решение <span aria-hidden="true">→</span>
                  </button>
                </>
              )}
            </form>
          ) : (
            <>
              <h3>Решение за организатором</h3>
              <p>Голоса помогают сравнить варианты, но итоговую совместимую пару закрепляет организатор.</p>
            </>
          )}
        </article>

        {showHistory ? (
        <article className="vote-history-panel">
          <div className="history-heading">
            <div>
              <p className="section-kicker">ЖУРНАЛ ИЗМЕНЕНИЙ</p>
              <h3>История голосований</h3>
            </div>
            <span>{planVotes.history_total}</span>
          </div>
          {planVotes.history.length === 0 ? (
            <p className="panel-empty">Изменений пока нет.</p>
          ) : (
            <ol className="vote-history-list">
              {planVotes.history.map((entry) => (
                <li key={entry.id}>
                  <span className="history-mark" aria-hidden="true">
                    {entry.display_name.slice(0, 1).toUpperCase()}
                  </span>
                  <div>
                    <strong>{entry.display_name}</strong>
                    <p>{historyText(entry)}</p>
                    <time dateTime={entry.created_at}>{dateFormatter.format(new Date(entry.created_at))}</time>
                  </div>
                </li>
              ))}
            </ol>
          )}
          {planVotes.history_total > planVotes.history.length && (
            <p className="history-note">Показаны последние {planVotes.history.length} изменений.</p>
          )}
          <button className="quiet-button history-close" onClick={() => setShowHistory(false)} type="button">
            Скрыть историю
          </button>
        </article>
        ) : (
          <button className="history-toggle" onClick={() => setShowHistory(true)} type="button">
            <span>История голосований</span>
            <strong>{planVotes.history_total}</strong>
            <small>Посмотреть изменения выбора участников</small>
          </button>
        )}
      </div>
    </section>
  );
}

const availabilityLabels: Record<AvailabilityStatus, string> = {
  preferred: "Предпочитаю",
  available: "Могу",
  if_needed: "Если нужно",
  unavailable: "Не могу",
  unanswered: "Нет ответа",
};

const availabilityStatusOrder: AvailabilityStatus[] = [
  "preferred",
  "available",
  "if_needed",
  "unavailable",
  "unanswered",
];

function AvailabilitySection({
  availability,
  meeting,
  currentUserID,
  working,
  onRespond,
}: {
  availability: AvailabilityView;
  meeting: MeetingDetail;
  currentUserID: string;
  working: boolean;
  onRespond: (timeOptionID: string, status: AvailabilityStatus) => Promise<void>;
}) {
  const [expandedTimeID, setExpandedTimeID] = useState("");
  const [showAllRecommendations, setShowAllRecommendations] = useState(false);
  const [timeFilter, setTimeFilter] = useState<"all" | "unanswered">("all");
  const timeByID = new Map(availability.items.map((item) => [item.id, item]));
  const compactDate = createMeetingDateTimeFormatter(meeting.timezone, "compact");
  const unansweredTimes = availability.items.filter((item) => item.my_status === "unanswered");
  const visibleTimes = timeFilter === "unanswered" ? unansweredTimes : availability.items;
  const visibleRecommendations = showAllRecommendations
    ? availability.recommendations
    : availability.recommendations.slice(0, RECOMMENDATION_PREVIEW_LIMIT);

  return (
    <section className="availability-section" aria-labelledby="availability-title">
      <div className="section-heading">
        <div>
          <p className="section-kicker">КОГДА ВСЕМ УДОБНО</p>
          <h2 id="availability-title">Доступность</h2>
        </div>
        <span>{availability.participants.length} участников</span>
      </div>
      <div className="availability-intro">
        <p>Отметьте каждый вариант. «Не могу» — жёсткое препятствие; рекомендации не смешивают время разных планов.</p>
        <p className="weight-legend">
          Вес: <strong>предпочитаю +{availability.weights.preferred}</strong>
          <span>могу +{availability.weights.available}</span>
          <span>если нужно +{availability.weights.if_needed}</span>
          <span>не могу {availability.weights.unavailable}</span>
        </p>
      </div>

      <div className="recommendation-list">
        {visibleRecommendations.map((recommendation) => {
          const time = timeByID.get(recommendation.time_option_id);
          if (!time) return null;
          const isGeneral = time.plan_option_id === null;
          return (
            <article className="recommendation-card" key={recommendation.plan_option_id}>
              <div>
                <small>Для плана</small>
                <h3>{recommendation.plan_title}</h3>
              </div>
              <div className="recommendation-time">
                <strong>{compactDate.format(new Date(time.starts_at))}</strong>
                <small>{isGeneral ? "Общее время" : "Время этого плана"}</small>
              </div>
              <p>{recommendation.explanation}</p>
              <span className={recommendation.provisional ? "status-provisional" : "status-ready"}>
                {recommendation.provisional ? "Предварительно" : "Все ответили"}
              </span>
            </article>
          );
        })}
      </div>

      {availability.recommendations.length > RECOMMENDATION_PREVIEW_LIMIT && (
        <button
          aria-expanded={showAllRecommendations}
          className="list-expand-button recommendation-expand"
          onClick={() => setShowAllRecommendations((current) => !current)}
          type="button"
        >
          {showAllRecommendations
            ? "Свернуть рекомендации"
            : `Все рекомендации · ${availability.recommendations.length}`}
        </button>
      )}

      <div className="availability-toolbar">
        <div>
          <strong>{availability.items.length - unansweredTimes.length} / {availability.items.length}</strong>
          <span>ваших ответов сохранено</span>
        </div>
        <div className="availability-filter" role="group" aria-label="Фильтр вариантов времени">
          <button
            aria-pressed={timeFilter === "all"}
            className={timeFilter === "all" ? "active" : ""}
            onClick={() => setTimeFilter("all")}
            type="button"
          >
            Все <span>{availability.items.length}</span>
          </button>
          <button
            aria-pressed={timeFilter === "unanswered"}
            className={timeFilter === "unanswered" ? "active" : ""}
            onClick={() => setTimeFilter("unanswered")}
            type="button"
          >
            Без ответа <span>{unansweredTimes.length}</span>
          </button>
        </div>
      </div>

      <div className="availability-result-list">
        {visibleTimes.map((option) => {
          const planTitle = option.plan_option_id
            ? meeting.plan_options.find((plan) => plan.id === option.plan_option_id)?.title
            : null;
          const participantCount = availability.participants.length;
          const answeredCount = participantCount - option.counts.unanswered;
          const participationPercent = participantCount === 0
            ? 0
            : Math.round((answeredCount / participantCount) * 100);
          const responseStatusByUser = new Map(
            option.responses.map((response) => [response.user_id, response.status]),
          );
          const participantsByStatus = new Map(
            availabilityStatusOrder.map((status) => [
              status,
              availability.participants.filter(
                (participant) => (responseStatusByUser.get(participant.user_id) ?? "unanswered") === status,
              ),
            ]),
          );
          const timeLabel = compactDate.format(new Date(option.starts_at));

          return (
            <article className="availability-result-card" aria-label={`Вариант времени ${timeLabel}`} key={option.id}>
              <header>
                <div>
                  <span>{planTitle ? `Для плана «${planTitle}»` : "Общее для всех планов"}</span>
                  <h3>{timeLabel}</h3>
                  <small>
                    {option.ends_at
                      ? `до ${compactDate.format(new Date(option.ends_at))}`
                      : "Без указанной длительности"}
                  </small>
                </div>
                <div className="availability-result-score">
                  <strong>{participationPercent}%</strong>
                  <span>{answeredCount} из {participantCount} ответили</span>
                  <small>вес {option.score > 0 ? `+${option.score}` : option.score}</small>
                </div>
              </header>

              <div
                aria-label={availabilityStatusOrder
                  .map((status) => `${availabilityLabels[status]}: ${option.counts[status]}`)
                  .join(", ")}
                className="availability-result-bar"
                role="img"
              >
                {availabilityStatusOrder.map((status) => {
                  const count = option.counts[status];
                  if (count === 0 || participantCount === 0) return null;
                  return (
                    <i
                      className={`status-${status}`}
                      key={status}
                      style={{ width: `${(count / participantCount) * 100}%` }}
                    />
                  );
                })}
              </div>

              <div className="availability-breakdown">
                {availabilityStatusOrder.map((status) => {
                  const count = option.counts[status];
                  const percent = participantCount === 0 ? 0 : Math.round((count / participantCount) * 100);
                  return (
                    <span className={`status-${status}`} key={status}>
                      <i aria-hidden="true" />
                      {availabilityLabels[status]}
                      <strong>{count}</strong>
                      <small>{percent}%</small>
                    </span>
                  );
                })}
              </div>

              <div className="availability-result-actions">
                <label>
                  <span>Ваш ответ</span>
                  <select
                    aria-label={`Ваша доступность: ${timeLabel}`}
                    className={`availability-select status-${option.my_status}`}
                    disabled={working || meeting.state !== "collecting"}
                    value={option.my_status}
                    onChange={(event) => void onRespond(
                      option.id,
                      event.currentTarget.value as AvailabilityStatus,
                    )}
                  >
                    {availabilityStatusOrder.map((value) => (
                      <option value={value} key={value}>{availabilityLabels[value]}</option>
                    ))}
                  </select>
                </label>
                <button
                  aria-expanded={expandedTimeID === option.id}
                  className="quiet-button"
                  onClick={() => setExpandedTimeID((current) => current === option.id ? "" : option.id)}
                  type="button"
                >
                  {expandedTimeID === option.id ? "Скрыть участников" : "Кто как ответил"}
                </button>
              </div>

              {expandedTimeID === option.id && (
                <div className="availability-response-groups">
                  {availabilityStatusOrder.map((status) => {
                    const participants = participantsByStatus.get(status) ?? [];
                    if (participants.length === 0) return null;
                    return (
                      <section key={status}>
                        <header>
                          <span>{availabilityLabels[status]}</span>
                          <strong>{participants.length}</strong>
                        </header>
                        <div>
                          {participants.map((participant) => (
                            <span className="participant-chip" key={participant.user_id}>
                              <i aria-hidden="true">{participant.display_name.slice(0, 1).toUpperCase()}</i>
                              {participant.display_name}
                              {participant.user_id === currentUserID && <small>вы</small>}
                            </span>
                          ))}
                        </div>
                      </section>
                    );
                  })}
                </div>
              )}
            </article>
          );
        })}
        {visibleTimes.length === 0 && (
          <p className="panel-empty">Вы уже ответили на все варианты времени.</p>
        )}
      </div>
    </section>
  );
}

function PollComposer({
  disabled,
  timeZone,
  onCreate,
}: {
  disabled: boolean;
  timeZone: string;
  onCreate: (input: CreatePollInput) => Promise<boolean>;
}) {
  const [question, setQuestion] = useState("");
  const [responseMode, setResponseMode] = useState<"single" | "multiple">("single");
  const [isAnonymous, setIsAnonymous] = useState(false);
  const [allowRevote, setAllowRevote] = useState(true);
  const [deadline, setDeadline] = useState("");
  const [deadlineError, setDeadlineError] = useState("");
  const [options, setOptions] = useState(["", ""]);
  const normalizedOptions = options.map((option) => option.trim()).filter(Boolean);
  const canSubmit = !disabled && question.trim().length > 0 && normalizedOptions.length >= 2;

  function updateOption(index: number, value: string) {
    setOptions((current) => current.map((option, optionIndex) =>
      optionIndex === index ? value : option,
    ));
  }

  function removeOption(index: number) {
    setOptions((current) => current.filter((_, optionIndex) => optionIndex !== index));
  }

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!canSubmit) return;
    let normalizedDeadline: string | null = null;
    try {
      normalizedDeadline = deadline ? zonedDateTimeToISO(deadline, timeZone) : null;
      setDeadlineError("");
    } catch (dateError) {
      setDeadlineError(
        dateError instanceof Error ? dateError.message : "Проверьте срок ответа.",
      );
      return;
    }
    const created = await onCreate({
      question: question.trim(),
      response_mode: responseMode,
      is_anonymous: isAnonymous,
      allow_revote: allowRevote,
      deadline: normalizedDeadline,
      options: normalizedOptions,
    });
    if (created) {
      setQuestion("");
      setResponseMode("single");
      setIsAnonymous(false);
      setAllowRevote(true);
      setDeadline("");
      setOptions(["", ""]);
    }
  }

  return (
    <form className="poll-create-form" onSubmit={(event) => void submit(event)}>
      <div className="poll-composer-heading">
        <div>
          <p className="section-kicker">НОВЫЙ ВОПРОС</p>
          <h3>Создать опрос</h3>
        </div>
        <span>{options.length} / 10 вариантов</span>
      </div>
      <label className="field poll-question-field">
        <span>Вопрос</span>
        <input
          autoComplete="off"
          maxLength={200}
          onChange={(event) => setQuestion(event.currentTarget.value)}
          placeholder="Например, что берём с собой?"
          required
          value={question}
        />
      </label>
      <label className="field">
        <span>Сколько вариантов можно выбрать</span>
        <select
          onChange={(event) => setResponseMode(event.currentTarget.value === "multiple" ? "multiple" : "single")}
          value={responseMode}
        >
          <option value="single">Только один вариант</option>
          <option value="multiple">Несколько вариантов</option>
        </select>
      </label>
      <div className="poll-setting-list">
        <label className="poll-setting">
          <span>
            <strong>Анонимный опрос</strong>
            <small>Скрывает имена и историю ответов</small>
          </span>
          <input
            checked={isAnonymous}
            onChange={(event) => setIsAnonymous(event.currentTarget.checked)}
            type="checkbox"
          />
        </label>
        <label className="poll-setting">
          <span>
            <strong>Разрешить переголосование</strong>
            <small>Ответ можно будет изменить или снять</small>
          </span>
          <input
            checked={allowRevote}
            onChange={(event) => setAllowRevote(event.currentTarget.checked)}
            type="checkbox"
          />
        </label>
      </div>
      <label className="field">
        <span>
          Принимать ответы до
          <small>необязательно · {meetingTimeZoneLabel(timeZone)}</small>
        </span>
        <input
          onChange={(event) => {
            setDeadline(event.currentTarget.value);
            setDeadlineError("");
          }}
          type="datetime-local"
          value={deadline}
        />
      </label>
      {deadlineError && (
        <p className="form-error poll-deadline-error" role="alert">{deadlineError}</p>
      )}
      <fieldset className="poll-option-editor">
        <legend>Варианты ответа</legend>
        <div>
          {options.map((option, index) => (
            <label key={index}>
              <span>{String(index + 1).padStart(2, "0")}</span>
              <input
                aria-label={`Вариант ответа ${index + 1}`}
                maxLength={120}
                onChange={(event) => updateOption(index, event.currentTarget.value)}
                placeholder={index === 0 ? "Первый вариант" : "Ещё один вариант"}
                required
                value={option}
              />
              <button
                aria-label={`Удалить вариант ${index + 1}`}
                disabled={disabled || options.length <= 2}
                onClick={() => removeOption(index)}
                type="button"
              >
                ×
              </button>
            </label>
          ))}
        </div>
        <button
          className="quiet-button"
          disabled={disabled || options.length >= 10}
          onClick={() => setOptions((current) => [...current, ""])}
          type="button"
        >
          + Добавить вариант
        </button>
      </fieldset>
      <div className="poll-composer-note">
        <p>
          {responseMode === "single"
            ? `Выбор сохраняется сразу${allowRevote ? " и может быть изменён" : " и станет окончательным"}.`
            : `Участник отмечает несколько вариантов и подтверждает набор${allowRevote ? ", затем сможет его изменить" : " один раз"}.`}
        </p>
        <button className="secondary-button" disabled={!canSubmit} type="submit">
          Создать опрос
        </button>
      </div>
    </form>
  );
}

function PollCard({
  poll,
  isOwner,
  meetingState,
  timeZone,
  working,
  onDelete,
  onLoadHistory,
  onVote,
  onClose,
}: {
  poll: Poll;
  isOwner: boolean;
  meetingState: Meeting["state"];
  timeZone: string;
  working: boolean;
  onDelete: () => Promise<void>;
  onLoadHistory: () => Promise<PollHistoryPage>;
  onVote: (optionIDs: string[]) => Promise<void>;
  onClose: (optionID: string | null) => Promise<void>;
}) {
  const savedSelected = poll.options
    .filter((option) => option.selected_by_user)
    .map((option) => option.id);
  const [selected, setSelected] = useState<string[]>(savedSelected);
  const [finalChoice, setFinalChoice] = useState(poll.selected_option_id ?? "");
  const [expandedVotersID, setExpandedVotersID] = useState("");
  const [showHistory, setShowHistory] = useState(false);
  const [history, setHistory] = useState<PollHistoryPage | null>(null);
  const [historyLoading, setHistoryLoading] = useState(false);
  const [historyError, setHistoryError] = useState("");

  useEffect(() => {
    setSelected(poll.options.filter((option) => option.selected_by_user).map((option) => option.id));
    setFinalChoice(poll.selected_option_id ?? "");
  }, [poll]);

  const deadlineText = poll.deadline
    ? createMeetingDateTimeFormatter(timeZone).format(new Date(poll.deadline))
    : "Без срока";
  const historyDateFormatter = createMeetingDateTimeFormatter(timeZone);
  const meetingAcceptsPolls = meetingState === "collecting" || meetingState === "scheduled";
  const deadlineExpired = typeof poll.deadline === "string" && new Date(poll.deadline).getTime() <= Date.now();
  const answerIsFinal = !poll.allow_revote && savedSelected.length > 0;
  const canAnswer = poll.accepting_answers && meetingAcceptsPolls && !answerIsFinal;
  const canManage = poll.can_manage && meetingAcceptsPolls && poll.state === "open";
  const participationPercent = poll.participant_count === 0
    ? 0
    : Math.round((poll.respondent_count / poll.participant_count) * 100);
  const normalizedSelected = [...selected].sort().join(",");
  const normalizedSaved = [...savedSelected].sort().join(",");
  const answerChanged = normalizedSelected !== normalizedSaved;

  async function loadHistory() {
    setHistoryLoading(true);
    setHistoryError("");
    try {
      setHistory(await onLoadHistory());
    } catch (error) {
      setHistoryError(errorMessage(error));
    } finally {
      setHistoryLoading(false);
    }
  }

  async function submitVote(optionIDs: string[]) {
    await onVote(optionIDs);
    if (showHistory) {
      await loadHistory();
    }
  }

  function toggle(optionID: string) {
    if (poll.response_mode === "single") {
      setSelected([optionID]);
      if (!savedSelected.includes(optionID)) {
        void submitVote([optionID]);
      }
      return;
    }
    setSelected((current) => current.includes(optionID)
      ? current.filter((id) => id !== optionID)
      : [...current, optionID]);
  }

  return (
    <article className={`poll-card poll-${poll.state}`}>
      <header>
        <div>
          <div className="poll-mode-list">
            <span className="poll-mode">{poll.response_mode === "single" ? "один ответ" : "несколько ответов"}</span>
            <span className="poll-mode">{poll.is_anonymous ? "анонимный" : "видны голоса"}</span>
            <span className="poll-mode">{poll.allow_revote ? "можно изменить" : "ответ окончательный"}</span>
          </div>
          <h3>{poll.question}</h3>
          <small>{deadlineText} · {poll.respondent_count} из {poll.participant_count} ответили</small>
        </div>
        {poll.state === "closed" && (
          <span className="poll-closed-label">
            {poll.selected_option_id ? "Итог закреплён" : "Опрос остановлен"}
          </span>
        )}
        {isOwner && meetingState === "draft" && (
          <button className="quiet-button" disabled={working} type="button" onClick={() => void onDelete()}>
            Удалить
          </button>
        )}
      </header>
      <div className="poll-participation">
        <span><i style={{ width: `${participationPercent}%` }} aria-hidden="true" /></span>
        <small>{participationPercent}% участия</small>
      </div>
      <div className="poll-option-list">
        {poll.options.map((option) => {
          const chosen = poll.selected_option_id === option.id;
          const selectedByUser = selected.includes(option.id);
          const percent = poll.respondent_count === 0
            ? 0
            : Math.round((option.vote_count / poll.respondent_count) * 100);
          return (
            <article
              className={`poll-option ${chosen ? "poll-option-final" : ""} ${selectedByUser ? "poll-option-selected" : ""}`}
              key={option.id}
            >
              <label className="poll-option-choice">
                {canAnswer ? (
                  <input
                    type={poll.response_mode === "single" ? "radio" : "checkbox"}
                    name={`poll-${poll.id}`}
                    checked={selectedByUser}
                    disabled={working}
                    onChange={() => toggle(option.id)}
                  />
                ) : (
                  <span className="poll-input-spacer" aria-hidden="true" />
                )}
                <span className="poll-option-body">
                  <span className="poll-option-copy">
                    <strong>{option.label}</strong>
                    <small>{option.vote_count} {option.vote_count === 1 ? "голос" : "голосов"}</small>
                  </span>
                  <span className="poll-result-track"><i style={{ width: `${percent}%` }} aria-hidden="true" /></span>
                </span>
                <strong className="poll-percent">{percent}%</strong>
                <span className="poll-option-flags">
                  {selectedByUser && <small>ваш выбор</small>}
                  {chosen && <small>итог</small>}
                </span>
              </label>
              {!poll.is_anonymous ? (
              <div className="result-voters poll-result-voters">
                <button
                  aria-expanded={expandedVotersID === option.id}
                  disabled={option.voters.length === 0}
                  onClick={() => setExpandedVotersID((current) => current === option.id ? "" : option.id)}
                  type="button"
                >
                  {option.voters.length === 0
                    ? "Пока без голосов"
                    : `Кто выбрал · ${option.voters.length}`}
                </button>
                {expandedVotersID === option.id && option.voters.length > 0 && (
                  <div className="poll-voters">
                    {option.voters.map((voter) => (
                      <span title={voter.display_name} key={voter.user_id}>
                        <i aria-hidden="true">{voter.display_name.slice(0, 1).toUpperCase()}</i>
                        {voter.display_name}
                      </span>
                    ))}
                  </div>
                )}
              </div>
              ) : (
                <p className="poll-anonymous-note">Имена проголосовавших скрыты</p>
              )}
            </article>
          );
        })}
      </div>
      {(canAnswer || canManage) && (
        <div className={`poll-actions ${!canAnswer ? "poll-actions-owner-only" : ""}`}>
          {canAnswer && (
            <div className="poll-answer-actions">
            {poll.response_mode === "multiple" && (
              <button
                className="secondary-button"
                disabled={working || !answerChanged}
                type="button"
                onClick={() => void submitVote(selected)}
              >
                Сохранить выбранное · {selected.length}
              </button>
            )}
            {savedSelected.length > 0 && (
              <button
                className="quiet-button"
                disabled={working}
                type="button"
                onClick={() => {
                  setSelected([]);
                  void submitVote([]);
                }}
              >
                Снять свой ответ
              </button>
            )}
            {poll.response_mode === "single" && savedSelected.length === 0 && (
              <small>Нажмите на вариант — ответ сохранится сразу.</small>
            )}
            </div>
          )}
          {canManage && (
            <div className="poll-owner-decision">
              <label>
                <span>Закрепить вариант <small>необязательно</small></span>
                <select aria-label={`Итог опроса «${poll.question}»`} value={finalChoice} onChange={(event) => setFinalChoice(event.target.value)}>
                  <option value="">Выберите итог…</option>
                  {poll.options.map((option) => <option value={option.id} key={option.id}>{option.label}</option>)}
                </select>
              </label>
              <div className="poll-owner-actions">
                <button
                  className="primary-button compact"
                  disabled={working || !finalChoice}
                  type="button"
                  onClick={() => void onClose(finalChoice)}
                >
                  Закрепить итог
                </button>
                <button
                  className="quiet-button"
                  disabled={working}
                  type="button"
                  onClick={() => void onClose(null)}
                >
                  Остановить без итога
                </button>
              </div>
              <small>Оба действия закрывают ответы. Закреплённый вариант будет отмечен отдельно.</small>
            </div>
          )}
        </div>
      )}
      {!poll.is_anonymous && <div className="poll-history">
        <button
          aria-expanded={showHistory}
          className="poll-history-toggle"
          disabled={historyLoading}
          onClick={() => {
            if (showHistory) {
              setShowHistory(false);
              return;
            }
            setShowHistory(true);
            if (!history) void loadHistory();
          }}
          type="button"
        >
          <span>{showHistory ? "Скрыть историю ответов" : "История ответов"}</span>
          <strong>{history ? history.total : "→"}</strong>
        </button>
        {showHistory && (
          <div aria-label={`История ответов «${poll.question}»`} className="poll-history-panel" role="region">
            {historyLoading && !history && <p className="panel-empty">Загружаем изменения…</p>}
            {historyError && (
              <div className="poll-history-error">
                <p>{historyError}</p>
                <button className="quiet-button" onClick={() => void loadHistory()} type="button">
                  Повторить
                </button>
              </div>
            )}
            {history && history.items.length === 0 && (
              <p className="panel-empty">Ответы ещё не менялись.</p>
            )}
            {history && history.items.length > 0 && (
              <ol className="poll-history-list">
                {history.items.map((entry) => {
                  const previous = entry.previous_option_labels.join(", ");
                  const next = entry.new_option_labels.join(", ");
                  const text = entry.action === "cast"
                    ? `выбрал: ${next}`
                    : entry.action === "retract"
                      ? `отозвал ответ: ${previous}`
                      : `изменил: ${previous} → ${next}`;
                  return (
                    <li key={entry.id}>
                      <span aria-hidden="true">{entry.display_name.slice(0, 1).toUpperCase()}</span>
                      <div>
                        <strong>{entry.display_name}</strong>
                        <p>{text}</p>
                        <time dateTime={entry.created_at}>
                          {historyDateFormatter.format(new Date(entry.created_at))}
                        </time>
                      </div>
                    </li>
                  );
                })}
              </ol>
            )}
            {history && history.total > history.items.length && (
              <p className="poll-history-note">
                Показаны последние {history.items.length} из {history.total} изменений.
              </p>
            )}
          </div>
        )}
      </div>}
      {answerIsFinal && poll.state === "open" && (
        <p className="poll-readonly-note poll-answer-final-note">
          Ваш ответ принят. Автор опроса отключил переголосование, поэтому изменить его нельзя.
        </p>
      )}
      {!canAnswer && !answerIsFinal && poll.state === "open" && (
        <p className="poll-readonly-note">
          {deadlineExpired
            ? "Срок ответа прошёл. Автор опроса или организатор может подвести итог."
            : "В этой встрече ответы уже закрыты."}
        </p>
      )}
      {poll.state === "closed" && !poll.selected_option_id && (
        <p className="poll-readonly-note">
          Опрос остановлен без закреплённого варианта; результаты сохранены.
        </p>
      )}
    </article>
  );
}

function newInvitationSecret(): string {
  const bytes = new Uint8Array(32);
  crypto.getRandomValues(bytes);
  let binary = "";
  bytes.forEach((value) => {
    binary += String.fromCharCode(value);
  });
  return btoa(binary).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/u, "");
}
