import type {
  AttendanceStatus,
  AttendanceView,
  Meeting,
  MeetingDetail,
  PlanOption,
  RequirementPage,
  TimeOption,
} from "../../api";

export function FixedMeetingProgressOverview({
  attendance,
  dateFormatter,
  meeting,
  preparation,
  selectedPlan,
  selectedTime,
  onAddPlan,
  onAddTime,
  onOpenSection,
}: {
  attendance: AttendanceView | null;
  dateFormatter: Intl.DateTimeFormat;
  meeting: MeetingDetail;
  preparation: RequirementPage | null;
  selectedPlan: PlanOption | undefined;
  selectedTime: TimeOption | undefined;
  onAddPlan: () => void;
  onAddTime: () => void;
  onOpenSection: (section: "attendance" | "people") => void;
}) {
  const detailsReady = meeting.plan_options.length === 1 && meeting.time_options.length === 1;
  const unanswered = attendance?.unanswered_count ?? meeting.participants.length;
  const stages = ["Детали", "Участие", "Подготовка", "Встреча"];
  const activeStage = meeting.state === "draft"
    ? 0
    : meeting.state === "completed"
      ? 4
      : meeting.state === "cancelled"
        ? selectedPlan && selectedTime ? 2 : 0
        : unanswered > 0 ? 1 : 2;

  let headline = "Укажите готовый план";
  let description = "Здесь не будет голосования: оставьте один план и одно время встречи.";
  let action: { label: string; run: () => void } | null = {
    label: meeting.plan_options.length === 0 ? "Добавить план" : "Добавить время",
    run: meeting.plan_options.length === 0 ? onAddPlan : onAddTime,
  };
  if (meeting.state === "draft" && detailsReady) {
    headline = "Приглашение ещё не готово";
    description = "Обычно Ryden создаёт его сразу после встречи. Повторите действие, чтобы открыть сбор ответов.";
    action = { label: "Повторить запуск", run: () => onOpenSection("people") };
  } else if (meeting.state === "scheduled") {
    headline = attendance?.my_status === "unanswered" ? "Вы пойдёте?" : "Ответ об участии сохранён";
    description = "Решение уже принято. Группа видит, кто идёт, кто не идёт и от кого ещё нужен ответ.";
    action = { label: "Открыть участие", run: () => onOpenSection("attendance") };
  } else if (meeting.state === "completed") {
    headline = "Встреча завершена";
    description = "Состав участников, готовый план и подготовка сохранены как итог группы.";
    action = null;
  } else if (meeting.state === "cancelled") {
    headline = "Встреча отменена";
    description = "План и ответы об участии сохранены, но больше не изменяются.";
    action = null;
  }

  return (
    <section className={`meeting-progress-overview overview-${meeting.state}`} aria-labelledby="meeting-progress-title">
      <header className="meeting-progress-heading">
        <div>
          <p className="section-kicker">КАК ИДУТ ДЕЛА</p>
          <h2 id="meeting-progress-title">Готовность встречи</h2>
        </div>
        <span>План уже готов</span>
      </header>
      <ol className="meeting-route" aria-label="Этапы встречи">
        {stages.map((stage, index) => {
          const state = meeting.state === "completed"
            ? "done"
            : meeting.state === "cancelled"
              ? index < activeStage ? "done" : index === activeStage ? "stopped" : "waiting"
              : index < activeStage ? "done" : index === activeStage ? "current" : "waiting";
          return (
            <li className={`route-${state}`} key={stage}>
              <span aria-hidden="true">{state === "done" ? "✓" : String(index + 1).padStart(2, "0")}</span>
              <strong>{stage}</strong>
              <small>
                {state === "done" && "готово"}
                {state === "current" && "сейчас"}
                {state === "stopped" && "остановлено"}
                {state === "waiting" && "дальше"}
              </small>
            </li>
          );
        })}
      </ol>
      <div className="meeting-next-card">
        <div className="meeting-next-copy">
          <p className="section-kicker">{meeting.state === "scheduled" ? "ВАШ ОТВЕТ" : "СЛЕДУЮЩИЙ ШАГ"}</p>
          <h3>{headline}</h3>
          <p>{description}</p>
          {action && (
            <button className="secondary-button" onClick={action.run} type="button">
              {action.label} <span aria-hidden="true">→</span>
            </button>
          )}
        </div>
        <dl className="meeting-progress-metrics">
          <div><dt>Пойдут</dt><dd>{attendance?.going_count ?? 0}</dd><small>подтвердили участие</small></div>
          <div><dt>Не пойдут</dt><dd>{attendance?.not_going_count ?? 0}</dd><small>ответили отказом</small></div>
          <div><dt>Без ответа</dt><dd>{unanswered}</dd><small>из {attendance?.participant_count ?? meeting.participants.length}</small></div>
        </dl>
        {meeting.state === "draft" && (
          <ul className="meeting-task-list" aria-label="Готовность деталей">
            <li className={meeting.plan_options.length === 1 ? "done" : "pending"}>
              <span aria-hidden="true">{meeting.plan_options.length === 1 ? "✓" : "·"}</span>
              <div><strong>Один план</strong><small>{meeting.plan_options.length === 1 ? "готов" : "ещё не указан"}</small></div>
            </li>
            <li className={meeting.time_options.length === 1 ? "done" : "pending"}>
              <span aria-hidden="true">{meeting.time_options.length === 1 ? "✓" : "·"}</span>
              <div><strong>Одно время</strong><small>{meeting.time_options.length === 1 ? "готово" : "ещё не указано"}</small></div>
            </li>
          </ul>
        )}
      </div>
      {selectedPlan && selectedTime && (
        <dl className="overview-decision-strip" aria-label="Подтверждённые детали">
          <div><dt>План</dt><dd>{selectedPlan.title}</dd></div>
          <div><dt>Время встречи</dt><dd>{dateFormatter.format(new Date(selectedTime.starts_at))}</dd></div>
          <div><dt>Подготовка</dt><dd>{preparation?.total ?? 0} пунктов</dd></div>
        </dl>
      )}
    </section>
  );
}

export function AttendanceSection({
  attendance,
  meetingState,
  readOnly = false,
  working,
  onRespond,
}: {
  attendance: AttendanceView;
  meetingState: Meeting["state"];
  readOnly?: boolean;
  working: boolean;
  onRespond: (status: AttendanceStatus) => void;
}) {
  const editable = meetingState === "scheduled" && !readOnly;
  const statusLabel: Record<AttendanceStatus, string> = {
    going: "Пойдёт",
    maybe: "Думает",
    not_going: "Не пойдёт",
    unanswered: "Без ответа",
  };
  const total = Math.max(1, attendance.participant_count);
  const goingEnd = (attendance.going_count / total) * 100;
  const maybeEnd = goingEnd + (attendance.maybe_count / total) * 100;
  const notGoingEnd = maybeEnd + (attendance.not_going_count / total) * 100;
  const chartBackground = `conic-gradient(
    var(--attendance-going) 0 ${goingEnd}%,
    var(--attendance-maybe) ${goingEnd}% ${maybeEnd}%,
    var(--attendance-not-going) ${maybeEnd}% ${notGoingEnd}%,
    var(--attendance-unanswered) ${notGoingEnd}% 100%
  )`;
  const sortedParticipants = attendance.participants.slice().sort((left, right) => {
    const order: Record<AttendanceStatus, number> = { going: 0, maybe: 1, not_going: 2, unanswered: 3 };
    return order[left.status] - order[right.status] || left.display_name.localeCompare(right.display_name, "ru");
  });
  return (
    <section className="attendance-section" aria-labelledby="attendance-title">
      <header className="section-heading">
        <div>
          <p className="section-kicker">УЧАСТИЕ</p>
          <h2 id="attendance-title">Кто будет</h2>
        </div>
        <span>{attendance.participant_count}</span>
      </header>
      <p className="poll-intro">
        План и время уже известны. Отметьте, сможете ли вы прийти.
      </p>
      <dl className="attendance-summary">
        <div><dt>Пойдут</dt><dd>{attendance.going_count}</dd></div>
        <div><dt>Думают</dt><dd>{attendance.maybe_count}</dd></div>
        <div><dt>Не пойдут</dt><dd>{attendance.not_going_count}</dd></div>
        <div><dt>Без ответа</dt><dd>{attendance.unanswered_count}</dd></div>
      </dl>
      <div className="attendance-insights">
        <div className="attendance-donut" style={{ background: chartBackground }} role="img" aria-label="Распределение ответов участников">
          <span><strong>{attendance.going_count}</strong><small>идут</small></span>
        </div>
        <ul className="attendance-legend" aria-label="Легенда ответов">
          <li className="legend-going"><i />Идут <strong>{attendance.going_count}</strong></li>
          <li className="legend-maybe"><i />Думают <strong>{attendance.maybe_count}</strong></li>
          <li className="legend-not-going"><i />Не идут <strong>{attendance.not_going_count}</strong></li>
          <li className="legend-unanswered"><i />Без ответа <strong>{attendance.unanswered_count}</strong></li>
        </ul>
      </div>
      <div className="attendance-response-card">
        <div>
          <p className="section-kicker">ВАШ ОТВЕТ</p>
          <h3>
            {attendance.my_status === "unanswered"
              ? "Вы пойдёте?"
              : statusLabel[attendance.my_status]}
          </h3>
        </div>
        {editable ? (
          <div className="attendance-actions">
            <button
              aria-pressed={attendance.my_status === "going"}
              className={attendance.my_status === "going" ? "selected" : ""}
              disabled={working}
              onClick={() => onRespond("going")}
              type="button"
            >
              Пойду
            </button>
            <button
              aria-pressed={attendance.my_status === "maybe"}
              className={attendance.my_status === "maybe" ? "selected" : ""}
              disabled={working}
              onClick={() => onRespond("maybe")}
              type="button"
            >
              Думаю
            </button>
            <button
              aria-pressed={attendance.my_status === "not_going"}
              className={attendance.my_status === "not_going" ? "selected" : ""}
              disabled={working}
              onClick={() => onRespond("not_going")}
              type="button"
            >
              Не пойду
            </button>
            {attendance.my_status !== "unanswered" && (
              <button className="quiet-button" disabled={working} onClick={() => onRespond("unanswered")} type="button">
                Очистить
              </button>
            )}
          </div>
        ) : (
          <p className="muted">Ответы сохранены, изменить их уже нельзя.</p>
        )}
      </div>
      <div className="attendance-list-heading">
        <div>
          <p className="section-kicker">ВСЕ УЧАСТНИКИ</p>
          <h3>Кто как ответил</h3>
        </div>
        <span>{attendance.participant_count}</span>
      </div>
      <div className="attendance-list" aria-label="Ответы участников">
        {sortedParticipants.map((participant) => (
          <article className={`attendance-person attendance-${participant.status}`} key={participant.user_id}>
            <i aria-hidden="true">{participant.display_name.slice(0, 1).toUpperCase()}</i>
            <div>
              <strong>{participant.display_name}</strong>
              <small>{participant.role === "owner" ? "организатор" : "участник"}</small>
            </div>
            <span>{statusLabel[participant.status]}</span>
          </article>
        ))}
      </div>
      {attendance.participants.length < attendance.participant_count && (
        <p className="panel-empty">
          Показаны первые {attendance.participants.length} из {attendance.participant_count} участников.
        </p>
      )}
    </section>
  );
}
