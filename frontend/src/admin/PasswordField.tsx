import { useId, useState } from "react";

// Mirrors auth.PasswordProblems in backend/internal/auth/password.go. The
// server enforces these; this component only gives live feedback.
export const MIN_PASSWORD_LENGTH = 12;

const passwordRules: { label: string; test: (p: string) => boolean }[] = [
  { label: `${MIN_PASSWORD_LENGTH}+ characters`, test: (p) => [...p].length >= MIN_PASSWORD_LENGTH },
  { label: "Uppercase letter", test: (p) => /\p{Lu}/u.test(p) },
  { label: "Lowercase letter", test: (p) => /\p{Ll}/u.test(p) },
  { label: "Number", test: (p) => /\p{Nd}/u.test(p) },
  { label: "Symbol (! @ # ?)", test: (p) => /[\p{P}\p{S}]/u.test(p) },
];

const COMMON = ["password", "passw0rd", "qwerty", "123456", "admin", "letmein", "welcome", "goblog", "iloveyou", "abc123"];

const LEVELS = ["", "Weak", "Fair", "Good", "Strong"] as const;

// A simple 0–4 score: the rules decide the ceiling, length pushes a valid
// password from Good to Strong, and common words or repeats cost a level.
export function passwordStrength(p: string): { score: number; label: string; met: boolean[]; hint: string } {
  const met = passwordRules.map((r) => r.test(p));
  if (!p) return { score: 0, label: "", met, hint: "" };

  const metCount = met.filter(Boolean).length;
  let score = metCount <= 2 ? 1 : metCount < met.length ? 2 : [...p].length >= 16 ? 4 : 3;

  const lower = p.toLowerCase();
  let hint = "";
  if (COMMON.some((w) => lower.includes(w)) || /(.)\1\1/.test(p)) {
    score = Math.max(1, score - 1);
    hint = "Avoid common words and repeated characters.";
  } else if (score === 3) {
    hint = "Make it 16 characters or longer to make it strong.";
  }
  return { score, label: LEVELS[score], met, hint };
}

// New-password + confirmation fields with a strength meter and a live
// checklist of the rules. Works as plain inputs before hydration.
export function PasswordFields({ label = "New password", error }: { label?: string; error?: string }) {
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [visible, setVisible] = useState(false);
  const id = useId();
  const { score, label: strengthLabel, met, hint } = passwordStrength(password);

  return (
    <>
      <div className={error ? "field has-error" : "field"}>
        <label className="field-label" htmlFor={`${id}-password`}>
          {label}
        </label>
        <div className="pw-input">
          <input
            id={`${id}-password`}
            type={visible ? "text" : "password"}
            name="password"
            autoComplete="new-password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            aria-describedby={`${id}-strength ${id}-rules`}
          />
          <button type="button" className="pw-toggle" onClick={() => setVisible((v) => !v)} aria-pressed={visible}>
            {visible ? "Hide" : "Show"}
          </button>
        </div>

        <div className="pw-meter" data-score={score} aria-hidden="true">
          <span />
          <span />
          <span />
          <span />
        </div>
        <p id={`${id}-strength`} className="pw-strength" aria-live="polite">
          {strengthLabel ? (
            <>
              Strength: <strong>{strengthLabel}</strong>
              {hint && <span className="pw-hint"> · {hint}</span>}
            </>
          ) : (
            "Use a mix of letters, numbers and symbols."
          )}
        </p>

        <ul id={`${id}-rules`} className="pw-rules">
          {passwordRules.map((r, i) => (
            <li key={r.label} className={met[i] ? "is-met" : undefined}>
              <span className="pw-check" aria-hidden="true">
                {met[i] ? "✓" : ""}
              </span>
              {r.label}
              <span className="visually-hidden">{met[i] ? " (done)" : " (missing)"}</span>
            </li>
          ))}
        </ul>
        {error && <span className="field-error">{error}</span>}
      </div>

      <div className="field">
        <label className="field-label" htmlFor={`${id}-confirm`}>
          Confirm password
        </label>
        <input
          id={`${id}-confirm`}
          type={visible ? "text" : "password"}
          name="confirm"
          autoComplete="new-password"
          required
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
        />
        {confirm && (
          <span className={confirm === password ? "pw-match is-met" : "pw-match"} aria-live="polite">
            {confirm === password ? "✓ Passwords match" : "Passwords don't match yet"}
          </span>
        )}
      </div>
    </>
  );
}
