import { useState } from "react";
import { api, type IncomingMeetingInvite } from "../../api";
import { errorMessage } from "../../error-message";

interface MeetingInvitationsDialogProps {
  invitations: IncomingMeetingInvite[];
  onClose: () => void;
  onAccepted: (invitation: IncomingMeetingInvite) => void;
  onDeclined: (invitationID: string) => void;
}

export function MeetingInvitationsDialog({
  invitations,
  onClose,
  onAccepted,
  onDeclined,
}: MeetingInvitationsDialogProps) {
  const [workingID, setWorkingID] = useState("");
  const [error, setError] = useState("");

  async function accept(invitation: IncomingMeetingInvite) {
    setWorkingID(invitation.id);
    setError("");
    try {
      await api.acceptMeetingInvite(invitation.id);
      onAccepted(invitation);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setWorkingID("");
    }
  }

  async function decline(invitationID: string) {
    setWorkingID(invitationID);
    setError("");
    try {
      await api.declineMeetingInvite(invitationID);
      onDeclined(invitationID);
    } catch (requestError) {
      setError(errorMessage(requestError));
    } finally {
      setWorkingID("");
    }
  }

  return (
    <div className="dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        aria-labelledby="meeting-invitations-title"
        aria-modal="true"
        className="create-dialog meeting-invitations-dialog"
        onMouseDown={(event) => event.stopPropagation()}
        role="dialog"
      >
        <button aria-label="Закрыть приглашения" className="dialog-close" onClick={onClose} type="button">×</button>
        <p className="section-kicker">ПРИГЛАШЕНИЯ</p>
        <div className="meeting-invitations-heading">
          <div>
            <h2 id="meeting-invitations-title">Вас зовут на встречу</h2>
            <p className="muted">После принятия встреча появится на главной.</p>
          </div>
          <span>{invitations.length}</span>
        </div>
        {error && <p className="form-error" role="alert">{error}</p>}
        {invitations.length === 0 ? (
          <div className="meeting-invitations-empty">
            <strong>Новых приглашений нет</strong>
            <p>Когда друг позовёт вас на встречу, она появится здесь.</p>
          </div>
        ) : (
          <div className="meeting-invitations-list">
            {invitations.map((invitation) => {
              const date = invitation.starts_at
                ? new Intl.DateTimeFormat("ru-RU", {
                  day: "numeric",
                  month: "long",
                  hour: "2-digit",
                  minute: "2-digit",
                  timeZone: invitation.timezone,
                }).format(new Date(invitation.starts_at))
                : "Время ещё выбирают";
              return (
                <article className="meeting-invitation-card" key={invitation.id}>
                  <div className="meeting-invitation-date">
                    <span aria-hidden="true">○</span>
                    <time>{date}</time>
                  </div>
                  <h3>{invitation.meeting_title}</h3>
                  <p>Организатор: <strong>{invitation.owner_display_name}</strong></p>
                  <div className="meeting-invitation-actions">
                    <button
                      className="quiet-button"
                      disabled={Boolean(workingID)}
                      onClick={() => void decline(invitation.id)}
                      type="button"
                    >
                      Отказаться
                    </button>
                    <button
                      className="primary-button"
                      disabled={Boolean(workingID)}
                      onClick={() => void accept(invitation)}
                      type="button"
                    >
                      {workingID === invitation.id ? "Сохраняем…" : "Принять"}
                    </button>
                  </div>
                </article>
              );
            })}
          </div>
        )}
      </section>
    </div>
  );
}
