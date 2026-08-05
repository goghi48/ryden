import { ApiError } from "./api";

export function errorMessage(error: unknown): string {
  if (error instanceof ApiError) {
    return error.message;
  }
  if (error instanceof Error && !(error instanceof TypeError) && error.message.trim()) {
    return error.message;
  }
  return "Связь прервалась. Попробуйте ещё раз.";
}
