import type {
  AttendanceView,
  AvailabilityView,
  MeetingDetail,
  PlanOption,
  PlanVotePage,
  Poll,
  RequirementPage,
  TimeOption,
} from "../../api";
import { FixedMeetingProgressOverview } from "./fixed-meeting";

type MeetingOverviewSection =
  | "attendance"
  | "availability"
  | "people"
  | "polls"
  | "preparation"
  | "votes";

type OverviewMetric = {
  label: string;
  value: string;
  detail: string;
};

type OverviewTask = {
  label: string;
  detail: string;
  done: boolean;
};

export function MeetingProgressOverview({
  attendance,
  availability,
  dateFormatter,
  meeting,
  planVotes,
  polls,
  preparation,
  selectedPlan,
  selectedTime,
  onAddPlan,
  onAddTime,
  onOpenSection,
}: {
  attendance: AttendanceView | null;
  availability: AvailabilityView | null;
  dateFormatter: Intl.DateTimeFormat;
  meeting: MeetingDetail;
  planVotes: PlanVotePage | null;
  polls: Poll[];
  preparation: RequirementPage | null;
  selectedPlan: PlanOption | undefined;
  selectedTime: TimeOption | undefined;
  onAddPlan: () => void;
  onAddTime: () => void;
  onOpenSection: (section: MeetingOverviewSection) => void;
}) {
  if (meeting.coordination_mode === "fixed") {
    return (
      <FixedMeetingProgressOverview
        attendance={attendance}
        dateFormatter={dateFormatter}
        meeting={meeting}
        preparation={preparation}
        selectedPlan={selectedPlan}
        selectedTime={selectedTime}
        onAddPlan={onAddPlan}
        onAddTime={onAddTime}
        onOpenSection={onOpenSection}
      />
    );
  }
  const planReady = meeting.plan_options.length >= 2;
  const timeReady = meeting.time_options.length >= 2;
  const participantCount = planVotes?.participant_count ?? meeting.participants.length;
  const availabilityParticipantCount =
    availability?.participants.length ?? meeting.participants.length;
  const planAnsweredCount = planVotes?.answered_count ?? 0;
  const totalTimeAnswers = availability?.items.reduce(
    (total, option) =>
      total + Math.max(0, availabilityParticipantCount - option.counts.unanswered),
    0,
  ) ?? 0;
  const possibleTimeAnswers =
    (availability?.items.length ?? 0) * availabilityParticipantCount;
  const timeAnswerPercent = possibleTimeAnswers === 0
    ? 0
    : Math.round((totalTimeAnswers / possibleTimeAnswers) * 100);
  const groupCoreAnswersReady = participantCount > 0
    && planAnsweredCount === participantCount
    && possibleTimeAnswers > 0
    && totalTimeAnswers === possibleTimeAnswers;
  const myPlanAnswered = planVotes?.options.some((option) => option.selected_by_user) ?? false;
  const myUnansweredTimes = availability?.items.filter(
    (option) => option.my_status === "unanswered",
  ).length ?? 0;
  const openPolls = polls.filter((poll) => poll.accepting_answers);
  const myUnansweredPolls = openPolls.filter(
    (poll) => !poll.options.some((option) => option.selected_by_user),
  ).length;
  const myRequirements = preparation?.items.filter((item) => item.my_quantity > 0) ?? [];
  const myQuantity = myRequirements.reduce((total, item) => total + item.my_quantity, 0);
  const unassignedQuantity = preparation?.items.reduce(
    (total, item) => total + item.remaining_quantity,
    0,
  ) ?? 0;

  let headline = "";
  let description = "";
  let metrics: OverviewMetric[] = [];
  let tasks: OverviewTask[] = [];
  let action: { label: string; run: () => void } | null = null;

  if (meeting.state === "draft") {
    headline = planReady && timeReady ? "Черновик готов к участникам" : "Подготовьте сбор ответов";
    description = planReady && timeReady
      ? "Основные варианты собраны. Создайте приватную ссылку и откройте голосование."
      : "Для честного выбора нужны минимум два плана и два варианта времени.";
    metrics = [
      {
        label: "Планы",
        value: String(meeting.plan_options.length),
        detail: planReady ? "минимум набран" : "нужно минимум 2",
      },
      {
        label: "Время",
        value: String(meeting.time_options.length),
        detail: timeReady ? "минимум набран" : "нужно минимум 2",
      },
      {
        label: "Участники",
        value: String(meeting.participants.length),
        detail: "пока только свои",
      },
    ];
    tasks = [
      {
        label: "Варианты плана",
        detail: planReady ? "можно сравнивать" : `ещё ${2 - meeting.plan_options.length}`,
        done: planReady,
      },
      {
        label: "Варианты времени",
        detail: timeReady ? "можно отмечать доступность" : `ещё ${2 - meeting.time_options.length}`,
        done: timeReady,
      },
      {
        label: "Приватное приглашение",
        detail: planReady && timeReady ? "можно создать" : "после вариантов",
        done: false,
      },
    ];
    action = !planReady
      ? { label: "Добавить план", run: onAddPlan }
      : !timeReady
        ? { label: "Добавить время", run: onAddTime }
        : { label: "Перейти к приглашению", run: () => onOpenSection("people") };
  } else if (meeting.state === "collecting") {
    const personalDone = myPlanAnswered && myUnansweredTimes === 0 && myUnansweredPolls === 0;
    headline = personalDone
      ? meeting.participant_role === "owner"
        ? "Сравните ответы и примите решение"
        : "Ваши ответы сохранены"
      : "Закройте свои ответы";
    description = personalDone
      ? meeting.participant_role === "owner"
        ? "Ryden уже собрал результаты отдельно по планам и времени; итоговая пара остаётся за вами."
        : "Можно следить за результатами: решение закрепит организатор."
      : "Ryden показывает только оставшиеся действия — сохранённые ответы повторять не нужно.";
    metrics = [
      {
        label: "План выбрали",
        value: `${planAnsweredCount} / ${participantCount}`,
        detail: groupCoreAnswersReady ? "все ответили" : "ответы группы",
      },
      {
        label: "Время отмечено",
        value: `${timeAnswerPercent}%`,
        detail: possibleTimeAnswers === 0 ? "нет вариантов" : "всех отметок",
      },
      {
        label: "Открытые опросы",
        value: String(openPolls.length),
        detail: openPolls.length === 0 ? "дополнительных нет" : "вопросы группы",
      },
    ];
    tasks = [
      {
        label: "Ваш выбор плана",
        detail: myPlanAnswered ? "сохранён" : "не выбран",
        done: myPlanAnswered,
      },
      {
        label: "Ваша доступность",
        detail: myUnansweredTimes === 0 ? "всё отмечено" : `без ответа: ${myUnansweredTimes}`,
        done: myUnansweredTimes === 0,
      },
      {
        label: "Ваши опросы",
        detail: openPolls.length === 0
          ? "нет открытых"
          : myUnansweredPolls === 0
            ? "всё отвечено"
            : `без ответа: ${myUnansweredPolls}`,
        done: myUnansweredPolls === 0,
      },
    ];
    action = !myPlanAnswered
      ? { label: "Выбрать план", run: () => onOpenSection("votes") }
      : myUnansweredTimes > 0
        ? { label: "Отметить время", run: () => onOpenSection("availability") }
        : myUnansweredPolls > 0
          ? { label: "Ответить на опросы", run: () => onOpenSection("polls") }
          : meeting.participant_role === "owner"
            ? { label: "Сравнить и подтвердить", run: () => onOpenSection("votes") }
            : { label: "Посмотреть результаты", run: () => onOpenSection("votes") };
  } else if (meeting.state === "scheduled") {
    headline = "Распределите подготовку";
    description = "Решение закреплено. Теперь группе важно разобрать остаток и довести позиции до готовности.";
    metrics = [
      {
        label: "Готово",
        value: `${preparation?.completed_count ?? 0} / ${preparation?.total ?? 0}`,
        detail: "позиций подготовки",
      },
      {
        label: "Не распределено",
        value: String(unassignedQuantity),
        detail: "единиц осталось",
      },
      {
        label: "На вас",
        value: String(myQuantity),
        detail: `${myRequirements.length} позиций`,
      },
    ];
    tasks = [
      {
        label: "План и время",
        detail: "подтверждены",
        done: Boolean(selectedPlan && selectedTime),
      },
      {
        label: "Свободный остаток",
        detail: unassignedQuantity === 0 ? "всё распределено" : `осталось: ${unassignedQuantity}`,
        done: unassignedQuantity === 0,
      },
      {
        label: "Общая подготовка",
        detail: (preparation?.open_count ?? 0) === 0 ? "всё готово" : `в работе: ${preparation?.open_count ?? 0}`,
        done: (preparation?.total ?? 0) > 0 && (preparation?.open_count ?? 0) === 0,
      },
    ];
    action = { label: "Открыть подготовку", run: () => onOpenSection("preparation") };
  } else if (meeting.state === "completed") {
    headline = "Встреча завершена";
    description = "Решение, ответы и распределённая подготовка сохранены как общий итог группы.";
    metrics = [
      {
        label: "План",
        value: selectedPlan?.title ?? "—",
        detail: "итоговый выбор",
      },
      {
        label: "Подготовка",
        value: `${preparation?.completed_count ?? 0} / ${preparation?.total ?? 0}`,
        detail: "готовых позиций",
      },
      {
        label: "Участники",
        value: String(meeting.participants.length),
        detail: "в общем итоге",
      },
    ];
  } else {
    headline = selectedPlan && selectedTime ? "Сохранён итог до отмены" : "Встреча остановлена";
    description = "Изменения закрыты, но варианты, ответы и подготовка остались доступны участникам.";
    metrics = [
      {
        label: "План",
        value: selectedPlan?.title ?? "Не выбран",
        detail: selectedPlan ? "решение сохранено" : "решения не было",
      },
      {
        label: "Участники",
        value: String(meeting.participants.length),
        detail: "сохранили доступ",
      },
      {
        label: "Подготовка",
        value: String(preparation?.total ?? 0),
        detail: "позиций в записи",
      },
    ];
  }

  const activeStage = meeting.state === "draft"
    ? 0
    : meeting.state === "collecting"
      ? groupCoreAnswersReady ? 2 : 1
      : meeting.state === "scheduled"
        ? 3
        : meeting.state === "completed"
          ? 4
          : selectedPlan && selectedTime
            ? 3
            : meeting.participants.length > 1
              ? 1
              : 0;
  const stages = ["Варианты", "Ответы", "Решение", "Подготовка"];

  return (
    <section className={`meeting-progress-overview overview-${meeting.state}`} aria-labelledby="meeting-progress-title">
      <header className="meeting-progress-heading">
        <div>
          <p className="section-kicker">КАК ИДУТ ДЕЛА</p>
          <h2 id="meeting-progress-title">Готовность встречи</h2>
        </div>
        <span>{meeting.participant_role === "owner" ? "Вы организатор" : "Вы участник"}</span>
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
          <p className="section-kicker">
            {meeting.state === "collecting" ? "ВАШИ ОТВЕТЫ" : "СЛЕДУЮЩИЙ ШАГ"}
          </p>
          <h3>{headline}</h3>
          <p>{description}</p>
          {action && (
            <button className="secondary-button" onClick={action.run} type="button">
              {action.label} <span aria-hidden="true">→</span>
            </button>
          )}
        </div>

        <dl className="meeting-progress-metrics">
          {metrics.map((metric) => (
            <div key={metric.label}>
              <dt>{metric.label}</dt>
              <dd>{metric.value}</dd>
              <small>{metric.detail}</small>
            </div>
          ))}
        </dl>

        {tasks.length > 0 && (
          <ul className="meeting-task-list" aria-label={meeting.state === "collecting" ? "Ваши ответы" : "Готовность этапа"}>
            {tasks.map((task) => (
              <li className={task.done ? "done" : "pending"} key={task.label}>
                <span aria-hidden="true">{task.done ? "✓" : "·"}</span>
                <div><strong>{task.label}</strong><small>{task.detail}</small></div>
              </li>
            ))}
          </ul>
        )}
      </div>

      {selectedPlan && selectedTime && (
        <dl className="overview-decision-strip" aria-label="Подтверждённое решение">
          <div><dt>План</dt><dd>{selectedPlan.title}</dd></div>
          <div><dt>Время встречи</dt><dd>{dateFormatter.format(new Date(selectedTime.starts_at))}</dd></div>
        </dl>
      )}
    </section>
  );
}
