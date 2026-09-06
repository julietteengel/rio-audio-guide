// mobile/src/data/AssistantRepository.ts
import { API_BASE_URL } from "../config";
import type { Locale } from "../i18n/dictionary";

export type ConversationTurn = { question: string; answer: string };

export type GroundingLevel = "grounded" | "mixed" | "general";

export type AssistantAnswer = {
  answer: string;
  groundingLevel: GroundingLevel;
};

export class AssistantApiError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}

export function toGroundingLevel(raw: string): GroundingLevel {
  return raw === "grounded" || raw === "mixed" || raw === "general" ? raw : "general";
}

export async function askAssistant(
  token: string,
  placeId: string,
  language: Locale,
  question: string,
  history: ConversationTurn[],
): Promise<AssistantAnswer> {
  const res = await fetch(`${API_BASE_URL}/places/${encodeURIComponent(placeId)}/assistant`, {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify({ language, question, history }),
  });

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }

  if (!res.ok) {
    const message =
      body && typeof body === "object" && "error" in body
        ? String((body as { error: unknown }).error)
        : "request failed";
    throw new AssistantApiError(message, res.status);
  }

  const parsed = body as { answer: string; grounding_level: string };
  return { answer: parsed.answer, groundingLevel: toGroundingLevel(parsed.grounding_level) };
}
