import { useEffect, useState } from "react";
import { api, type User } from "../api";
import type { Theme } from "../theme";

interface ThemeButtonProps {
  theme: Theme;
  onChange: () => void;
}

interface AppHeaderProps {
  user: User;
  theme: Theme;
  onThemeChange: () => void;
  onFriends: () => void;
  incomingInviteCount: number;
  onMeetingInvites: () => void;
  onEditProfile: () => void;
  onLogout: () => Promise<void>;
}

interface StoredAvatarImageProps {
  userID: string;
  revision: number | null | undefined;
  legacyURL?: string | null;
  previewURL?: string;
}

interface AvatarProps {
  user: User;
  large?: boolean;
  previewURL?: string;
}

export function Brand() {
  return (
    <a className="brand" href="/" aria-label="На главную Ryden">
      <img className="brand-frog" src="/brand/ryden-frog-wave.webp" alt="" aria-hidden="true" />
      <span className="brand-name">Ryden</span>
    </a>
  );
}

export function ThemeButton({ theme, onChange }: ThemeButtonProps) {
  const nextThemeLabel = theme === "light" ? "Включить тёмную тему" : "Включить светлую тему";
  return (
    <button className="icon-button" type="button" onClick={onChange} aria-label={nextThemeLabel}>
      <span aria-hidden="true">{theme === "light" ? "☾" : "☀"}</span>
    </button>
  );
}

export function LoadingScreen({ theme, onThemeChange }: {
  theme: Theme;
  onThemeChange: () => void;
}) {
  return (
    <div className="page-shell">
      <header className="topbar">
        <Brand />
        <ThemeButton theme={theme} onChange={onThemeChange} />
      </header>
      <main className="center-state" aria-live="polite">
        <span className="loading-spinner" aria-hidden="true" />
        <p>Возвращаемся к вашим встречам…</p>
      </main>
    </div>
  );
}

export function AppHeader({
  user,
  theme,
  onThemeChange,
  onFriends,
  incomingInviteCount,
  onMeetingInvites,
  onEditProfile,
  onLogout,
}: AppHeaderProps) {
  return (
    <header className="app-header">
      <Brand />
      <div className="header-actions">
        <ThemeButton theme={theme} onChange={onThemeChange} />
        <button
          aria-label={`Приглашения на встречи: ${incomingInviteCount}`}
          className="icon-button meeting-invites-button"
          onClick={onMeetingInvites}
          type="button"
        >
          <svg aria-hidden="true" viewBox="0 0 24 24">
            <path d="M5 6.5h14v11H5zM5.5 7l6.5 5 6.5-5" />
          </svg>
          {incomingInviteCount > 0 && (
            <span className="header-count-badge" aria-hidden="true">
              {incomingInviteCount > 99 ? "99+" : incomingInviteCount}
            </span>
          )}
        </button>
        <button className="icon-button friends-button" type="button" onClick={onFriends} aria-label="Друзья">
          <svg aria-hidden="true" viewBox="0 0 24 24">
            <path d="M8.5 11a3 3 0 1 0 0-6 3 3 0 0 0 0 6ZM15.8 10a2.4 2.4 0 1 0 0-4.8" />
            <path d="M3 19c.3-3.4 2.1-5 5.5-5s5.2 1.6 5.5 5M14.7 13.3c3.8-.4 5.8 1.2 6.1 4" />
          </svg>
        </button>
        <button
          className="profile-chip profile-button"
          type="button"
          onClick={onEditProfile}
          aria-label="Редактировать профиль"
        >
          <Avatar user={user} />
          <span><strong>{user.display_name}</strong></span>
        </button>
        <button
          aria-label="Выйти"
          className="logout-button"
          type="button"
          onClick={() => void onLogout()}
        >
          <svg aria-hidden="true" viewBox="0 0 24 24">
            <path d="M10 5H6a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h4" />
            <path d="m15 8 4 4-4 4M19 12H9" />
          </svg>
        </button>
      </div>
    </header>
  );
}

export function StoredAvatarImage({
  userID,
  revision,
  legacyURL,
  previewURL,
}: StoredAvatarImageProps) {
  const [source, setSource] = useState(previewURL || legacyURL || "");

  useEffect(() => {
    if (previewURL) {
      setSource(previewURL);
      return;
    }
    if (!revision) {
      setSource(legacyURL || "");
      return;
    }

    let active = true;
    let objectURL = "";
    setSource("");
    void api.getUserAvatar(userID, revision)
      .then((file) => {
        if (!active) return;
        objectURL = URL.createObjectURL(file);
        setSource(objectURL);
      })
      .catch(() => {
        if (active) setSource(legacyURL || "");
      });

    return () => {
      active = false;
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [legacyURL, previewURL, revision, userID]);

  if (!source) return null;
  return (
    <img
      src={source}
      alt=""
      referrerPolicy="no-referrer"
      onError={(event) => {
        event.currentTarget.hidden = true;
      }}
    />
  );
}

export function Avatar({
  user,
  large = false,
  previewURL,
}: AvatarProps) {
  return (
    <span className={`avatar${large ? " avatar-large" : ""}`} aria-hidden="true">
      <span>{user.display_name.slice(0, 1).toUpperCase()}</span>
      <StoredAvatarImage
        userID={user.id}
        revision={user.avatar_revision}
        legacyURL={user.avatar_url}
        previewURL={previewURL}
      />
    </span>
  );
}
