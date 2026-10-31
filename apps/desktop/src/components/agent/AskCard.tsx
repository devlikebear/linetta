import { useState } from "react";
import { useI18n } from "../../lib/i18n";
import type { AgentAsk, AgentAskQuestion } from "../../lib/types";
import { Markdown } from "./Markdown";

/** What the writer has picked for one question. The free answer counts only
 *  while `other` is on, so turning it off keeps the typing without sending it. */
interface Pick {
  chosen: string[];
  other: boolean;
  text: string;
}

const NO_PICK: Pick = { chosen: [], other: false, text: "" };

function answered(pick: Pick): boolean {
  return pick.chosen.length > 0 || (pick.other && pick.text.trim() !== "");
}

/** The card's answers as the writer's next message: one `header: answer` line
 *  per question. The model has the options in front of it (agent/ask.go's
 *  askedMessage), so the labels are enough, and a restored conversation shows
 *  this as what the writer said because it is. Exported for the tests. */
export function formatAskAnswers(questions: AgentAskQuestion[], picks: Pick[]): string {
  return questions
    .map((q, i) => {
      const pick = picks[i] ?? NO_PICK;
      const answers = [...pick.chosen, ...(pick.other && pick.text.trim() ? [pick.text.trim()] : [])];
      return `${q.header}: ${answers.join(", ")}`;
    })
    .join("\n");
}

function QuestionFields({
  question,
  index,
  pick,
  disabled,
  onChange,
}: {
  question: AgentAskQuestion;
  index: number;
  pick: Pick;
  disabled: boolean;
  onChange: (pick: Pick) => void;
}) {
  const { t } = useI18n();
  const multi = question.multi_select === true;
  const name = `agent-ask-${index}`;

  function toggle(label: string) {
    if (!multi) {
      onChange({ ...pick, chosen: [label], other: false });
      return;
    }
    const chosen = pick.chosen.includes(label) ? pick.chosen.filter((l) => l !== label) : [...pick.chosen, label];
    onChange({ ...pick, chosen });
  }

  return (
    <fieldset className="agent-ask-question" disabled={disabled} data-testid={`agent-ask-question-${index}`}>
      <legend className="agent-ask-header">
        <span>{question.header}</span>
        <span className="agent-ask-mode">{t(multi ? "agentPanel.ask.pickMany" : "agentPanel.ask.pickOne")}</span>
      </legend>
      <div className="agent-ask-text">
        <Markdown text={question.question} />
      </div>
      {question.options.map((option) => (
        <label key={option.label} className={`agent-ask-option${pick.chosen.includes(option.label) ? " is-chosen" : ""}`}>
          <input
            type={multi ? "checkbox" : "radio"}
            name={name}
            checked={pick.chosen.includes(option.label)}
            onChange={() => toggle(option.label)}
          />
          <span className="agent-ask-option-body">
            <span className="agent-ask-option-label">
              {option.label}
              {option.recommended ? <span className="agent-ask-recommended">{t("agentPanel.ask.recommended")}</span> : null}
            </span>
            {option.description ? (
              <span className="agent-ask-option-description">
                <Markdown text={option.description} />
              </span>
            ) : null}
          </span>
        </label>
      ))}
      <label className={`agent-ask-option${pick.other ? " is-chosen" : ""}`}>
        <input
          type={multi ? "checkbox" : "radio"}
          name={name}
          checked={pick.other}
          data-testid={`agent-ask-other-${index}`}
          onChange={() => onChange(multi ? { ...pick, other: !pick.other } : { ...pick, chosen: [], other: true })}
        />
        <span className="agent-ask-option-label">{t("agentPanel.ask.other")}</span>
      </label>
      {pick.other ? (
        <textarea
          className="agent-ask-input"
          data-testid={`agent-ask-input-${index}`}
          aria-label={t("agentPanel.ask.other")}
          placeholder={t("agentPanel.ask.otherPlaceholder")}
          value={pick.text}
          rows={2}
          autoFocus
          onChange={(e) => onChange({ ...pick, text: e.target.value })}
        />
      ) : null}
    </fieldset>
  );
}

interface Props {
  ask: AgentAsk;
  /** Whether the card still waits for an answer. Anything the writer sent
   *  after it answers it, by card or by typing, and it folds up. */
  open: boolean;
  /** A turn is running, so nothing can be sent yet. */
  disabled: boolean;
  onSubmit: (answers: string) => void;
}

/** The agent's questions, answered together in one message (agent/ask.go). The
 *  card asks nothing of the engine: its answers go out as an ordinary turn,
 *  which is what lets it wait as long as the writer needs, across restarts. */
export function AskCard({ ask, open, disabled, onSubmit }: Props) {
  const { t } = useI18n();
  const [picks, setPicks] = useState<Pick[]>(() => ask.questions.map(() => NO_PICK));

  if (!open) {
    return (
      <details className="agent-ask agent-ask-closed" data-testid="agent-ask-closed">
        <summary>{t("agentPanel.ask.closed", { count: ask.questions.length })}</summary>
        {ask.questions.map((question, index) => (
          <div key={index} className="agent-ask-text">
            <Markdown text={question.question} />
          </div>
        ))}
      </details>
    );
  }

  const done = picks.filter(answered).length;
  return (
    <form
      className="agent-ask is-open"
      data-testid="agent-ask"
      onSubmit={(e) => {
        e.preventDefault();
        if (done === ask.questions.length && !disabled) onSubmit(formatAskAnswers(ask.questions, picks));
      }}
    >
      {ask.questions.map((question, index) => (
        <QuestionFields
          key={index}
          question={question}
          index={index}
          pick={picks[index] ?? NO_PICK}
          disabled={disabled}
          onChange={(pick) => setPicks((cur) => cur.map((p, i) => (i === index ? pick : p)))}
        />
      ))}
      <div className="agent-ask-foot">
        <span data-testid="agent-ask-progress">
          {t("agentPanel.ask.progress", { done, total: ask.questions.length })}
        </span>
        <button type="submit" className="btn accent sm" data-testid="agent-ask-submit" disabled={disabled || done < ask.questions.length}>
          {t("agentPanel.ask.submit")}
        </button>
      </div>
    </form>
  );
}
