import { useCallback, useEffect, useState } from "react";
import { api, type MeetingInviteCandidate } from "../../api";
import { StoredAvatarImage } from "../../components/app-shell";
import { errorMessage } from "../../error-message";

interface MeetingShareDialogProps {
  meetingID: string;
  meetingTitle: string;
  inviteLink: string;
  working: boolean;
  onClose: () => void;
  onCopy: () => Promise<void>;
  onReplace: () => Promise<string | null>;
}

export function MeetingShareDialog({
  meetingID,
  meetingTitle,
  inviteLink,
  working,
  onClose,
  onCopy,
  onReplace,
}: MeetingShareDialogProps) {
  const [friendCandidates, setFriendCandidates] = useState<MeetingInviteCandidate[]>([]);
  const [selectedFriendIDs, setSelectedFriendIDs] = useState<string[]>([]);
  const [friendInvitesLoading, setFriendInvitesLoading] = useState(true);
  const [friendInvitesWorking, setFriendInvitesWorking] = useState(false);
  const [friendInvitesError, setFriendInvitesError] = useState("");

  const shareText = `Присоединяйтесь к встрече «${meetingTitle}» в Ryden`;
  const telegramURL = inviteLink
    ? `https://t.me/share/url?url=${encodeURIComponent(inviteLink)}&text=${encodeURIComponent(shareText)}`
    : "";
  const vkURL = inviteLink
    ? `https://vk.com/share.php?url=${encodeURIComponent(inviteLink)}&title=${encodeURIComponent(shareText)}`
    : "";

  const loadFriendCandidates = useCallback(async () => {
    setFriendInvitesLoading(true);
    setFriendInvitesError("");
    try {
      const page = await api.getMeetingInviteCandidates(meetingID);
      setFriendCandidates(page.items);
      setSelectedFriendIDs([]);
    } catch (requestError) {
      setFriendInvitesError(errorMessage(requestError));
    } finally {
      setFriendInvitesLoading(false);
    }
  }, [meetingID]);

  useEffect(() => {
    void loadFriendCandidates();
  }, [loadFriendCandidates]);

  async function inviteSelectedFriends() {
    if (selectedFriendIDs.length === 0) return;
    setFriendInvitesWorking(true);
    setFriendInvitesError("");
    try {
      await api.sendMeetingInvites(meetingID, selectedFriendIDs);
      await loadFriendCandidates();
    } catch (requestError) {
      setFriendInvitesError(errorMessage(requestError));
    } finally {
      setFriendInvitesWorking(false);
    }
  }

  async function shareNative() {
    if (!inviteLink || !navigator.share) return;
    try {
      await navigator.share({ title: meetingTitle, text: shareText, url: inviteLink });
    } catch (shareError) {
      if (shareError instanceof DOMException && shareError.name === "AbortError") return;
    }
  }

  return (
    <div className="dialog-backdrop share-dialog-backdrop" role="presentation" onMouseDown={onClose}>
      <section
        aria-labelledby="share-dialog-title"
        aria-modal="true"
        className="share-dialog"
        onMouseDown={(event) => event.stopPropagation()}
        role="dialog"
      >
        <button aria-label="Закрыть приглашение" className="dialog-close" onClick={onClose} type="button">×</button>
        <p className="section-kicker">ПОДЕЛИТЬСЯ</p>
        <h2 id="share-dialog-title">Поделиться встречей</h2>
        <section className="friend-invite-picker" aria-labelledby="friend-invite-title">
          <div className="friend-invite-heading">
            <div>
              <h3 id="friend-invite-title">Пригласить друзей</h3>
              <p>Выберите нескольких друзей — каждый сам примет или отклонит приглашение.</p>
            </div>
            {selectedFriendIDs.length > 0 && <span>Выбрано: {selectedFriendIDs.length}</span>}
          </div>
          {friendInvitesError && <p className="form-error" role="alert">{friendInvitesError}</p>}
          {friendInvitesLoading ? (
            <p className="muted">Загружаем друзей…</p>
          ) : friendCandidates.length === 0 ? (
            <p className="friend-invite-empty">Сначала добавьте друзей через кнопку в верхней панели.</p>
          ) : (
            <div className="friend-invite-list">
              {friendCandidates.map((candidate) => (
                <FriendInvitationChoice
                  candidate={candidate}
                  checked={selectedFriendIDs.includes(candidate.user_id)}
                  disabled={friendInvitesWorking}
                  key={candidate.user_id}
                  onChange={(checked) => {
                    setSelectedFriendIDs((current) => checked
                      ? [...current, candidate.user_id]
                      : current.filter((id) => id !== candidate.user_id));
                  }}
                />
              ))}
            </div>
          )}
          <button
            className="secondary-button friend-invite-submit"
            disabled={selectedFriendIDs.length === 0 || friendInvitesWorking}
            onClick={() => void inviteSelectedFriends()}
            type="button"
          >
            {friendInvitesWorking
              ? "Отправляем…"
              : `Пригласить${selectedFriendIDs.length > 0 ? ` · ${selectedFriendIDs.length}` : ""}`}
          </button>
        </section>
        {inviteLink ? (
          <>
            <p className="muted">Участнику понадобится аккаунт Ryden. Ссылка действует семь дней.</p>
            <div className="share-link">
              <input aria-label="Ссылка приглашения" onFocus={(event) => event.currentTarget.select()} readOnly value={inviteLink} />
              <button aria-label="Скопировать ссылку" onClick={() => void onCopy()} title="Скопировать" type="button">⧉</button>
            </div>
            <div className="share-options" aria-label="Способы отправки">
              {typeof navigator.share === "function" && (
                <button className="share-option" onClick={() => void shareNative()} type="button">
                  <span aria-hidden="true">↗</span>
                  Другие приложения
                </button>
              )}
              <a className="share-option" href={telegramURL} rel="noreferrer noopener" target="_blank">
                <span aria-hidden="true">TG</span>
                Telegram
              </a>
              <a className="share-option" href={vkURL} rel="noreferrer noopener" target="_blank">
                <span aria-hidden="true">VK</span>
                ВКонтакте
              </a>
            </div>
          </>
        ) : (
          <div className="share-link-missing">
            <p>Из соображений безопасности прежняя ссылка больше не показывается после обновления страницы.</p>
            <p className="muted">Создайте новую ссылку — предыдущая сразу перестанет работать.</p>
            <button className="primary-button" disabled={working} onClick={() => void onReplace()} type="button">
              {working ? "Готовим ссылку…" : "Создать новую ссылку"}
            </button>
          </div>
        )}
      </section>
    </div>
  );
}

function FriendInvitationChoice({
  candidate,
  checked,
  disabled,
  onChange,
}: {
  candidate: MeetingInviteCandidate;
  checked: boolean;
  disabled: boolean;
  onChange: (checked: boolean) => void;
}) {
  const selectable = !candidate.is_participant && candidate.invitation_status === null;
  const state = candidate.is_participant || candidate.invitation_status === "accepted"
    ? "Уже участник"
    : candidate.invitation_status === "pending"
      ? "Приглашение отправлено"
      : candidate.invitation_status === "declined"
        ? "Отказался"
        : "Можно пригласить";

  return (
    <label className={`friend-invite-row${selectable ? "" : " disabled"}`}>
      <input
        checked={checked}
        disabled={!selectable || disabled}
        onChange={(event) => onChange(event.currentTarget.checked)}
        type="checkbox"
      />
      <span className="friend-avatar" aria-hidden="true">
        <span>{candidate.display_name.slice(0, 1).toUpperCase()}</span>
        <StoredAvatarImage
          userID={candidate.user_id}
          revision={candidate.avatar_revision}
          legacyURL={candidate.avatar_url}
        />
      </span>
      <span>
        <strong>{candidate.display_name}</strong>
        <small>@{candidate.nickname}</small>
      </span>
      <small>{state}</small>
    </label>
  );
}
