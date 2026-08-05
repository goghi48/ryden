import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { App } from "./App";
import type { Meeting, TimeOption } from "./api";

describe("App", () => {
  beforeEach(() => {
    cleanup();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
    localStorage.clear();
    vi.stubGlobal(
      "matchMedia",
      vi.fn().mockReturnValue({
        matches: false,
        addEventListener: vi.fn(),
        removeEventListener: vi.fn(),
      }),
    );
    Object.defineProperty(URL, "createObjectURL", {
      configurable: true,
      value: vi.fn(() => "blob:test-photo"),
    });
    Object.defineProperty(URL, "revokeObjectURL", {
      configurable: true,
      value: vi.fn(),
    });
  });

  it("shows the private account entry after session bootstrap fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue(
        new Response(JSON.stringify({ error: { code: "unauthorized", message: "Нет сессии" } }), {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );

    render(<App />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /встреча начинается/i })).toBeInTheDocument();
    });
    expect(screen.getByRole("tab", { name: "Создать аккаунт" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
  });

  it("updates the current profile from the header editor", async () => {
    let profileBody: { display_name: string; nickname: string; avatar_url: string | null } | null = null;
    const user = {
      id: "user-1",
      email: "owner@example.test",
      display_name: "Owner",
      nickname: "owner",
      avatar_url: null,
      avatar_revision: null,
    };
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "test-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [], limit: 50, offset: 0 }));
      }
      if (url === "/api/v1/me" && method === "PUT") {
        profileBody = JSON.parse(String(init?.body));
        return Promise.resolve(json({
          ...user,
          display_name: profileBody?.display_name,
          avatar_url: profileBody?.avatar_url,
        }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    expect(await screen.findByLabelText("Сортировка встреч")).toHaveValue("start");
    expect(screen.queryByText("Сначала", { exact: true })).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: "Редактировать профиль" }));
    expect(screen.getByRole("dialog", { name: "Как вас видит группа" })).toBeInTheDocument();

    fireEvent.change(screen.getByLabelText("Отображаемое имя"), {
      target: { value: "Анна Р." },
    });
    expect(screen.getByRole("button", { name: "Выбрать фото" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Сохранить профиль/ }));

    await waitFor(() => {
      expect(screen.queryByRole("dialog", { name: "Как вас видит группа" })).not.toBeInTheDocument();
    });
    expect(profileBody).toEqual({
      display_name: "Анна Р.",
      nickname: "owner",
      avatar_url: null,
    });
    expect(screen.getByText("Анна Р.")).toBeInTheDocument();
  });

  it("shows incoming meeting invitations in the header and lets the invitee decline", async () => {
    const methods: string[] = [];
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      methods.push(`${method} ${url}`);
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user: {
            id: "invitee-1",
            email: "invitee@example.test",
            display_name: "Ира",
            nickname: "ira",
            avatar_url: null,
            avatar_revision: null,
          },
          access_token: "invitee-token",
          access_token_expires_at: "2026-08-04T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [], limit: 50, offset: 0 }));
      }
      if (url.startsWith("/api/v1/me/meeting-invitations?") && method === "GET") {
        return Promise.resolve(json({
          items: [{
            id: "invite-1",
            meeting_id: "meeting-1",
            meeting_title: "Ужин",
            owner_display_name: "Анна",
            starts_at: "2026-08-10T12:00:00Z",
            ends_at: null,
            timezone: "Asia/Novosibirsk",
            created_at: "2026-08-03T10:00:00Z",
          }],
          total: 1,
          limit: 50,
          offset: 0,
        }));
      }
      if (url === "/api/v1/me/meeting-invitations/invite-1" && method === "DELETE") {
        return Promise.resolve(json({ meeting_id: "meeting-1", changed: true, joined: false }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    const invitesButton = await screen.findByRole("button", { name: "Приглашения на встречи: 1" });
    fireEvent.click(invitesButton);
    expect(screen.getByRole("dialog", { name: "Вас зовут на встречу" })).toBeInTheDocument();
    expect(screen.getByText("Ужин")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Отказаться" }));

    await waitFor(() => expect(screen.getByText("Новых приглашений нет")).toBeInTheDocument());
    expect(methods).toContain("DELETE /api/v1/me/meeting-invitations/invite-1");
  });

  it("creates and displays a meeting with an optional duration and external location", async () => {
    let createBody: {
      event_type: string;
      coordination_mode: "planning" | "fixed";
      cover_url: string | null;
      location_name: string | null;
      location_url: string | null;
      starts_at: string;
      ends_at: string | null;
    } | null = null;
    let updateBody: {
      title: string;
      description: string;
      event_type: string;
      cover_url: string | null;
      location_name: string | null;
      location_url: string | null;
      version: number;
    } | null = null;
    let planUpdateBody: {
      title: string;
      description: string;
      version: number;
    } | null = null;
    let timeUpdateBody: {
      plan_option_id: string | null;
      starts_at: string;
      ends_at: string | null;
      version: number;
    } | null = null;
    let uploadedMeetingPhoto: BodyInit | null = null;
    let invitationCreated = false;
    const user = {
      id: "user-1",
      email: "owner@example.test",
      display_name: "Owner",
      avatar_url: null,
    };
    const meeting: Meeting = {
      id: "meeting-location",
      owner_id: user.id,
      title: "Игровой вечер",
      description: "Берём любимые игры",
      event_type: "other",
      coordination_mode: "planning",
      cover_url: null,
      has_photo: false,
      location_name: "Дом Анны",
      location_url: "https://maps.example.test/anna",
      timezone: "Asia/Novosibirsk",
      state: "draft",
      version: 1,
      participant_role: "owner",
      created_at: "2026-07-29T08:00:00Z",
      updated_at: "2026-07-29T08:00:00Z",
    };
    let meetingView = { ...meeting };
    let planOptions = Array.from({ length: 7 }, (_, index) => ({
      id: `plan-${index + 1}`,
      title: `План ${index + 1}`,
      description: "",
      position: index,
      created_at: meeting.created_at,
    }));
    let timeOptions: TimeOption[] = Array.from({ length: 7 }, (_, index) => ({
      id: `time-${index + 1}`,
      starts_at: `2026-08-05T${String(index + 10).padStart(2, "0")}:00:00Z`,
      ends_at: `2026-08-05T${String(index + 11).padStart(2, "0")}:00:00Z`,
      position: index,
      created_at: meeting.created_at,
    }));
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "test-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [], limit: 50, offset: 0 }));
      }
      if (url === "/api/v1/meetings" && method === "POST") {
        createBody = JSON.parse(String(init?.body));
        return Promise.resolve(json(meeting, 201));
      }
      if (url === "/api/v1/meetings/meeting-location/photo?version=1" && method === "PUT") {
        uploadedMeetingPhoto = init?.body ?? null;
        meetingView = {
          ...meetingView,
          has_photo: true,
          version: 2,
          updated_at: "2026-07-29T08:02:00Z",
        };
        return Promise.resolve(json({
          version: 2,
          changed: true,
          updated_at: meetingView.updated_at,
        }));
      }
      if (url === "/api/v1/meetings/meeting-location/invitations" && method === "POST") {
        invitationCreated = true;
        return Promise.resolve(json({ expires_at: "2026-08-05T08:00:00Z" }, 201));
      }
      if (url.startsWith("/api/v1/meetings/meeting-location/friend-invitations?") && method === "GET") {
        return Promise.resolve(json({
          items: [
            {
              user_id: "friend-dima",
              nickname: "dima",
              display_name: "Дима",
              avatar_url: null,
              avatar_revision: null,
              invitation_id: null,
              invitation_status: null,
              is_participant: false,
            },
            {
              user_id: "friend-lena",
              nickname: "lena",
              display_name: "Лена",
              avatar_url: null,
              avatar_revision: null,
              invitation_id: null,
              invitation_status: null,
              is_participant: false,
            },
          ],
          total: 2,
          limit: 50,
          offset: 0,
        }));
      }
      if (url === "/api/v1/meetings/meeting-location" && method === "PUT") {
        updateBody = JSON.parse(String(init?.body));
        meetingView = {
          ...meetingView,
          ...updateBody,
          version: meetingView.version + 1,
          updated_at: "2026-07-29T08:05:00Z",
        };
        return Promise.resolve(json(meetingView));
      }
      if (url === "/api/v1/meetings/meeting-location/plan-options/plan-1" && method === "PUT") {
        planUpdateBody = JSON.parse(String(init?.body));
        planOptions = planOptions.map((option) => option.id === "plan-1"
          ? { ...option, title: planUpdateBody!.title, description: planUpdateBody!.description }
          : option);
        meetingView = { ...meetingView, version: meetingView.version + 1 };
        return Promise.resolve(json(planOptions[0]));
      }
      if (url === "/api/v1/meetings/meeting-location/time-options/time-1" && method === "PUT") {
        timeUpdateBody = JSON.parse(String(init?.body));
        timeOptions = timeOptions.map((option) => option.id === "time-1"
          ? {
            ...option,
            plan_option_id: timeUpdateBody!.plan_option_id ?? undefined,
            starts_at: timeUpdateBody!.starts_at,
            ends_at: timeUpdateBody!.ends_at,
          }
          : option);
        meetingView = { ...meetingView, version: meetingView.version + 1 };
        return Promise.resolve(json(timeOptions[0]));
      }
      if (url === "/api/v1/meetings/meeting-location") {
        return Promise.resolve(json({
          ...meetingView,
          plan_options: planOptions,
          time_options: timeOptions,
          participants: [{
            user_id: user.id,
            display_name: user.display_name,
            role: "owner",
            joined_at: meeting.created_at,
          }],
        }));
      }
      if (url === "/api/v1/meetings/meeting-location/polls") {
        return Promise.resolve(json({ items: [] }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-location/notes?")) {
        return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-location/availability") {
        return Promise.resolve(json({
          weights: { preferred: 3, available: 2, if_needed: 1, unavailable: -4, unanswered: 0 },
          participants: [],
          items: [],
          recommendations: [],
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-location/plan-votes")) {
        return Promise.resolve(json({
          options: [],
          responses: [],
          history: [],
          participant_count: 1,
          answered_count: 0,
          history_total: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-location/requirements")) {
        return Promise.resolve(json({
          items: [],
          total: 0,
          open_count: 0,
          completed_count: 0,
          limit: 50,
          offset: 0,
        }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    expect(await screen.findByRole("link", { name: "На главную Ryden" })).toHaveAttribute("href", "/");
    expect(screen.queryByText("owner@example.test")).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: /Создать встречу/ }));
    fireEvent.change(screen.getByLabelText("Название встречи"), {
      target: { value: meeting.title },
    });
    fireEvent.change(screen.getByLabelText(/Описание/), {
      target: { value: meeting.description },
    });
    fireEvent.change(screen.getByLabelText("Дата"), {
      target: { value: "2026-08-05" },
    });
    fireEvent.change(screen.getByLabelText("Время"), {
      target: { value: "18:00" },
    });
    const durationDays = screen.getByLabelText("Дни");
    fireEvent.input(durationDays, { target: { value: "02" } });
    expect(durationDays).toHaveValue(2);
    fireEvent.change(durationDays, { target: { value: "31" } });
    fireEvent.change(screen.getByLabelText("Часы"), { target: { value: "23" } });
    fireEvent.change(screen.getByLabelText("Минуты"), { target: { value: "59" } });
    const createForm = screen.getByRole("dialog", { name: "Создать встречу" }).querySelector("form");
    if (!createForm) throw new Error("create meeting form not found");
    fireEvent.submit(createForm);
    expect(await screen.findByText("Укажите не больше 30 дней, 23 часов и 59 минут.")).toBeInTheDocument();
    expect(screen.queryByText("Связь прервалась. Попробуйте ещё раз.")).not.toBeInTheDocument();
    fireEvent.change(durationDays, { target: { value: "30" } });
    const closeBitmap = vi.fn();
    vi.stubGlobal("createImageBitmap", vi.fn().mockResolvedValue({
      width: 1600,
      height: 900,
      close: closeBitmap,
    }));
    vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue({
      drawImage: vi.fn(),
      fillRect: vi.fn(),
      fillStyle: "",
    } as unknown as CanvasRenderingContext2D);
    vi.spyOn(HTMLCanvasElement.prototype, "toBlob").mockImplementation((callback) => {
      callback(new Blob(["compressed-photo"], { type: "image/jpeg" }));
    });
    const meetingPhoto = new File(["safe-photo"], "meeting.png", { type: "image/png" });
    fireEvent.change(screen.getByLabelText(/Фото встречи/), {
      target: { files: [meetingPhoto] },
    });
    expect(await screen.findByRole("group", { name: "Область кадрирования" })).toBeInTheDocument();
    expect(screen.queryByRole("slider")).not.toBeInTheDocument();
    fireEvent.click(await screen.findByRole("button", { name: "Использовать этот кадр" }));
    await screen.findByText("Кадр готов");
    fireEvent.change(screen.getByLabelText(/^Место/), {
      target: { value: meeting.location_name },
    });
    fireEvent.change(screen.getByLabelText(/Ссылка на место/), {
      target: { value: meeting.location_url },
    });
    fireEvent.click(screen.getByRole("button", { name: /Создать и пригласить/ }));

    await screen.findByRole("heading", { name: meeting.title });
    expect(screen.getByRole("region", { name: "Краткая информация о встрече" })).toBeInTheDocument();
    expect(createBody).toMatchObject({
      event_type: "other",
      coordination_mode: "fixed",
      cover_url: null,
      location_name: meeting.location_name,
      location_url: meeting.location_url,
      starts_at: expect.stringMatching(/^2026-08-05T/),
      ends_at: expect.any(String),
    });
    expect(
      new Date(createBody!.ends_at as string).getTime()
      - new Date(createBody!.starts_at).getTime(),
    ).toBe((30 * 24 * 60 + 23 * 60 + 59) * 60_000);
    const uploadedMeetingFile: unknown = uploadedMeetingPhoto;
    expect(uploadedMeetingFile).toBeInstanceOf(File);
    if (!(uploadedMeetingFile instanceof File)) throw new Error("meeting photo was not uploaded");
    expect(uploadedMeetingFile.type).toBe("image/jpeg");
    expect(invitationCreated).toBe(true);
    expect(closeBitmap).toHaveBeenCalledOnce();
    fireEvent.click(screen.getByRole("button", { name: "Поделиться встречей" }));
    expect(screen.getByRole("dialog", { name: "Поделиться встречей" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Скопировать ссылку" })).toBeInTheDocument();
    const dimaInvite = await screen.findByRole("checkbox", { name: "Дима @dima Можно пригласить" });
    const lenaInvite = screen.getByRole("checkbox", { name: "Лена @lena Можно пригласить" });
    fireEvent.click(dimaInvite);
    fireEvent.click(lenaInvite);
    expect(dimaInvite).toBeChecked();
    expect(lenaInvite).toBeChecked();
    expect(screen.getByRole("button", { name: "Пригласить · 2" })).toBeEnabled();
    fireEvent.click(screen.getByRole("button", { name: "Закрыть приглашение" }));
    fireEvent.click(screen.getByRole("button", { name: "Управление встречей" }));
    expect(screen.getByText("Заменить фото встречи")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    expect(screen.getByRole("link", { name: /Дом Анны/ })).toHaveAttribute(
      "href",
      meeting.location_url,
    );
    fireEvent.click(screen.getByRole("button", { name: /Варианты 7 вариантов плана/ }));
    const planPanel = screen.getByRole("article", { name: "Варианты плана" });
    expect(within(planPanel).queryByText("План 6")).not.toBeInTheDocument();
    fireEvent.click(within(planPanel).getByRole("button", { name: "Показать все · 7" }));
    expect(within(planPanel).getByText("План 6")).toBeInTheDocument();

    const timePanel = screen.getByRole("article", { name: "Варианты времени" });
    const showAllTimes = within(timePanel).getByRole("button", { name: "Показать все · 7" });
    expect(showAllTimes).toHaveAttribute("aria-expanded", "false");
    fireEvent.click(showAllTimes);
    expect(showAllTimes).toHaveAttribute("aria-expanded", "true");

    fireEvent.click(within(planPanel).getAllByRole("button", { name: "Изменить" })[0]);
    fireEvent.change(within(planPanel).getByLabelText("Название варианта"), {
      target: { value: "План с пиццей" },
    });
    fireEvent.change(within(planPanel).getByLabelText(/Пояснение/), {
      target: { value: "Заказываем заранее" },
    });
    fireEvent.click(within(planPanel).getByRole("button", { name: "Сохранить план" }));
    await within(planPanel).findByText("План с пиццей");
    expect(planUpdateBody).toEqual({
      title: "План с пиццей",
      description: "Заказываем заранее",
      version: 2,
    });

    fireEvent.click(within(timePanel).getAllByRole("button", { name: "Изменить" })[0]);
    fireEvent.change(within(timePanel).getByLabelText("К какому плану относится"), {
      target: { value: "plan-1" },
    });
    fireEvent.change(within(timePanel).getByLabelText("Дата"), {
      target: { value: "2026-08-06" },
    });
    fireEvent.change(within(timePanel).getByLabelText("Время"), {
      target: { value: "18:00" },
    });
    fireEvent.change(within(timePanel).getByLabelText("Часы"), {
      target: { value: "3" },
    });
    fireEvent.click(within(timePanel).getByRole("button", { name: "Сохранить время" }));
    await within(timePanel).findByText("Для плана: План с пиццей");
    expect(timeUpdateBody).toMatchObject({
      plan_option_id: "plan-1",
      starts_at: "2026-08-06T11:00:00.000Z",
      ends_at: "2026-08-06T14:00:00.000Z",
      version: 3,
    });

    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    fireEvent.click(screen.getByRole("button", { name: "Управление встречей" }));
    fireEvent.change(screen.getByLabelText("Название встречи"), {
      target: { value: "Игры и пицца" },
    });
    fireEvent.change(screen.getByLabelText(/^Место/), {
      target: { value: "У Анны" },
    });
    fireEvent.change(screen.getByLabelText(/Ссылка на место/), {
      target: { value: "" },
    });
    fireEvent.click(screen.getByRole("button", { name: /Сохранить изменения/ }));

    await screen.findByRole("heading", { name: "Игры и пицца" });
    expect(updateBody).toMatchObject({
      title: "Игры и пицца",
      event_type: "other",
      location_name: "У Анны",
      location_url: null,
      version: 4,
    });
    expect(screen.queryByRole("link", { name: /У Анны/ })).not.toBeInTheDocument();
    expect(screen.getByText("После сохранения изменения увидят все участники.")).toBeInTheDocument();
  });

  it("shows transparent poll results and saves a single choice immediately", async () => {
    let voted = false;
    let closed = false;
    let historyRequests = 0;
    let voteBody: { option_ids: string[] } | null = null;
    let closeBody: { selected_option_id: string | null } | null = null;
    let createPollBody: Record<string, unknown> | null = null;
    const user = {
      id: "user-1",
      email: "owner@example.test",
      display_name: "Анна",
      avatar_url: null,
    };
    const meeting = {
      id: "meeting-poll",
      owner_id: user.id,
      title: "Пикник",
      description: "Собираем решения",
      event_type: "other",
      timezone: "Asia/Novosibirsk",
      state: "collecting",
      version: 2,
      participant_role: "owner",
      created_at: "2026-07-29T08:00:00Z",
      updated_at: "2026-07-29T08:00:00Z",
    };
    const participants = [
      { user_id: user.id, display_name: user.display_name, role: "owner", joined_at: meeting.created_at },
      ...Array.from({ length: 9 }, (_, index) => ({
        user_id: `user-${index + 2}`,
        display_name: index === 0 ? "Борис" : `Участник ${index + 2}`,
        role: "participant",
        joined_at: meeting.created_at,
      })),
    ];
    const timeOption = {
      id: "time-1",
      plan_option_id: undefined,
      starts_at: "2026-08-05T11:00:00Z",
      ends_at: "2026-08-05T13:00:00Z",
      position: 0,
    };
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
    const pollPage = () => ({
      items: [{
        id: "poll-1",
        created_by_user_id: user.id,
        question: "Что берём?",
        response_mode: "single",
        is_anonymous: false,
        allow_revote: true,
        can_manage: true,
        state: closed ? "closed" : "open",
        accepting_answers: !closed,
        participant_count: 10,
        respondent_count: voted ? 2 : 1,
        total_selections: voted ? 2 : 1,
        created_at: "2026-07-29T08:00:00Z",
        ...(closed ? { closed_at: "2026-07-29T08:20:00Z" } : {}),
        options: [
          {
            id: "option-water",
            label: "Воду",
            position: 0,
            vote_count: voted ? 1 : 0,
            selected_by_user: voted,
            voters: voted ? [{
              user_id: user.id,
              display_name: user.display_name,
              updated_at: "2026-07-29T08:10:00Z",
            }] : [],
          },
          {
            id: "option-blanket",
            label: "Плед",
            position: 1,
            vote_count: 1,
            selected_by_user: false,
            voters: [{
              user_id: "user-2",
              display_name: "Борис",
              updated_at: "2026-07-29T08:05:00Z",
            }],
          },
        ],
      }],
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "test-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [meeting], limit: 20, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-poll") {
        return Promise.resolve(json({
          ...meeting,
          plan_options: [],
          time_options: [timeOption],
          participants,
        }));
      }
      if (url === "/api/v1/meetings/meeting-poll/polls" && method === "POST") {
        createPollBody = JSON.parse(String(init?.body));
        return Promise.resolve(json(pollPage().items[0], 201));
      }
      if (url === "/api/v1/meetings/meeting-poll/polls") {
        return Promise.resolve(json(pollPage()));
      }
      if (url.startsWith("/api/v1/meetings/meeting-poll/notes?")) {
        return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0 }));
      }
      if (url === "/api/v1/polls/poll-1/vote" && method === "PUT") {
        voteBody = JSON.parse(String(init?.body));
        voted = true;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (url.startsWith("/api/v1/polls/poll-1/history?")) {
        historyRequests += 1;
        return Promise.resolve(json({
          items: [
            ...(voted ? [{
              id: "poll-history-2",
              user_id: user.id,
              display_name: user.display_name,
              action: "cast",
              previous_option_ids: [],
              previous_option_labels: [],
              new_option_ids: ["option-water"],
              new_option_labels: ["Воду"],
              created_at: "2026-07-29T08:10:00Z",
            }] : []),
            {
              id: "poll-history-1",
              user_id: "user-2",
              display_name: "Борис",
              action: "cast",
              previous_option_ids: [],
              previous_option_labels: [],
              new_option_ids: ["option-blanket"],
              new_option_labels: ["Плед"],
              created_at: "2026-07-29T08:05:00Z",
            },
          ],
          total: voted ? 2 : 1,
          limit: 50,
          offset: 0,
        }));
      }
      if (url === "/api/v1/meetings/meeting-poll/polls/poll-1/close" && method === "POST") {
        closeBody = JSON.parse(String(init?.body));
        closed = true;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (url === "/api/v1/meetings/meeting-poll/availability") {
        return Promise.resolve(json({
          weights: { preferred: 3, available: 2, if_needed: 1, unavailable: -4, unanswered: 0 },
          participants,
          items: [{
            ...timeOption,
            my_status: "preferred",
            counts: { preferred: 3, available: 3, if_needed: 1, unavailable: 1, unanswered: 2 },
            responses: [
              { user_id: "user-1", status: "preferred" },
              { user_id: "user-2", status: "available" },
              { user_id: "user-3", status: "preferred" },
              { user_id: "user-4", status: "available" },
              { user_id: "user-5", status: "if_needed" },
              { user_id: "user-6", status: "unavailable" },
              { user_id: "user-7", status: "preferred" },
              { user_id: "user-8", status: "available" },
            ],
            score: 12,
          }],
          recommendations: [],
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-poll/plan-votes")) {
        return Promise.resolve(json({
          options: [],
          responses: [],
          history: [],
          participant_count: 2,
          answered_count: 0,
          history_total: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-poll/requirements")) {
        return Promise.resolve(json({
          items: [],
          total: 0,
          open_count: 0,
          completed_count: 0,
          limit: 50,
          offset: 0,
        }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: /Пикник/ }));
    await screen.findByRole("heading", { name: "Пикник" });
    expect(screen.queryByRole("heading", { name: "Опросы" })).not.toBeInTheDocument();

    expect(screen.queryByRole("button", { name: /Участники 10 человек/ })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Время 1 вариант/ }));
    await screen.findByRole("heading", { name: "Доступность" });
    expect(screen.queryByRole("table")).not.toBeInTheDocument();
    const timeResult = screen.getByRole("article", { name: /Вариант времени/ });
    expect(within(timeResult).getByText(/18:00 GMT\+7/i)).toBeInTheDocument();
    expect(within(timeResult).getByText("80%")).toBeInTheDocument();
    expect(within(timeResult).getByText("8 из 10 ответили")).toBeInTheDocument();
    fireEvent.click(within(timeResult).getByRole("button", { name: "Кто как ответил" }));
    expect(within(timeResult).getByText("Борис")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    fireEvent.click(screen.getByRole("button", { name: /Опросы 1 опрос/ }));
    await screen.findByRole("heading", { name: "Все опросы" });
    fireEvent.click(screen.getByRole("button", { name: "+ Новый опрос" }));
    expect(screen.getByRole("heading", { name: "Создать опрос" })).toBeInTheDocument();
    expect(screen.getByRole("checkbox", { name: /Анонимный опрос/ })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: /Разрешить переголосование/ })).toBeChecked();
    fireEvent.click(screen.getByRole("checkbox", { name: /Анонимный опрос/ }));
    fireEvent.click(screen.getByRole("checkbox", { name: /Разрешить переголосование/ }));
    fireEvent.change(screen.getByLabelText("Вопрос"), { target: { value: "Когда заказывать еду?" } });
    fireEvent.change(screen.getByLabelText("Вариант ответа 1"), { target: { value: "Утром" } });
    fireEvent.change(screen.getByLabelText("Вариант ответа 2"), { target: { value: "Вечером" } });
    fireEvent.click(screen.getByRole("button", { name: "Создать опрос" }));
    await waitFor(() => expect(createPollBody).toMatchObject({
      question: "Когда заказывать еду?",
      response_mode: "single",
      is_anonymous: true,
      allow_revote: false,
      options: ["Утром", "Вечером"],
    }));
    expect(screen.queryByText("Борис")).not.toBeInTheDocument();
    expect(screen.getByText("10% участия")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Кто выбрал · 1" }));
    expect(screen.getAllByText("Борис").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("radio", { name: /Воду/ }));

    await waitFor(() => {
      expect(voteBody).toEqual({ option_ids: ["option-water"] });
      expect(screen.getByText("20% участия")).toBeInTheDocument();
    });
    expect(screen.getAllByText("Анна").length).toBeGreaterThan(0);
    expect(historyRequests).toBe(0);
    fireEvent.click(screen.getByRole("button", { name: /История ответов/ }));
    const pollHistory = await screen.findByRole("region", { name: "История ответов «Что берём?»" });
    expect(historyRequests).toBe(1);
    expect(within(pollHistory).getByText("выбрал: Воду")).toBeInTheDocument();
    expect(within(pollHistory).getByText("выбрал: Плед")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Остановить без итога" }));
    await waitFor(() => {
      expect(closeBody).toEqual({ selected_option_id: null });
      expect(screen.getByText("Опрос остановлен")).toBeInTheDocument();
    });
    expect(screen.getByText(/без закреплённого варианта/)).toBeInTheDocument();
  });

  it("shows decision evidence and requires an explicit review before scheduling", async () => {
    let scheduled = false;
    let decisionBody: { plan_option_id: string; time_option_id: string } | null = null;
    const user = {
      id: "user-1",
      email: "owner@example.test",
      display_name: "Анна",
      avatar_url: null,
    };
    const meeting = {
      id: "meeting-decision",
      owner_id: user.id,
      title: "Большой пикник",
      description: "Проверяем итог перед подтверждением",
      event_type: "other",
      timezone: "Asia/Novosibirsk",
      state: "collecting",
      version: 4,
      participant_role: "owner",
      created_at: "2026-07-29T08:00:00Z",
      updated_at: "2026-07-29T08:00:00Z",
    };
    const participants = [
      { user_id: user.id, display_name: user.display_name, role: "owner", joined_at: meeting.created_at },
      { user_id: "user-2", display_name: "Борис", role: "participant", joined_at: meeting.created_at },
      { user_id: "user-3", display_name: "Вера", role: "participant", joined_at: meeting.created_at },
      { user_id: "user-4", display_name: "Глеб", role: "participant", joined_at: meeting.created_at },
    ];
    const plans = [
      {
        id: "plan-park",
        title: "Пикник в парке",
        description: "Берём пледы и еду",
        position: 0,
        created_at: meeting.created_at,
      },
      {
        id: "plan-home",
        title: "Вечер дома",
        description: "Запасной вариант",
        position: 1,
        created_at: meeting.created_at,
      },
    ];
    const times = [
      {
        id: "time-park",
        plan_option_id: null,
        starts_at: "2026-08-05T11:00:00Z",
        ends_at: "2026-08-05T14:00:00Z",
        position: 0,
        created_at: meeting.created_at,
      },
      {
        id: "time-home",
        plan_option_id: "plan-home",
        starts_at: "2026-08-05T13:00:00Z",
        ends_at: "2026-08-05T16:00:00Z",
        position: 1,
        created_at: meeting.created_at,
      },
    ];
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "test-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [meeting], limit: 20, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-decision") {
        return Promise.resolve(json({
          ...meeting,
          state: scheduled ? "scheduled" : "collecting",
          version: scheduled ? 5 : 4,
          selected_plan_option_id: scheduled ? "plan-park" : null,
          selected_time_option_id: scheduled ? "time-park" : null,
          plan_options: plans,
          time_options: times,
          participants,
        }));
      }
      if (url === "/api/v1/meetings/meeting-decision/polls") {
        return Promise.resolve(json({ items: [] }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-decision/notes?")) {
        return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-decision/availability") {
        return Promise.resolve(json({
          weights: { preferred: 3, available: 2, if_needed: 1, unavailable: -4, unanswered: 0 },
          participants,
          items: [
            {
              ...times[0],
              my_status: "preferred",
              counts: { preferred: 1, available: 1, if_needed: 0, unavailable: 1, unanswered: 1 },
              responses: [
                { user_id: "user-1", status: "preferred" },
                { user_id: "user-2", status: "available" },
                { user_id: "user-3", status: "unavailable" },
              ],
              score: 1,
            },
            {
              ...times[1],
              my_status: "available",
              counts: { preferred: 0, available: 2, if_needed: 1, unavailable: 0, unanswered: 1 },
              responses: [
                { user_id: "user-1", status: "available" },
                { user_id: "user-2", status: "available" },
                { user_id: "user-3", status: "if_needed" },
              ],
              score: 5,
            },
          ],
          recommendations: [
            {
              plan_option_id: "plan-park",
              plan_title: "Пикник в парке",
              time_option_id: "time-park",
              score: 1,
              provisional: true,
              explanation: "Меньше жёстких конфликтов среди совместимых вариантов.",
            },
            {
              plan_option_id: "plan-home",
              plan_title: "Вечер дома",
              time_option_id: "time-home",
              score: 5,
              provisional: true,
              explanation: "Лучший результат для запасного плана.",
            },
          ],
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-decision/plan-votes")) {
        return Promise.resolve(json({
          options: [
            { ...plans[0], vote_count: 2, selected_by_user: true },
            { ...plans[1], vote_count: 1, selected_by_user: false },
          ],
          responses: [
            { user_id: "user-1", display_name: "Анна", plan_option_id: "plan-park", updated_at: meeting.updated_at },
            { user_id: "user-2", display_name: "Борис", plan_option_id: "plan-park", updated_at: meeting.updated_at },
            { user_id: "user-3", display_name: "Вера", plan_option_id: "plan-home", updated_at: meeting.updated_at },
          ],
          history: [],
          participant_count: 4,
          answered_count: 3,
          history_total: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-decision/requirements")) {
        return Promise.resolve(json({
          items: [],
          total: 0,
          open_count: 0,
          completed_count: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url === "/api/v1/meetings/meeting-decision/decision" && method === "POST") {
        decisionBody = JSON.parse(String(init?.body));
        scheduled = true;
        return Promise.resolve(json({
          plan_option_id: "plan-park",
          time_option_id: "time-park",
          state: "scheduled",
        }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: /Большой пикник/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Выбор плана/ }));
    const evidence = await screen.findByRole("region", { name: "Основания решения" });

    expect(within(evidence).getByText("67%")).toBeInTheDocument();
    expect(within(evidence).getByText("3")).toBeInTheDocument();
    expect(within(evidence).getByText("1", { selector: "strong" })).toBeInTheDocument();
    expect(screen.getByText("Выбрана предварительная рекомендация Ryden")).toBeInTheDocument();
    expect(screen.getByText("План: без ответа 1")).toBeInTheDocument();
    expect(screen.getByText("Время: без ответа 1")).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Проверить решение" }));
    const review = screen.getByRole("region", { name: "Проверка решения" });
    expect(within(review).getByRole("heading", { name: "Подтвердить эту встречу?" })).toBeInTheDocument();
    expect(within(review).getByText("План не выбрали: 1.")).toBeInTheDocument();
    expect(within(review).getByText("Выбранное время не отметили: 1.")).toBeInTheDocument();
    expect(within(review).getByText("Не смогут в выбранное время: 1.")).toBeInTheDocument();
    expect(decisionBody).toBeNull();

    fireEvent.click(within(review).getByRole("button", { name: "Вернуться к выбору" }));
    expect(screen.getByRole("button", { name: "Проверить решение" })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Проверить решение" }));
    fireEvent.click(screen.getByRole("button", { name: "Да, подтвердить встречу" }));

    await waitFor(() => {
      expect(decisionBody).toEqual({
        plan_option_id: "plan-park",
        time_option_id: "time-park",
      });
      expect(screen.getByRole("heading", { name: "Встреча подтверждена" })).toBeInTheDocument();
    });
  });

  it("keeps preparation focused on unfinished items", async () => {
    let claimBody: { quantity: number } | null = null;
    let updateBody: { name: string; required_quantity: number } | null = null;
    let myWaterQuantity = 2;
    let napkinsName = "Салфетки";
    let napkinsQuantity = 20;
    const user = {
      id: "user-1",
      email: "owner@example.test",
      display_name: "Анна",
      avatar_url: null,
    };
    const meeting = {
      id: "meeting-preparation",
      owner_id: user.id,
      title: "Подготовка к пикнику",
      description: "Распределяем вещи",
      event_type: "other",
      timezone: "Asia/Novosibirsk",
      state: "scheduled",
      version: 3,
      participant_role: "owner",
      created_at: "2026-07-29T08:00:00Z",
      updated_at: "2026-07-29T08:00:00Z",
    };
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });
    const requirementPage = () => ({
      items: [
        {
          id: "requirement-water",
          name: "Вода",
          required_quantity: 10,
          claimed_quantity: myWaterQuantity + 2,
          remaining_quantity: 8 - myWaterQuantity,
          status: "open",
          my_quantity: myWaterQuantity,
          assignees: [
            { user_id: user.id, display_name: user.display_name, quantity: myWaterQuantity, updated_at: meeting.updated_at },
            { user_id: "user-2", display_name: "Борис", quantity: 2, updated_at: meeting.updated_at },
          ],
          created_at: meeting.created_at,
          updated_at: meeting.updated_at,
        },
        {
          id: "requirement-napkins",
          name: napkinsName,
          required_quantity: napkinsQuantity,
          claimed_quantity: 0,
          remaining_quantity: napkinsQuantity,
          status: "open",
          my_quantity: 0,
          assignees: [],
          created_at: meeting.created_at,
          updated_at: meeting.updated_at,
        },
        {
          id: "requirement-blankets",
          name: "Пледы",
          required_quantity: 2,
          claimed_quantity: 2,
          remaining_quantity: 0,
          status: "completed",
          my_quantity: 0,
          assignees: [{ user_id: "user-2", display_name: "Борис", quantity: 2, updated_at: meeting.updated_at }],
          created_at: meeting.created_at,
          updated_at: meeting.updated_at,
        },
      ],
      total: 3,
      open_count: 2,
      completed_count: 1,
      limit: 50,
      offset: 0,
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "test-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [meeting], limit: 20, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-preparation") {
        return Promise.resolve(json({
          ...meeting,
          plan_options: [],
          time_options: [],
          participants: [
            { user_id: user.id, display_name: user.display_name, role: "owner", joined_at: meeting.created_at },
            { user_id: "user-2", display_name: "Борис", role: "participant", joined_at: meeting.created_at },
          ],
        }));
      }
      if (url === "/api/v1/meetings/meeting-preparation/polls") {
        return Promise.resolve(json({ items: [] }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-preparation/notes?")) {
        return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-preparation/availability") {
        return Promise.resolve(json({
          weights: { preferred: 3, available: 2, if_needed: 1, unavailable: -4, unanswered: 0 },
          participants: [],
          items: [],
          recommendations: [],
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-preparation/plan-votes")) {
        return Promise.resolve(json({
          options: [],
          responses: [],
          history: [],
          participant_count: 2,
          answered_count: 0,
          history_total: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-preparation/requirements") && method === "GET") {
        return Promise.resolve(json(requirementPage()));
      }
      if (url.endsWith("/requirements/requirement-water/claim") && method === "PUT") {
        claimBody = JSON.parse(String(init?.body));
        myWaterQuantity = claimBody?.quantity ?? myWaterQuantity;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      if (url.endsWith("/requirements/requirement-napkins") && method === "PUT") {
        updateBody = JSON.parse(String(init?.body));
        napkinsName = updateBody?.name ?? napkinsName;
        napkinsQuantity = updateBody?.required_quantity ?? napkinsQuantity;
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    fireEvent.click(await screen.findByRole("button", { name: /Подготовка к пикнику/ }));
    fireEvent.click(await screen.findByRole("button", { name: /Подготовка 1 из 3 готово/ }));
    await screen.findByRole("heading", { name: "Что ещё нужно" });
    expect(screen.getByRole("article", { name: "Вода" })).toBeInTheDocument();
    expect(screen.getByRole("article", { name: "Салфетки" })).toBeInTheDocument();
    expect(screen.queryByRole("article", { name: "Пледы" })).not.toBeInTheDocument();

    fireEvent.click(screen.getByRole("checkbox", { name: /Показывать готовые/ }));
    expect(screen.getByRole("article", { name: "Пледы" })).toBeInTheDocument();

    const waterCard = screen.getByRole("article", { name: "Вода" });
    fireEvent.click(within(waterCard).getByRole("button", { name: "Взять остаток · 6" }));
    fireEvent.click(within(waterCard).getByRole("button", { name: "Сохранить · 8" }));
    await waitFor(() => expect(claimBody).toEqual({ quantity: 8 }));

    const napkinsCard = screen.getByRole("article", { name: "Салфетки" });
    fireEvent.click(within(napkinsCard).getByRole("button", { name: "Изменить" }));
    fireEvent.change(within(napkinsCard).getByLabelText("Название позиции"), {
      target: { value: "Бумажные салфетки" },
    });
    fireEvent.change(within(napkinsCard).getByLabelText(/Нужно всего/), {
      target: { value: "30" },
    });
    fireEvent.click(within(napkinsCard).getByRole("button", { name: "Сохранить изменения" }));

    await waitFor(() => {
      expect(updateBody).toEqual({ name: "Бумажные салфетки", required_quantity: 30 });
      expect(screen.getByRole("article", { name: "Бумажные салфетки" })).toBeInTheDocument();
    });
  });

  it("requires two steps to cancel and then renders a read-only record", async () => {
    let cancelled = false;
    let cancelCalls = 0;
    const user = {
      id: "user-1",
      email: "owner@example.test",
      display_name: "Owner",
      avatar_url: null,
    };
    const meeting = () => ({
      id: "meeting-1",
      owner_id: user.id,
      title: "Cancel me",
      description: "A disposable test meeting",
      event_type: "other",
      timezone: "Asia/Novosibirsk",
      state: cancelled ? "cancelled" : "draft",
      version: cancelled ? 2 : 1,
      participant_role: "owner",
      created_at: "2026-07-29T08:00:00Z",
      updated_at: "2026-07-29T08:00:00Z",
    });
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "test-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?") && method === "GET") {
        return Promise.resolve(json({ items: [meeting()], limit: 20, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-1" && method === "GET") {
        return Promise.resolve(json({
          ...meeting(),
          plan_options: [],
          time_options: [],
          participants: [{
            user_id: user.id,
            display_name: user.display_name,
            role: "owner",
            joined_at: "2026-07-29T08:00:00Z",
          }],
        }));
      }
      if (url === "/api/v1/meetings/meeting-1/polls") {
        return Promise.resolve(json({ items: [] }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-1/notes?")) {
        return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-1/availability") {
        return Promise.resolve(json({
          weights: { preferred: 3, available: 2, if_needed: 1, unavailable: -4, unanswered: 0 },
          participants: [],
          items: [],
          recommendations: [],
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-1/plan-votes")) {
        return Promise.resolve(json({
          options: [],
          responses: [],
          history: [],
          participant_count: 1,
          answered_count: 0,
          history_total: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-1/requirements")) {
        return Promise.resolve(json({
          items: [],
          total: 0,
          open_count: 0,
          completed_count: 0,
          limit: 50,
          offset: 0,
        }));
      }
      if (url === "/api/v1/meetings/meeting-1/cancel" && method === "POST") {
        cancelCalls += 1;
        cancelled = true;
        return Promise.resolve(json({
          meeting_id: "meeting-1",
          state: "cancelled",
          version: 2,
          updated_at: "2026-07-29T08:05:00Z",
        }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);

    const card = await screen.findByRole("button", { name: /Cancel me/ });
    fireEvent.click(card);
    await screen.findByRole("heading", { name: "Cancel me" });

    fireEvent.click(screen.getByRole("button", { name: "Управление встречей" }));
    fireEvent.click(screen.getByRole("button", { name: "Отменить встречу" }));
    expect(cancelCalls).toBe(0);
    expect(screen.getByRole("button", { name: "Да, отменить" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Оставить встречу" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Да, отменить" }));
    await screen.findByRole("heading", { name: "Сбор остановлен" });

    expect(cancelCalls).toBe(1);
    expect(screen.queryByRole("button", { name: "Отменить встречу" })).not.toBeInTheDocument();
    expect(screen.getByText(
      "Ссылка приглашения закрыта. Варианты, ответы и подготовка сохранены для просмотра.",
    )).toBeInTheDocument();
  });

  it("shows attendance instead of voting for a fixed meeting", async () => {
    let myStatus: "going" | "unanswered" = "unanswered";
    let attendanceBody: { status: string } | null = null;
    let noteBody: { text: string } | null = null;
    let noteText = "";
    const user = {
      id: "fixed-guest",
      email: "fixed@example.test",
      display_name: "Борис",
      avatar_url: null,
    };
    const meeting = {
      id: "meeting-fixed",
      owner_id: "fixed-owner",
      title: "Готовый ужин",
      description: "Место и время уже выбраны",
      event_type: "dinner",
      coordination_mode: "fixed",
      timezone: "Asia/Novosibirsk",
      state: "scheduled",
      selected_plan_option_id: "fixed-plan",
      selected_time_option_id: "fixed-time",
      version: 3,
      participant_role: "participant",
      created_at: "2026-07-29T08:00:00Z",
      updated_at: "2026-07-29T08:05:00Z",
    };
    const participants = [
      { user_id: "fixed-owner", display_name: "Анна", role: "owner", joined_at: meeting.created_at },
      { user_id: user.id, display_name: "Борис", role: "participant", joined_at: meeting.created_at },
    ];
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "fixed-access-token",
          access_token_expires_at: "2026-07-29T09:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [meeting], limit: 50, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-fixed") {
        return Promise.resolve(json({
          ...meeting,
          plan_options: [{
            id: "fixed-plan", title: "Ужин у Анны", description: "",
            position: 0, created_at: meeting.created_at,
          }],
          time_options: [{
            id: "fixed-time", plan_option_id: null,
            starts_at: "2026-08-10T12:00:00Z", ends_at: "2026-08-10T15:00:00Z",
            position: 0, created_at: meeting.created_at,
          }],
          participants,
        }));
      }
      if (url === "/api/v1/meetings/meeting-fixed/polls") {
        return Promise.resolve(json({ items: [] }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-fixed/notes?")) {
        return Promise.resolve(json({
          items: noteText ? [{
            user_id: user.id,
            display_name: user.display_name,
            text: noteText,
            created_at: "2026-07-29T08:10:00Z",
            updated_at: "2026-07-29T08:10:00Z",
          }] : [],
          total: noteText ? 1 : 0,
          limit: 100,
          offset: 0,
        }));
      }
      if (url === "/api/v1/meetings/meeting-fixed/notes/mine" && method === "PUT") {
        noteBody = JSON.parse(String(init?.body));
        noteText = noteBody?.text ?? "";
        return Promise.resolve(json({ changed: true }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-fixed/requirements")) {
        return Promise.resolve(json({
          items: [], total: 0, open_count: 0, completed_count: 0, limit: 50, offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-fixed/attendance") && method === "GET") {
        return Promise.resolve(json({
          participant_count: 2,
          going_count: myStatus === "going" ? 2 : 1,
          maybe_count: 0,
          not_going_count: 0,
          unanswered_count: myStatus === "going" ? 0 : 1,
          my_status: myStatus,
          participants: [
            { user_id: "fixed-owner", display_name: "Анна", role: "owner", status: "going" },
            { user_id: user.id, display_name: "Борис", role: "participant", status: myStatus },
          ],
          limit: 100,
          offset: 0,
        }));
      }
      if (url === "/api/v1/meetings/meeting-fixed/attendance" && method === "PUT") {
        attendanceBody = JSON.parse(String(init?.body));
        myStatus = "going";
        return Promise.resolve(new Response(null, { status: 204 }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /Готовый ужин/ }));
    await screen.findByRole("heading", { name: "Готовый ужин" });
    expect(screen.getByRole("button", { name: "Поделиться встречей" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Добавить в календарь" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Управление встречей" })).not.toBeInTheDocument();
    expect(screen.getByRole("button", {
      name: "Участие: Не ответил. Открыть и изменить ответ",
    })).toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Голоса" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Время" })).not.toBeInTheDocument();
    expect(screen.queryByRole("tab", { name: "Опросы" })).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {
      name: "Участие: Не ответил. Открыть и изменить ответ",
    }));
    expect(screen.getByRole("heading", { name: "Вы пойдёте?" })).toBeInTheDocument();
    expect(screen.getAllByText("Борис").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Пойду" }));
    await waitFor(() => {
      expect(attendanceBody).toEqual({ status: "going" });
      expect(screen.getByRole("heading", { name: "Пойдёт" })).toBeInTheDocument();
    });
    expect(screen.getAllByText("2").length).toBeGreaterThan(0);

    fireEvent.click(screen.getByRole("button", { name: "Закрыть" }));
    fireEvent.click(screen.getByRole("button", { name: /Заметки 0 заметок/ }));
    await screen.findByRole("heading", { name: "Важное от участников" });
    fireEvent.change(screen.getByLabelText("Ваша заметка"), {
      target: { value: "Буду на машине, могу забрать двоих" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Сохранить заметку" }));

    await waitFor(() => {
      expect(noteBody).toEqual({ text: "Буду на машине, могу забрать двоих" });
      expect(screen.getByText("Буду на машине, могу забрать двоих")).toBeInTheDocument();
    });
  });

  it("lets the owner edit every active fixed-meeting detail from the corner settings", async () => {
    let updateBody: Record<string, unknown> | null = null;
    const user = {
      id: "fixed-owner",
      email: "owner@example.test",
      display_name: "Анна",
      avatar_url: null,
    };
    let meeting: Meeting = {
      id: "meeting-fixed-owner",
      owner_id: user.id,
      title: "Ужин у Анны",
      description: "Всё уже решено",
      event_type: "other",
      coordination_mode: "fixed",
      cover_url: null,
      has_photo: false,
      location_name: "У Анны",
      location_url: null,
      timezone: "Asia/Novosibirsk",
      state: "scheduled",
      selected_plan_option_id: "fixed-plan-owner",
      selected_time_option_id: "fixed-time-owner",
      version: 3,
      participant_role: "owner",
      created_at: "2026-08-01T08:00:00Z",
      updated_at: "2026-08-01T08:05:00Z",
    };
    let timeOption: TimeOption = {
      id: "fixed-time-owner",
      plan_option_id: undefined,
      starts_at: "2026-08-15T12:00:00Z",
      ends_at: "2026-08-15T15:00:00Z",
      position: 0,
      created_at: meeting.created_at,
    };
    const json = (body: unknown, status = 200) => new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    });

    vi.stubGlobal("fetch", vi.fn().mockImplementation((input: RequestInfo | URL, init?: RequestInit) => {
      const url = String(input);
      const method = init?.method ?? "GET";
      if (url === "/api/v1/auth/refresh") {
        return Promise.resolve(json({
          user,
          access_token: "fixed-owner-token",
          access_token_expires_at: "2026-08-03T20:00:00Z",
        }));
      }
      if (url.startsWith("/api/v1/meetings?")) {
        return Promise.resolve(json({ items: [meeting], limit: 50, offset: 0 }));
      }
      if (url === "/api/v1/meetings/meeting-fixed-owner" && method === "PUT") {
        updateBody = JSON.parse(String(init?.body));
        meeting = {
          ...meeting,
          title: String(updateBody?.title),
          description: String(updateBody?.description),
          location_name: updateBody?.location_name as string | null,
          location_url: updateBody?.location_url as string | null,
          version: 4,
          updated_at: "2026-08-03T12:00:00Z",
        };
        timeOption = {
          ...timeOption,
          starts_at: String(updateBody?.starts_at),
          ends_at: updateBody?.ends_at as string | null,
        };
        return Promise.resolve(json(meeting));
      }
      if (url === "/api/v1/meetings/meeting-fixed-owner") {
        return Promise.resolve(json({
          ...meeting,
          plan_options: [{
            id: "fixed-plan-owner",
            title: meeting.title,
            description: meeting.description,
            position: 0,
            created_at: meeting.created_at,
          }],
          time_options: [timeOption],
          participants: [{
            user_id: user.id,
            display_name: user.display_name,
            role: "owner",
            joined_at: meeting.created_at,
          }],
        }));
      }
      if (url === "/api/v1/meetings/meeting-fixed-owner/polls") {
        return Promise.resolve(json({ items: [] }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-fixed-owner/notes?")) {
        return Promise.resolve(json({ items: [], total: 0, limit: 100, offset: 0 }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-fixed-owner/requirements")) {
        return Promise.resolve(json({
          items: [], total: 0, open_count: 0, completed_count: 0, limit: 50, offset: 0,
        }));
      }
      if (url.startsWith("/api/v1/meetings/meeting-fixed-owner/attendance")) {
        return Promise.resolve(json({
          participant_count: 1,
          going_count: 1,
          maybe_count: 0,
          not_going_count: 0,
          unanswered_count: 0,
          my_status: "going",
          participants: [{ user_id: user.id, display_name: user.display_name, role: "owner", status: "going" }],
          limit: 100,
          offset: 0,
        }));
      }
      return Promise.resolve(json({ error: { code: "unexpected_request", message: url } }, 500));
    }));

    render(<App />);
    fireEvent.click(await screen.findByRole("button", { name: /Ужин у Анны/ }));
    await screen.findByRole("heading", { name: "Ужин у Анны" });

    expect(screen.getByRole("button", { name: "Поделиться встречей" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Добавить в календарь" })).toBeInTheDocument();
    expect(screen.getByRole("button", {
      name: "Участие: Иду. Открыть и изменить ответ",
    })).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Управление встречей" }));

    expect(screen.getByRole("heading", { name: "Редактировать встречу" })).toBeInTheDocument();
    expect(screen.queryByText("Детали зафиксированы")).not.toBeInTheDocument();
    expect(screen.getByText("Добавить фото встречи")).toBeInTheDocument();
    fireEvent.change(screen.getByLabelText("Название встречи"), { target: { value: "Ужин перенесён" } });
    fireEvent.change(screen.getByLabelText(/Коротко о плане/), { target: { value: "Собираемся позже" } });
    fireEvent.change(screen.getByLabelText("Дата"), { target: { value: "2026-08-20" } });
    fireEvent.change(screen.getByLabelText("Время"), { target: { value: "19:10" } });
    fireEvent.change(screen.getByLabelText("Дни"), { target: { value: "1" } });
    fireEvent.change(screen.getByLabelText("Часы"), { target: { value: "2" } });
    fireEvent.change(screen.getByLabelText("Минуты"), { target: { value: "30" } });
    fireEvent.change(screen.getByLabelText(/^Место/), { target: { value: "У Димы" } });
    fireEvent.click(screen.getByRole("button", { name: /Сохранить изменения/ }));

    await waitFor(() => expect(updateBody).toMatchObject({
      title: "Ужин перенесён",
      description: "Собираемся позже",
      location_name: "У Димы",
      starts_at: "2026-08-20T12:10:00.000Z",
      ends_at: "2026-08-21T14:40:00.000Z",
      version: 3,
    }));
    await screen.findByRole("heading", { name: "Ужин перенесён" });
    expect(screen.getByText("У Димы")).toBeInTheDocument();
  });
});
